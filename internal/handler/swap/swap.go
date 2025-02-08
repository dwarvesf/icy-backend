package swap

import (
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/controller"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
)

type SwapRequest struct {
	ICYAmount          string `json:"icy_amount" binding:"required"`
	ICYTransactionHash string `json:"icy_tx" binding:"required"`
	BTCAddress         string `json:"btc_address" binding:"required"`
}

type handler struct {
	controller          controller.IController
	logger              *logger.Logger
	appConfig           *config.AppConfig
	oracle              oracle.IOracle
	db                  *gorm.DB
	btcProcessedTxStore onchainbtcprocessedtransaction.IStore
}

func New(
	controller controller.IController,
	logger *logger.Logger,
	appConfig *config.AppConfig,
	oracle oracle.IOracle,
	db *gorm.DB,
) IHandler {
	return &handler{
		controller:          controller,
		logger:              logger,
		appConfig:           appConfig,
		oracle:              oracle,
		db:                  db,
		btcProcessedTxStore: onchainbtcprocessedtransaction.New(db),
	}
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
func (h *handler) TriggerSwap(c *gin.Context) {
	var req SwapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("[TriggerSwap][ShouldBindJSON]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	// validate req
	err := validator.New().Struct(req)
	if err != nil {
		h.logger.Error("[TriggerSwap][Validator]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	// Check if this ICY transaction has already been processed for BTC
	_, err = h.btcProcessedTxStore.GetByIcyTransactionHash(req.ICYTransactionHash)
	if err == nil {
		// Transaction already processed
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, nil, nil, "ICY transaction has already been processed for BTC"))
		return
	}

	icyAmount := &model.Web3BigInt{
		Value:   req.ICYAmount,
		Decimal: 18,
	}

	// Get latest price to calculate BTC amount
	latestPrice, err := h.controller.ConfirmLatestPrice()
	if err != nil {
		h.logger.Error("[TriggerSwap][GetRealtimeICYBTC]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get latest price"))
		return
	}

	// Calculate BTC amount based on ICY amount and latest price using BigInt operations
	icyAmount.Decimal = 18 // Ensure consistent decimal precision
	latestPrice.Decimal = 18

	// Multiply ICY amount by 10^18 to preserve precision
	icyAmountBig := new(big.Int)
	icyAmountBig.SetString(icyAmount.Value, 10)

	priceAmountBig := new(big.Int)
	priceAmountBig.SetString(latestPrice.Value, 10)

	// Perform division with high precision
	btcAmountBig := new(big.Int).Div(icyAmountBig, priceAmountBig)

	btcAmount := &model.Web3BigInt{
		Value:   btcAmountBig.String(),
		Decimal: consts.BTC_DECIMALS, // Standard BTC decimals
	}

	// Check if this ICY transaction has already been processed for BTC
	existingProcessedTx, err := h.btcProcessedTxStore.GetByIcyTransactionHash(req.ICYTransactionHash)
	if existingProcessedTx != nil {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, nil, nil, "ICY transaction has already been processed for BTC"))
		return
	}

	// Begin a transaction to ensure atomicity
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// trigger swap if ICY burn is successful
	btcTxHash, err := h.controller.TriggerSwap(req.ICYTransactionHash, btcAmount, req.BTCAddress)
	if err != nil {
		tx.Rollback()
		h.logger.Error("[TriggerSwap][TriggerSwap]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to trigger swap"))
		return
	}

	// Record BTC transaction processing
	_, err = h.btcProcessedTxStore.Create(&model.OnchainBtcProcessedTransaction{
		IcyTransactionHash: req.ICYTransactionHash,
		BtcTransactionHash: btcTxHash,
		ProcessedAt:        time.Now(),
		Amount:             btcAmount.Value,
		Status:             model.BtcProcessingStatusCompleted,
	})
	if err != nil {
		tx.Rollback()
		h.logger.Error("[TriggerSwap][CreateBtcProcessedTransaction]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to record BTC transaction"))
		return
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		h.logger.Error("[TriggerSwap][CommitTransaction]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to commit transaction"))
		return
	}

	c.JSON(http.StatusOK, view.CreateResponse[any]("Swap triggered successfully", nil, nil, ""))
}
