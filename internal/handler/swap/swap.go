package swap

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/controller"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
)

type SwapRequest struct {
	ICYAmount  string `json:"icy_amount" binding:"required"`
	BTCAddress string `json:"btc_address" binding:"required"`
}

type handler struct {
	controller controller.IController
	logger     *logger.Logger
	appConfig  *config.AppConfig
	oracle     oracle.IOracle
}

func New(controller controller.IController, logger *logger.Logger, appConfig *config.AppConfig, oracle oracle.IOracle) IHandler {
	return &handler{
		controller: controller,
		logger:     logger,
		appConfig:  appConfig,
		oracle:     oracle,
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
// @Router /swap/trigger [post]
func (h *handler) TriggerSwap(c *gin.Context) {
	var req SwapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("[TriggerSwap][ShouldBindJSON]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, req, "invalid request"))
		return
	}

	icyAmount := &model.Web3BigInt{
		Value:   req.ICYAmount,
		Decimal: 18,
	}

	// Check if swap amount is less than or equal to circulated ICY
	circulatedICY, err := h.oracle.GetCirculatedICY()
	if err != nil {
		h.logger.Error("[TriggerSwap][GetCirculatedICY]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to get circulated ICY"))
		return
	}

	// Compare swap amount with circulated ICY
	if icyAmount.ToFloat() > circulatedICY.ToFloat() {
		h.logger.Error("[TriggerSwap][InsufficientCirculatedICY]", map[string]string{
			"swapAmount":    icyAmount.Value,
			"circulatedICY": circulatedICY.Value,
		})
		c.JSON(http.StatusBadRequest, view.CreateResponse[any](nil, err, nil, "swap amount exceeds circulated ICY"))
		return
	}

	// TODO: burn ICY before triggering swap, how?

	// check if icy onchain tx burn is successful

	// trigger swap if ICY burn is successful
	err = h.controller.TriggerSwap(icyAmount, req.BTCAddress)
	if err != nil {
		h.logger.Error("[TriggerSwap][TriggerSwap]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to trigger swap"))
		return
	}

	// TODO
	// Add telemetry

	c.JSON(http.StatusOK, view.CreateResponse[any]("Swap triggered successfully", nil, nil, ""))
}
