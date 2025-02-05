package http

import (
	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/handler"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func loadV1Routes(r *gin.Engine, h *handler.Handler, appConfig *config.AppConfig, logger *logger.Logger) {
	v1 := r.Group("/api/v1")

	oracle := v1.Group("/oracle")
	{
		oracle.GET("/circulated-icy", h.OracleHandler.GetCirculatedICY)
		oracle.GET("/treasury-btc", h.OracleHandler.GetTreasusyBTC)
		oracle.GET("/icy-btc-ratio", h.OracleHandler.GetICYBTCRatio)
		oracle.GET("/icy-btc-ratio-cached", h.OracleHandler.GetICYBTCRatioCached)
	}

	swap := v1.Group("/swap")
	{
		swap.POST("", h.SwapHandler.TriggerSwap)
	}

	// health check
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "ok",
		})
	})
}
