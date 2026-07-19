package http

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	swaggerFiles "github.com/swaggo/files"     // swagger embed files
	ginSwagger "github.com/swaggo/gin-swagger" // gin-swagger middleware
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/handler"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func setupCORS(r *gin.Engine, cfg *config.AppConfig) {
	corsOrigins := strings.Split(cfg.ApiServer.AllowedOrigins, ";")

	// A wildcard origin and credentialed CORS are mutually exclusive under the
	// Fetch spec: a browser will not expose a credentialed response to "*", and
	// emitting both Access-Control-Allow-Origin:* and Allow-Credentials:true is a
	// misconfiguration. If any configured origin is "*", drop credentials so the
	// two can never be sent together. Prod should still narrow AllowedOrigins to
	// the real front-ends (icy.so, icy.d.foundation) rather than rely on "*".
	allowCredentials := true
	for _, o := range corsOrigins {
		if strings.TrimSpace(o) == "*" {
			allowCredentials = false
			break
		}
	}

	// Build the middleware once and register it, instead of allocating a new
	// cors handler on every request.
	r.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins,
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
		AllowHeaders: []string{
			"Origin", "Host", "Content-Type", "Content-Length", "Accept-Encoding", "Accept-Language", "Accept",
			"X-CSRF-Token", "Authorization", "X-Requested-With", "X-Access-Token",
		},
		AllowCredentials: allowCredentials,
	}))
}

// recognizedNonProdEnvs is the allowlist of APP_ENV values that may bypass the
// api-key gate. Anything NOT in this set, including an empty or misspelled
// APP_ENV, is treated as production (fail-closed), so a misconfigured
// environment can never silently open the treasury-draining endpoints.
var recognizedNonProdEnvs = map[string]bool{
	"dev":         true,
	"development": true,
	"local":       true,
	"staging":     true,
	"test":        true,
}

func apiKeyMiddleware(appConfig *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recognizedNonProdEnvs[appConfig.ApiServer.AppEnv] {
			c.Next()
			return
		}

		// Skip API key check for health check, swagger routes, metrics, and transactions routes
		if c.Request.URL.Path == "/healthz" ||
			c.Request.URL.Path == "/metrics" ||
			strings.HasPrefix(c.Request.URL.Path, "/swagger") ||
			strings.HasPrefix(c.Request.URL.Path, "/api/v1/health") ||
			strings.HasPrefix(c.Request.URL.Path, "/api/v1/swap/info") ||
			strings.HasPrefix(c.Request.URL.Path, "/api/v1/transactions") {
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

		// Reject an empty key AFTER trimming the prefix. Without this, a header of
		// "ApiKey " (trailing space) trims to "" and, when the configured ApiKey is
		// also "" (misconfig), a constant-time compare of two empty strings passes.
		// An empty presented key is never valid, regardless of what is configured.
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}

		// Compare with configured API key in constant time to avoid leaking the key
		// via response-timing side channels.
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(appConfig.ApiServer.ApiKey)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func NewHttpServer(appConfig *config.AppConfig, logger *logger.Logger,
	oracle oracle.IOracle, baseRPC baserpc.IBaseRPC, btcRPC btcrpc.IBtcRpc,
	db *gorm.DB) *gin.Engine {

	// Create Prometheus registry and HTTP metrics
	metricsRegistry := prometheus.NewRegistry()
	httpMetrics := monitoring.NewHTTPMetrics()
	httpMetrics.MustRegister(metricsRegistry)

	r := gin.New()
	r.Use(
		gin.LoggerWithWriter(gin.DefaultWriter, "/healthz", "/metrics"),
		gin.Recovery(),
	)
	setupCORS(r, appConfig)

	// Add HTTP metrics middleware
	r.Use(monitoring.HTTPMetricsMiddleware(httpMetrics))

	// Add API key middleware
	r.Use(apiKeyMiddleware(appConfig))

	h := handler.New(appConfig, logger, oracle, baseRPC, btcRPC, db, metricsRegistry)

	// Add metrics endpoint (no API key required)
	r.GET("/metrics", h.MetricsHandler.Handler())

	// use ginSwagger middleware to serve the API docs
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// load api
	loadV1Routes(r, h)

	return r
}

func NewHttpServerWithMonitoring(appConfig *config.AppConfig, logger *logger.Logger,
	oracle oracle.IOracle, baseRPC baserpc.IBaseRPC, btcRPC btcrpc.IBtcRpc,
	db *gorm.DB, jobStatusManager *monitoring.JobStatusManager,
	externalAPIMetrics *monitoring.ExternalAPIMetrics,
	backgroundJobMetrics *monitoring.BackgroundJobMetrics) *gin.Engine {

	// Create Prometheus registry and register all metrics
	metricsRegistry := prometheus.NewRegistry()

	// HTTP metrics
	httpMetrics := monitoring.NewHTTPMetrics()
	httpMetrics.MustRegister(metricsRegistry)

	// External API metrics
	externalAPIMetrics.MustRegister(metricsRegistry)

	// Background job metrics
	backgroundJobMetrics.MustRegister(metricsRegistry)

	r := gin.New()
	r.Use(
		gin.LoggerWithWriter(gin.DefaultWriter, "/healthz", "/metrics"),
		gin.Recovery(),
	)
	setupCORS(r, appConfig)

	// Add HTTP metrics middleware
	r.Use(monitoring.HTTPMetricsMiddleware(httpMetrics))

	// Add API key middleware
	r.Use(apiKeyMiddleware(appConfig))

	h := handler.NewWithMonitoring(appConfig, logger, oracle, baseRPC, btcRPC, db, metricsRegistry, jobStatusManager, httpMetrics)

	// Add metrics endpoint (no API key required)
	r.GET("/metrics", h.MetricsHandler.Handler())

	// use ginSwagger middleware to serve the API docs
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// load api
	loadV1Routes(r, h)

	return r
}
