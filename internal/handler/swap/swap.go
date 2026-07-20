package swap

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
)

type GenerateSignatureRequest struct {
	ICYAmount  string `json:"icy_amount" binding:"required"`
	BTCAddress string `json:"btc_address" binding:"required"`
	SatAmount  string `json:"btc_amount" binding:"required"`

	// WalletSignature is an EIP-712 SwapRequest signed by the wallet that will
	// call swap(), and WalletDeadline is the expiry carried inside it. Together
	// they give this endpoint a real caller identity; the ApiKey cannot, since
	// it ships inside the browser bundle. Optional until REQUIRE_WALLET_AUTH is
	// turned on, so the frontend can deploy before enforcement begins.
	WalletSignature string `json:"wallet_signature"`
	WalletDeadline  int64  `json:"wallet_deadline"`
}

// oracleRateToleranceNum/Denom bound how far above the oracle-derived amount a
// client-supplied btc_amount may be before GenerateSignature rejects it (here 1%:
// reject when clientSat > serverSat * 101/100). The SIGNED amount is always the
// server-derived one; this tolerance only gates acceptance of the client input,
// absorbing benign rounding/rate-drift between /swap/info and this call.
const (
	oracleRateToleranceNum   = 101
	oracleRateToleranceDenom = 100
)

type handler struct {
	logger          *logger.Logger
	appConfig       *config.AppConfig
	oracle          oracle.IOracle
	baseRPC         baserpc.IBaseRPC
	btcRPC          btcrpc.IBtcRpc
	metricsRecorder *monitoring.BusinessMetricsRecorder
}

func New(
	logger *logger.Logger,
	appConfig *config.AppConfig,
	oracle oracle.IOracle,
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	metricsRecorder *monitoring.BusinessMetricsRecorder,
) IHandler {
	return &handler{
		logger:          logger,
		appConfig:       appConfig,
		oracle:          oracle,
		baseRPC:         baseRPC,
		btcRPC:          btcRPC,
		metricsRecorder: metricsRecorder,
	}
}

