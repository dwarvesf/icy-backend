package oracle

import (
	"net/http"
	"time"

	_ "github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
	"github.com/gin-gonic/gin"
)

type handler struct {
	oracle         oracle.IOracle
	logger         *logger.Logger
	appConfig      *config.AppConfig
	metricsRecorder *monitoring.BusinessMetricsRecorder
}

func New(oracle oracle.IOracle, logger *logger.Logger, appConfig *config.AppConfig, metricsRecorder *monitoring.BusinessMetricsRecorder) *handler {
	return &handler{
		oracle:          oracle,
		logger:          logger,
		appConfig:       appConfig,
		metricsRecorder: metricsRecorder,
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
	start := time.Now()
	
	circulatedICY, err := h.oracle.GetCirculatedICY()
	duration := time.Since(start).Seconds()
	
	if err != nil {
		h.logger.Error(err.Error())
		if h.metricsRecorder != nil {
			h.metricsRecorder.RecordOracleOperation("circulated_icy", "error", duration)
		}
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get circulated ICY"))
		return
	}
	
	if h.metricsRecorder != nil {
		h.metricsRecorder.RecordOracleOperation("circulated_icy", "success", duration)
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
	start := time.Now()
	
	treasuryBTC, err := h.oracle.GetBTCSupply()
	duration := time.Since(start).Seconds()
	
	if err != nil {
		h.logger.Error(err.Error())
		if h.metricsRecorder != nil {
			h.metricsRecorder.RecordOracleOperation("treasury_btc", "error", duration)
		}
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get treasury BTC"))
		return
	}
	
	if h.metricsRecorder != nil {
		h.metricsRecorder.RecordOracleOperation("treasury_btc", "success", duration)
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
	start := time.Now()
	
	realtimeICYBTC, err := h.oracle.GetRealtimeICYBTC()
	duration := time.Since(start).Seconds()
	
	if err != nil {
		h.logger.Error(err.Error())
		if h.metricsRecorder != nil {
			h.metricsRecorder.RecordOracleOperation("icy_btc_ratio_realtime", "error", duration)
		}
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get realtime ICY/BTC price"))
		return
	}
	
	if h.metricsRecorder != nil {
		h.metricsRecorder.RecordOracleOperation("icy_btc_ratio_realtime", "success", duration)
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
	start := time.Now()
	
	cachedRealtimeICYBTC, err := h.oracle.GetCachedRealtimeICYBTC()
	duration := time.Since(start).Seconds()
	
	if err != nil {
		h.logger.Error(err.Error())
		if h.metricsRecorder != nil {
			h.metricsRecorder.RecordOracleOperation("icy_btc_ratio_cached", "error", duration)
			h.metricsRecorder.RecordCacheOperation("oracle_price", "miss")
		}
		c.JSON(http.StatusInternalServerError, view.CreateResponse[any](nil, err, "", "can't get cached ICY/BTC price"))
		return
	}
	
	if h.metricsRecorder != nil {
		h.metricsRecorder.RecordOracleOperation("icy_btc_ratio_cached", "success", duration)
		// Fast response indicates cache hit (< 10ms typically means cached)
		if duration < 0.01 {
			h.metricsRecorder.RecordCacheOperation("oracle_price", "hit")
		} else {
			h.metricsRecorder.RecordCacheOperation("oracle_price", "miss")
		}
	}
	c.JSON(http.StatusOK, view.CreateResponse[any](cachedRealtimeICYBTC, nil, "", ""))
	return
}
