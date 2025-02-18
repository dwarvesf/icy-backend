package swap

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

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

type handler struct {
	logger              *logger.Logger
	appConfig           *config.AppConfig
	oracle              oracle.IOracle
	db                  *gorm.DB
	btcProcessedTxStore onchainbtcprocessedtransaction.IStore
	swapRequestStore    swaprequest.IStore
}

func New(
	logger *logger.Logger,
	appConfig *config.AppConfig,
	oracle oracle.IOracle,
	db *gorm.DB,
) IHandler {
	return &handler{
		logger:              logger,
		appConfig:           appConfig,
		oracle:              oracle,
		db:                  db,
		btcProcessedTxStore: onchainbtcprocessedtransaction.New(),
		swapRequestStore:    swaprequest.New(),
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
func (h *handler) CreateSwapRequest(c *gin.Context) {
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

	icyAmountFloat, err := strconv.ParseFloat(req.ICYAmount, 64)
	if err != nil {
		h.logger.Error("[TriggerSwap][ParseFloat]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid ICY amount"))
		return
	}
	if icyAmountFloat < h.appConfig.MinIcySwapAmount {
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, fmt.Errorf("minimum ICY amount is %v", h.appConfig.MinIcySwapAmount), nil, "invalid ICY amount"))
		return
	}

	// Check if the ICY transaction has already been exisiting
	existingTx, err := h.btcProcessedTxStore.GetByIcyTransactionHash(h.db, req.IcyTx)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		h.logger.Error("[TriggerSwap][CheckICYTransaction]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to check ICY transaction"))
		return
	}

	if existingTx != nil {
		h.logger.Error("[TriggerSwap][DuplicateICYTransaction]", map[string]string{
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
		h.logger.Error("[TriggerSwap][CreateSwapRequest]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to create swap request"))
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

	c.JSON(http.StatusOK, view.CreateResponse[any]("success", nil, nil, "swap request created successfully"))
}