func (h *handler) GenerateSignature(c *gin.Context) {
	var req GenerateSignatureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("[GenerateSignature][ShouldBindJSON]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	// Validate req
	err := validator.New().Struct(req)
	if err != nil {
		h.logger.Error("[GenerateSignature][Validator]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	// Establish who is asking, before doing any work on their behalf. When a
	// wallet signature is present it is always verified; whether one is
	// REQUIRED is config-gated so the frontend can ship first.
	// Normalize ONCE, at the edge. Validation trims but the raw value used to
	// be what got hashed into the swap signature and passed to Send, so a
	// padded address could pass validation and then fail at broadcast, after
	// the ICY leg had already burned.
	req.BTCAddress = btcrpc.NormalizeAddress(req.BTCAddress)

	caller, err := h.authenticateCaller(&req)
	if err != nil {
		h.logger.Error("[GenerateSignature][WalletAuth]", map[string]string{
			"error": err.Error(),
		})
		status, msg := http.StatusUnauthorized, "wallet signature is missing or invalid"
		switch {
		case errors.Is(err, ErrWalletRateLimited):
			status, msg = http.StatusTooManyRequests, "too many signature requests for this wallet"
		case errors.Is(err, ErrInsufficientICY):
			status, msg = http.StatusForbidden, "wallet does not hold enough ICY for this swap"
		}
		c.JSON(status, view.CreateResponse[any](nil, err, nil, msg))
		return
	}
	if caller != "" {
		// Attribution has to be readable to be worth anything. gin's default
		// log formatter does not emit context keys, so this is logged
		// explicitly: the documented rollout gates flipping REQUIRE_WALLET_AUTH
		// on seeing signed traffic arrive, and without this line that
		// confirmation never appears.
		c.Set("caller_wallet", caller)
		h.logger.Info("[GenerateSignature] authenticated caller " + caller)
	}

	// SECURITY: the destination address is signed and paid out, so it must be a
	// mainnet address. `binding:"required"` only checks it is non-empty, and
	// getDustLimit's prefix table silently accepts tb1/bcrt1/garbage with a
	// default dust limit, so without this the signer would happily authorise a
	// payout to an address that cannot be paid on the network we pay from. The
	// frontend checks this too, but the client is bypassable.
	if err := btcrpc.ValidateMainnetAddress(req.BTCAddress); err != nil {
		h.logger.Error("[GenerateSignature][ValidateMainnetAddress]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, nil, "btc_address must be a Bitcoin mainnet address"))
		return
	}

	// Convert ICY amount to Web3BigInt (18-decimals, wei-scaled)
	icyAmount := &model.Web3BigInt{
		Value:   req.ICYAmount,
		Decimal: 18,
	}

	icyAmountBig, ok := new(big.Int).SetString(req.ICYAmount, 10)
	if !ok || icyAmountBig.Sign() <= 0 {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("invalid ICY amount"), req, "invalid ICY amount"))
		return
	}

	// SECURITY (CRIT-1): derive the BTC payout server-side from the oracle rate and
	// sign THAT. The client-supplied btc_amount is never trusted for the signed value:
	// a signature worth more BTC than the ICY justifies drains the treasury, and this
	// endpoint may be reached unauthenticated. We reproduce the same proportional rate
	// the public /swap/info endpoint already exposes to the frontend
	// (satoshi = icy_wei * btcSupply / circulatedICY), reading the 5-minute cached
	// oracle balances so this stays fast on a hot, public endpoint.
	circulatedICY, err := h.oracle.GetCachedCirculatedICY()
	if err != nil {
		h.logger.Error("[GenerateSignature][GetCachedCirculatedICY]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusServiceUnavailable, view.CreateResponse[any](nil, err, nil, "failed to get oracle rate"))
		return
	}
	btcSupply, err := h.oracle.GetCachedBTCSupply()
	if err != nil {
		h.logger.Error("[GenerateSignature][GetCachedBTCSupply]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusServiceUnavailable, view.CreateResponse[any](nil, err, nil, "failed to get oracle rate"))
		return
	}

	circBig, okCirc := new(big.Int).SetString(circulatedICY.Value, 10)
	supplyBig, okSupply := new(big.Int).SetString(btcSupply.Value, 10)
	if !okCirc || !okSupply || circBig.Sign() <= 0 || supplyBig.Sign() < 0 {
		h.logger.Error("[GenerateSignature][OracleBalances]", map[string]string{
			"circulated_icy": circulatedICY.Value,
			"btc_supply":     btcSupply.Value,
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, errors.New("invalid oracle balances"), nil, "failed to compute swap rate"))
		return
	}

	// serverSat = icy_wei * btcSupply / circulatedICY (floored). This equals
	// icy_amount * (satoshi-per-ICY rate) and can never exceed the treasury's
	// proportional backing for the given ICY, regardless of what the client sent.
	serverSatBig := new(big.Int).Div(new(big.Int).Mul(icyAmountBig, supplyBig), circBig)

	// The signed BTC amount is ALWAYS the server-derived one.
	btcAmount := &model.Web3BigInt{
		Value:   serverSatBig.String(),
		Decimal: consts.BTC_DECIMALS,
	}

	// Defense-in-depth: reject a client-supplied btc_amount that is inflated beyond a
	// small tolerance versus the oracle-derived amount. Signing already uses the
	// server value, so this only surfaces a malicious or badly-stale client early.
	if clientSat, okClient := new(big.Int).SetString(req.SatAmount, 10); okClient && clientSat.Sign() > 0 {
		lhs := new(big.Int).Mul(clientSat, big.NewInt(oracleRateToleranceDenom))
		rhs := new(big.Int).Mul(serverSatBig, big.NewInt(oracleRateToleranceNum))
		if lhs.Cmp(rhs) > 0 {
			h.logger.Error("[GenerateSignature][InflatedBTCAmount]", map[string]string{
				"client_btc_amount": req.SatAmount,
				"oracle_btc_amount": serverSatBig.String(),
			})
			c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("btc amount exceeds oracle-derived amount"), nil, "btc_amount is inflated versus the oracle rate"))
			return
		}
	}

	// Dust check on the SERVER-derived amount.
	amountInt := serverSatBig.Int64()
	if h.btcRPC.IsDust(req.BTCAddress, amountInt) {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("amount is dust"), nil, "btc amount is dust, it should be greater than 546 satoshi"))
		return
	}

	btcDecimal, err := decimal.NewFromString(btcAmount.Value)
	if err != nil {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid BTC amount"))
		return
	}
	// PRECISION: stay in decimal.Decimal for the whole fee-vs-amount gate. The
	// prior code round-tripped through .InexactFloat64() to do the comparison,
	// which can lose satoshi-scale precision for large BTC amounts; this
	// enforcement gate stays big.Int/decimal end-to-end like the signed
	// serverSatBig amount above it.
	svcFeeDecimal := btcDecimal.Mul(decimal.NewFromFloat(h.appConfig.Bitcoin.ServiceFeeRate))
	minFeeDecimal := decimal.NewFromInt(h.appConfig.Bitcoin.MinSatshiFee)
	if svcFeeDecimal.LessThan(minFeeDecimal) {
		svcFeeDecimal = minFeeDecimal
	}
	// Gate on the NET amount, which is what actually gets sent, not on the
	// subtotal being merely non-negative. The dust check above runs on the
	// subtotal, so with the 3,000 sat minimum fee a subtotal of 3,000..3,545
	// cleared both gates and produced a payout of 0..545 sat: at or under every
	// dust limit, unbroadcastable forever. The user's ICY burns and no BTC can
	// ever be sent, so it has to be refused here, before anything is signed.
	netDecimal := btcDecimal.Sub(svcFeeDecimal)
	if netDecimal.IsNegative() || h.btcRPC.IsDust(req.BTCAddress, netDecimal.IntPart()) {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("amount after service fee is dust"), nil, "the amount left after the service fee is too small to send on Bitcoin"))
		return
	}

	nonce := big.NewInt(time.Now().UnixNano())
	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	// Generate signature
	signature, err := h.baseRPC.GenerateSignature(icyAmount, req.BTCAddress, btcAmount, nonce, deadline)
	if err != nil {
		h.logger.Error("[GenerateSignature][BaseRPC]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to generate signature"))
		return
	}

	c.JSON(http.StatusOK, view.CreateResponse[any](map[string]interface{}{
		"signature":  signature,
		"nonce":      nonce.String(),
		"deadline":   deadline.String(),
		"icy_amount": icyAmount.Value,
		"btc_amount": btcAmount.Value,
	}, nil, nil, "signature generated successfully"))
}

