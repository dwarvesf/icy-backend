package oracle

import (
	"net/http"

	_ "github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
	"github.com/gin-gonic/gin"
)

type handler struct {
	oracle    oracle.IOracle
	logger    *logger.Logger
	appConfig *config.AppConfig
}

func New(oracle oracle.IOracle, logger *logger.Logger, appConfig *config.AppConfig) *handler {
	return &handler{
		oracle:    oracle,
		logger:    logger,
		appConfig: appConfig,
	}
}

// Detail godoc
// @Summary Get Circulated ICY
// @Description Get Circulated ICY
// @id getCirculatedICY
// @Tags Oracle
// @Accept json
// @Produce json
// @Success 200 {object} model.Web3BigInt
// @Failure 500 {object} ErrorResponse
// @Router /oracle/circulated-icy [get]
func (h *handler) GetCirculatedICY(c *gin.Context) {
	circulatedICY, err := h.oracle.GetCirculatedICY()
	if err != nil {
		h.logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get circulated ICY"))
		return
	}
	c.JSON(http.StatusOK, view.CreateResponse[any](circulatedICY, nil, "", ""))
	return
}

// Detail godoc
// @Summary Get Treasury BTC
// @Description Get Treasury BTC
// @id getTreasuryBTC
// @Tags Oracle
// @Accept json
// @Produce json
// @Success 200 {object} model.Web3BigInt
// @Failure 500 {object} ErrorResponse
// @Router /oracle/treasury-btc [get]
func (h *handler) GetTreasusyBTC(c *gin.Context) {
	treasuryBTC, err := h.oracle.GetBTCSupply()
	if err != nil {
		h.logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get treasury BTC"))
		return
	}
	c.JSON(http.StatusOK, view.CreateResponse[any](treasuryBTC, nil, "", ""))
	return
}

// Detail godoc
// @Summary Get ICY/BTC Realtime Price
// @Description Get ICY/BTC Realtime Price
// @id getICYBTCRatio
// @Tags Oracle
// @Accept json
// @Produce json
// @Success 200 {object} model.Web3BigInt
// @Failure 500 {object} ErrorResponse
// @Router /oracle/icy-btc-ratio [get]
func (h *handler) GetICYBTCRatio(c *gin.Context) {
	realtimeICYBTC, err := h.oracle.GetRealtimeICYBTC()
	if err != nil {
		h.logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get realtime ICY/BTC price"))
		return
	}
	c.JSON(http.StatusOK, view.CreateResponse[any](realtimeICYBTC, nil, "", ""))
	return
}

// Detail godoc
// @Summary Get ICY/BTC cached Price
// @Description Get ICY/BTC cached Price
// @id getICYBTCRatioCached
// @Tags Oracle
// @Accept json
// @Produce json
// @Success 200 {object} model.Web3BigInt
// @Failure 500 {object} ErrorResponse
// @Router /oracle/icy-btc-ratio-cached [get]
func (h *handler) GetICYBTCRatioCached(c *gin.Context) {
	cachedRealtimeICYBTC, err := h.oracle.GetCachedRealtimeICYBTC()
	if err != nil {
		h.logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get cached ICY/BTC price"))
		return
	}
	c.JSON(http.StatusOK, view.CreateResponse[any](cachedRealtimeICYBTC, nil, "", ""))
	return
}
