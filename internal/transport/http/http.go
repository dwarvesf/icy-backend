package http

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"     // swagger embed files
	ginSwagger "github.com/swaggo/gin-swagger" // gin-swagger middleware
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/handler"
	"github.com/dwarvesf/icy-backend/internal/oracle"
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

func apiKeyMiddleware(appConfig *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if appConfig.ApiServer.AppEnv != "prod" && appConfig.ApiServer.AppEnv != "production" {
			c.Next()
			return
		}

		// Skip API key check for health check, swagger routes, and transactions routes
		if c.Request.URL.Path == "/healthz" ||
			strings.HasPrefix(c.Request.URL.Path, "/swagger") ||
			strings.HasPrefix(c.Request.URL.Path, "/api/v1/transactions") ||
			strings.HasPrefix(c.Request.URL.Path, "/api/v1/swap/generate-signature") {
			c.Next()
			return
		}

		// Check Authorization header
		apiKey := c.GetHeader("Authorization")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing API key"})
			c.Abort()
			return
		}

		// Remove "ApiKey " prefix if present
		if strings.HasPrefix(apiKey, "ApiKey ") {
			apiKey = strings.TrimPrefix(apiKey, "ApiKey ")
		}

		// Compare with configured API key
		if apiKey != appConfig.ApiServer.ApiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func NewHttpServer(appConfig *config.AppConfig, logger *logger.Logger, oracle oracle.IOracle, baseRPC baserpc.IBaseRPC, db *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(
		gin.LoggerWithWriter(gin.DefaultWriter, "/healthz"),
		gin.Recovery(),
	)
	setupCORS(r, appConfig)

	// Add API key middleware
	r.Use(apiKeyMiddleware(appConfig))

	h := handler.New(appConfig, logger, oracle, baseRPC, db)

	// use ginSwagger middleware to serve the API docs
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// load api
	loadV1Routes(r, h)

	return r
}
