package http

import (
	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func loadV1Routes(r *gin.Engine, appConfig *config.AppConfig, logger *logger.Logger) {
	// health check
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "ok",
		})
	})
}