func (h *handler) Info(c *gin.Context) {
	// Increased timeout from 15s to 45s for complex operations
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()

	start := time.Now()

	type result struct {
		satPerUSD            float64
		circulatedIcyBalance *model.Web3BigInt
		satBalance           *model.Web3BigInt
		err                  error
		source               string
	}

	resultCh := make(chan result, 3)

	// Use cached methods for better performance
	go func() {
		satPerUSD, err := h.btcRPC.GetSatoshiUSDPrice()
		resultCh <- result{satPerUSD: satPerUSD, err: err, source: "GetSatoshiUSDPrice"}
	}()

	go func() {
		// Use cached version with context for better timeout handling
		circulatedIcyBalance, err := h.oracle.GetCirculatedICYWithContext(ctx)
		resultCh <- result{circulatedIcyBalance: circulatedIcyBalance, err: err, source: "GetCirculatedICY"}
	}()

	go func() {
		// Use cached version with context for better timeout handling
		satBalance, err := h.oracle.GetBTCSupplyWithContext(ctx)
		resultCh <- result{satBalance: satBalance, err: err, source: "GetBTCSupply"}
	}()

	// Graceful degradation - collect partial results
	var satPerUSD float64
	var circulatedIcyBalance *model.Web3BigInt
	var satBalance *model.Web3BigInt
	var errors []string
	var hasValidData bool

	// Wait for all results with timeout
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			// Check if we have at least some data to return
			if hasValidData {
				h.logger.Info("[Info] Operation timed out but returning partial data", map[string]string{
					"duration": fmt.Sprintf("%v", time.Since(start).Seconds()),
					"errors":   fmt.Sprintf("%v", errors),
				})
				break
			}
			if h.metricsRecorder != nil {
				h.metricsRecorder.RecordSwapOperation("swap_info", "timeout", time.Since(start).Seconds())
			}
			c.JSON(http.StatusGatewayTimeout, view.CreateResponse[any](nil, ctx.Err(), nil, "operation timed out"))
			return
		case res := <-resultCh:
			if res.err != nil {
				errMsg := fmt.Sprintf("failed to get %s: %s", res.source, res.err.Error())
				errors = append(errors, errMsg)
				h.logger.Error(fmt.Sprintf("[Info][%s]", res.source), map[string]string{
					"error": res.err.Error(),
				})
				// Continue processing other results instead of failing immediately
				continue
			}

			// Assign successful results
			switch res.source {
			case "GetSatoshiUSDPrice":
				satPerUSD = res.satPerUSD
				hasValidData = true
			case "GetCirculatedICY":
				circulatedIcyBalance = res.circulatedIcyBalance
				hasValidData = true
			case "GetBTCSupply":
				satBalance = res.satBalance
				hasValidData = true
			}
		}
	}

	// If no valid data was retrieved, return error
	if !hasValidData {
		if h.metricsRecorder != nil {
			h.metricsRecorder.RecordSwapOperation("swap_info", "error", time.Since(start).Seconds())
		}
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, fmt.Errorf("all operations failed"), nil, "failed to retrieve any data"))
		return
	}
	h.logger.Info("[Info][GetInfo]", map[string]string{
		"duration": fmt.Sprintf("%v", time.Since(start).Seconds()),
		"partial":  fmt.Sprintf("%t", len(errors) > 0),
		"errors":   fmt.Sprintf("%d", len(errors)),
	})

	// Build response with available data - graceful degradation
	response := make(map[string]interface{})

	// Add warnings if there were errors
	if len(errors) > 0 {
		response["warnings"] = errors
		response["partial_data"] = true
	}

	// Add available data
	if circulatedIcyBalance != nil {
		response["circulated_icy_balance"] = circulatedIcyBalance.Value
	}

	if satBalance != nil {
		response["satoshi_balance"] = satBalance.Value
	}

	if satPerUSD > 0 {
		response["satoshi_per_usd"] = math.Floor(satPerUSD*100) / 100

		// Calculate satoshi USD rate if we have satPerUSD
		satusd := new(big.Float).Quo(new(big.Float).SetFloat64(1), new(big.Float).SetFloat64(satPerUSD))
		satusdFloat, _ := satusd.Float64()
		response["satoshi_usd_rate"] = fmt.Sprintf("%f", satusdFloat)
	}

	// Calculate rates only if we have both ICY and BTC data
	if circulatedIcyBalance != nil && satBalance != nil {
		icyDecimalRaw, err := decimal.NewFromString(circulatedIcyBalance.Value)
		if err != nil {
			h.logger.Error("[Info][ConvertIcyBalance]", map[string]string{
				"error": err.Error(),
			})
			response["icy_conversion_error"] = "failed to parse ICY balance"
		} else {
			icyDecimal := icyDecimalRaw.Div(decimal.NewFromInt(1e18))
			satDecimal, _ := decimal.NewFromString(satBalance.Value)

			// Calculate satoshi per 1 ICY
			icysat := satDecimal.Div(icyDecimal).InexactFloat64()
			response["icy_satoshi_rate"] = fmt.Sprintf("%.2f", icysat) // How many satoshi per 1 ICY

			// Calculate ICY USD rate if we also have satPerUSD
			if satPerUSD > 0 {
				satusd := new(big.Float).Quo(new(big.Float).SetFloat64(1), new(big.Float).SetFloat64(satPerUSD))
				satusdFloat, _ := satusd.Float64()
				icyusd := icysat * satusdFloat
				response["icy_usd_rate"] = fmt.Sprintf("%.4f", icyusd)
			}

			// Calculate minimum ICY to swap
			minIcySwap := model.Web3BigInt{
				Value:   fmt.Sprintf("%0.0f", h.appConfig.MinIcySwapAmount),
				Decimal: 18,
			}
			minIcyAmount := minIcySwap.ToFloat()
			minSatAmount := minIcyAmount * icysat
			if minSatAmount < 546 { // BTC dust limit
				svcFee := minSatAmount * h.appConfig.Bitcoin.ServiceFeeRate
				if svcFee < float64(h.appConfig.Bitcoin.MinSatshiFee) {
					svcFee = float64(h.appConfig.Bitcoin.MinSatshiFee)
				}
				minSatAmount = 546 + svcFee
				minIcyAmount = (minSatAmount / icysat) * 1e18
				minIcySwap.Value = fmt.Sprintf("%0.0f", minIcyAmount)
			}
			response["min_icy_to_swap"] = minIcySwap.Value
		}
	}

	// Always include service configuration
	response["service_fee_rate"] = h.appConfig.Bitcoin.ServiceFeeRate
	response["min_satoshi_fee"] = fmt.Sprintf("%d", h.appConfig.Bitcoin.MinSatshiFee)

	// Return partial success (200) if we have any data, even with errors
	duration := time.Since(start).Seconds()
	if h.metricsRecorder != nil {
		status := "success"
		if len(errors) > 0 {
			status = "partial_success"
		}
		h.metricsRecorder.RecordSwapOperation("swap_info", status, duration)
	}
	c.JSON(http.StatusOK, view.CreateResponse[any](response, nil, nil, "info retrieved successfully"))
}
