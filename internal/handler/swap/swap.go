package swap

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
	"github.com/dwarvesf/icy-backend/internal/store/swaprequest"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
)

type SwapRequest struct {
	ICYAmount  string `json:"icy_amount" binding:"required"`
	BTCAddress string `json:"btc_address" binding:"required"`
	IcyTx      string `json:"icy_tx" binding:"required"`
}

type GenerateSignatureRequest struct {
	ICYAmount  string `json:"icy_amount" binding:"required"`
	BTCAddress string `json:"btc_address" binding:"required"`
	SatAmount  string `json:"btc_amount" binding:"required"`
}

type handler struct {
	logger              *logger.Logger
	appConfig           *config.AppConfig
	oracle              oracle.IOracle
	baseRPC             baserpc.IBaseRPC
	btcRPC              btcrpc.IBtcRpc
	db                  *gorm.DB
	btcProcessedTxStore onchainbtcprocessedtransaction.IStore
	swapRequestStore    swaprequest.IStore
	metricsRecorder     *monitoring.BusinessMetricsRecorder
}

func New(
	logger *logger.Logger,
	appConfig *config.AppConfig,
	oracle oracle.IOracle,
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	db *gorm.DB,
	metricsRecorder *monitoring.BusinessMetricsRecorder,
) IHandler {
	return &handler{
		logger:              logger,
		appConfig:           appConfig,
		oracle:              oracle,
		baseRPC:             baseRPC,
		btcRPC:              btcRPC,
		db:                  db,
		btcProcessedTxStore: onchainbtcprocessedtransaction.New(),
		swapRequestStore:    swaprequest.New(),
		metricsRecorder:     metricsRecorder,
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

	// Convert ICY amount to Web3BigInt
	icyAmount := &model.Web3BigInt{
		Value:   req.ICYAmount,
		Decimal: 18,
	}

	// Convert BTC amount to Web3BigInt
	btcAmount := &model.Web3BigInt{
		Value:   req.SatAmount,
		Decimal: consts.BTC_DECIMALS, // Assuming BTC has 8 decimal places
	}

	amountInt, err := strconv.ParseInt(req.SatAmount, 10, 64)
	if err != nil {
		h.logger.Error("[GenerateSignature][ParseInt]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid BTC amount"))
		return
	}
	if h.btcRPC.IsDust(req.BTCAddress, amountInt) {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("amount is dust"), nil, "btc amount is dust, it should be greater than 546 satoshi"))
		return
	}

	btcDecimal, err := decimal.NewFromString(btcAmount.Value)
	if err != nil {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid BTC amount"))
		return
	}
	svcFee := btcDecimal.Mul(decimal.NewFromFloat(h.appConfig.Bitcoin.ServiceFeeRate)).InexactFloat64()
	if svcFee < float64(h.appConfig.Bitcoin.MinSatshiFee) {
		svcFee = float64(h.appConfig.Bitcoin.MinSatshiFee)
	}
	if btcDecimal.InexactFloat64()-svcFee < 0 {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("Sat amount is not enough to pay service fee"), nil, "failed to generate signature"))
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

// TriggerSwap godoc
// @Summary Trigger ICY-BTC Swap
// @Description Initiates a swap between ICY and BTC
// @id triggerSwap
// @Tags Swap
// @Accept json
// @Produce json
// @Param request body SwapRequest true "Swap request parameters"
// @Success 200 {object} view.MessageResponse
// @Failure 400 {object} view.ErrorResponse
// @Failure 500 {object} view.ErrorResponse
// @Router /swap [post]
func (h *handler) CreateSwapRequest(c *gin.Context) {
	var req SwapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("[CreateSwapRequest][ShouldBindJSON]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	// validate req
	err := validator.New().Struct(req)
	if err != nil {
		h.logger.Error("[CreateSwapRequest][Validator]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	icyAmountFloat, err := strconv.ParseFloat(req.ICYAmount, 64)
	if err != nil {
		h.logger.Error("[CreateSwapRequest][ParseFloat]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid ICY amount"))
		return
	}
	if float64(icyAmountFloat) < h.appConfig.MinIcySwapAmount {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, fmt.Errorf("minimum ICY amount is %v", h.appConfig.MinIcySwapAmount), nil, "invalid ICY amount"))
		return
	}

	// Check if the ICY transaction has already been exisiting
	existingTx, err := h.btcProcessedTxStore.GetByIcyTransactionHash(h.db, req.IcyTx)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		h.logger.Error("[CreateSwapRequest][CheckICYTransaction]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to check ICY transaction"))
		return
	}

	if existingTx != nil {
		h.logger.Error("[CreateSwapRequest][DuplicateICYTransaction]", map[string]string{
			"tx_hash": req.IcyTx,
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, fmt.Errorf("transaction already processed"), nil, "transaction has already been used for a swap"))
		return
	}

	// Begin a transaction to ensure atomicity
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create swap request
	swapRequest := &model.SwapRequest{
		ICYAmount:  req.ICYAmount,
		BTCAddress: req.BTCAddress,
		IcyTx:      req.IcyTx,
		Status:     model.SwapRequestStatusPending,
	}

	_, err = h.swapRequestStore.Create(tx, swapRequest)
	if err != nil {
		tx.Rollback()
		h.logger.Error("[CreateSwapRequest][Create]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to create swap request"))
		return
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		h.logger.Error("[CreateSwapRequest][CommitTransaction]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to commit transaction"))
		return
	}

	c.JSON(http.StatusOK, view.CreateResponse[any]("success", nil, nil, "swap request created successfully"))
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
