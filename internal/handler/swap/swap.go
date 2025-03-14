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
}

func New(
	logger *logger.Logger,
	appConfig *config.AppConfig,
	oracle oracle.IOracle,
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	db *gorm.DB,
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// Get minimum ICY to swap from config
	minIcySwap := model.Web3BigInt{
		Value:   fmt.Sprintf("%0.0f", h.appConfig.MinIcySwapAmount),
		Decimal: 18,
	}

	start := time.Now()

	type result struct {
		satPerUSD            float64
		circulatedIcyBalance *model.Web3BigInt
		satBalance           *model.Web3BigInt
		err                  error
		source               string
	}

	resultCh := make(chan result, 3)

	// Get satoshi per USD
	go func() {
		satPerUSD, err := h.btcRPC.GetSatoshiUSDPrice()
		resultCh <- result{satPerUSD: satPerUSD, err: err, source: "GetSatoshiUSDPrice"}
	}()

	go func() {
		circulatedIcyBalance, err := h.oracle.GetCirculatedICY()
		resultCh <- result{circulatedIcyBalance: circulatedIcyBalance, err: err, source: "GetCirculatedICY"}
	}()

	go func() {
		satBalance, err := h.oracle.GetBTCSupply()
		resultCh <- result{satBalance: satBalance, err: err, source: "GetBTCSupply"}
	}()

	// Collect results and handle errors
	var satPerUSD float64
	var circulatedIcyBalance *model.Web3BigInt
	var satBalance *model.Web3BigInt

	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			c.JSON(http.StatusGatewayTimeout, view.CreateResponse[any](nil, ctx.Err(), nil, "operation timed out"))
			return
		case res := <-resultCh:
			if res.err != nil {
				h.logger.Error(fmt.Sprintf("[Info][%s]", res.source), map[string]string{
					"error": res.err.Error(),
				})
				c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, res.err, nil, fmt.Sprintf("failed to get %s", res.source)))
				return
			}

			// Assign results based on source
			switch res.source {
			case "GetSatoshiUSDPrice":
				satPerUSD = res.satPerUSD
			case "GetCirculatedICY":
				circulatedIcyBalance = res.circulatedIcyBalance
			case "GetBTCSupply":
				satBalance = res.satBalance
			}
		}
	}
	h.logger.Info("[Info][GetInfo]", map[string]string{
		"duration": fmt.Sprintf("%v", time.Since(start).Seconds()),
	})

	// Convert Web3BigInt to decimal.Decimal for division
	icyDecimalRaw, err := decimal.NewFromString(circulatedIcyBalance.Value)
	if err != nil {
		h.logger.Error("[Info][ConvertIcyBalance]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to parse ICY balance"))
		return
	}
	icyDecimal := icyDecimalRaw.Div(decimal.NewFromInt(1e18))
	satDecimal, _ := decimal.NewFromString(satBalance.Value)
	// Calculate satoshi per 1 ICY
	satPerIcy := satDecimal.Div(icyDecimal).InexactFloat64()
	satusd := new(big.Float).Quo(new(big.Float).SetFloat64(1), new(big.Float).SetFloat64(satPerUSD))
	satusdFloat, _ := satusd.Float64()
	icyusd := satPerIcy * satusdFloat

	c.JSON(http.StatusOK, view.CreateResponse[any](map[string]interface{}{
		"circulated_icy_balance": circulatedIcyBalance.Value,
		"satoshi_balance":        satBalance.Value,
		"satoshi_per_usd":        math.Floor(satPerUSD*100) / 100,
		"icy_satoshi_rate":       fmt.Sprintf("%.0f", satPerIcy), // How many satoshi per 1 ICY
		"icy_usd_rate":           fmt.Sprintf("%.2f", icyusd),
		"satoshi_usd_rate":       fmt.Sprintf("%f", satusdFloat),
		"min_icy_to_swap":        minIcySwap.Value,
		"service_fee_rate":       h.appConfig.Bitcoin.ServiceFeeRate,
		"min_satoshi_fee":        fmt.Sprintf("%d", h.appConfig.Bitcoin.MinSatshiFee),
	}, nil, nil, "swap info retrieved successfully"))
}
