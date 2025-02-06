package swap

import (
	"math/big"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/dwarvesf/icy-backend/internal/consts"
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

	// trigger swap if ICY burn is successful
	err = h.controller.TriggerSwap(icyAmount, btcAmount, req.BTCAddress)
	if err != nil {
		h.logger.Error("[TriggerSwap][TriggerSwap]", map[string]string{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, nil, "failed to trigger swap"))
		return
	}

	c.JSON(http.StatusOK, view.CreateResponse[any]("Swap triggered successfully", nil, nil, ""))
}
