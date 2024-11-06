package http

import (
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func setupCORS(r *gin.Engine, cfg *config.AppConfig) {
	corsOrigins := strings.Split(cfg.ApiServer.AllowedOrigins, ";")
	r.Use(func(c *gin.Context) {
		cors.New(
			cors.Config{
				AllowOrigins: corsOrigins,
				AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
				AllowHeaders: []string{
					"Origin", "Host", "Content-Type", "Content-Length", "Accept-Encoding", "Accept-Language", "Accept",
					"X-CSRF-Token", "Authorization", "X-Requested-With", "X-Access-Token",
				},
				AllowCredentials: true,
			},
		)(c)
	})
}

func NewHttpServer(appConfig *config.AppConfig, logger *logger.Logger) *gin.Engine {
	r := gin.New()
	r.Use(
		gin.LoggerWithWriter(gin.DefaultWriter, "/healthz"),
		gin.Recovery(),
	)
	setupCORS(r, appConfig)

	// load api
	loadV1Routes(r, appConfig, logger)

	return r
}
