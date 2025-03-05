package swap

import (
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

	// Add rate validation before generating signature
	satAmountFloat, err := strconv.ParseFloat(req.SatAmount, 64)
	if err != nil {
		h.logger.Error("[GenerateSignature][ParseFloat]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid BTC amount"))
		return
	}

	rate := decimal.NewFromFloat(satAmountFloat).Div(decimal.NewFromFloat(icyAmount.ToFloat()))

	expectedRate, err := h.oracle.GetRealtimeICYBTC()
	if err != nil {
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get ICY/BTC rate"))
		return
	}

	// icyPerSat, _ := new(big.Float).Quo(new(big.Float).SetFloat64(1e8), new(big.Float).SetFloat64(expectedRate.ToFloat())).Float64()
	icyPerSat := new(big.Float).Quo(new(big.Float).SetFloat64(1e8), new(big.Float).SetFloat64(expectedRate.ToFloat()))
	satPerIcy, _ := new(big.Float).Quo(new(big.Float).SetFloat64(1), icyPerSat).Float64()
	// // Allow for small deviation (e.g., 5%)
	tolerance := decimal.NewFromFloat(0.05)
	upperLimit, _ := decimal.NewFromFloat(satPerIcy).Mul(decimal.NewFromFloat(1).Add(tolerance)).Float64()
	lowerLimit, _ := decimal.NewFromFloat(satPerIcy).Mul(decimal.NewFromFloat(1).Sub(tolerance)).Float64()

	// Update to floats without decimal places
	upperLimit = math.Ceil(upperLimit)
	lowerLimit = math.Floor(lowerLimit)

	if rate.GreaterThan(decimal.NewFromFloat(upperLimit)) || rate.LessThan(decimal.NewFromFloat(lowerLimit)) {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, errors.New("rate deviation too high"), nil, "failed to generate signature"))
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
	// Get minimum ICY to swap from config
	minIcySwap := model.Web3BigInt{
		Value:   fmt.Sprintf("%0.0f", h.appConfig.MinIcySwapAmount),
		Decimal: 18,
	}

	// Get ICY/BTC rate from oracle (using cached realtime rate)
	// This rate represents how many ICY tokens per 1 BTC (scaled by 10^8)
	rate, err := h.oracle.GetRealtimeICYBTC()
	if err != nil {
		h.logger.Error("[Info][GetCachedRealtimeICYBTC]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get ICY/BTC rate"))
		return
	}
	fmt.Println("[Info] rate", rate)

	satPerUSD, err := h.btcRPC.GetSatoshiUSDPrice()
	if err != nil {
		h.logger.Error("[Info][GetSatoshiUSDPrice]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get satoshi price"))
		return
	}

	// Get circulated ICY balance
	circulatedIcyBalance, err := h.oracle.GetCirculatedICY()
	if err != nil {
		h.logger.Error("[Info][GetCirculatedICY]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get circulated ICY balance"))
		return
	}

	// Get BTC supply
	satBalance, err := h.oracle.GetBTCSupply()
	if err != nil {
		h.logger.Error("[Info][GetBTCSupply]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get BTC balance"))
		return
	}

	// Calculate satoshi per 1 ICY
	// First, calculate ICY per 1 satoshi
	icyPerSat := new(big.Float).Quo(new(big.Float).SetFloat64(1e8), new(big.Float).SetFloat64(rate.ToFloat()))
	// Then, calculate satoshi per 1 ICY (reciprocal of ICY per 1 satoshi)
	satPerIcy := new(big.Float).Quo(new(big.Float).SetFloat64(1), icyPerSat)
	satusd := new(big.Float).Quo(new(big.Float).SetFloat64(1), new(big.Float).SetFloat64(satPerUSD))
	satusdFloat, _ := satusd.Float64()
	icyusd, _ := new(big.Float).Mul(icyPerSat, satusd).Float64()
	icyusdWeb3BigInt := model.Web3BigInt{
		Value:   fmt.Sprintf("%0.0f", icyusd*1e18),
		Decimal: 18,
	}

	c.JSON(http.StatusOK, view.CreateResponse[any](map[string]interface{}{
		"circulated_icy_balance": circulatedIcyBalance.Value,
		"satoshi_balance":        satBalance.Value,
		"satoshi_per_usd":        math.Floor(satPerUSD*100) / 100,
		"icy_satoshi_rate":       fmt.Sprintf("%.0f", satPerIcy), // How many satoshi per 1 ICY
		"icy_usd_rate":           icyusdWeb3BigInt.Value,
		"satoshi_usd_rate":       fmt.Sprintf("%f", satusdFloat),
		"min_icy_to_swap":        minIcySwap.Value,
		"service_fee_rate":       h.appConfig.Bitcoin.ServiceFeeRate,
		"min_satoshi_fee":        fmt.Sprintf("%d", h.appConfig.Bitcoin.MinSatshiFee),
	}, nil, nil, "swap info retrieved successfully"))
}
