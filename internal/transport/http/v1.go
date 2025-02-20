package http

import (
	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/handler"
)

func loadV1Routes(r *gin.Engine, h *handler.Handler) {
	v1 := r.Group("/api/v1")

	// Oracle routes (require API key)
	oracle := v1.Group("/oracle")
	{
		oracle.GET("/circulated-icy", h.OracleHandler.GetCirculatedICY)
		oracle.GET("/treasury-btc", h.OracleHandler.GetTreasusyBTC)
		oracle.GET("/icy-btc-ratio", h.OracleHandler.GetICYBTCRatio)
		oracle.GET("/icy-btc-ratio-cached", h.OracleHandler.GetICYBTCRatioCached)
	}

	// Swap routes (require API key)
	swap := v1.Group("/swap")
	{
		swap.POST("/generate-signature", h.SwapHandler.GenerateSignature)
		swap.POST("", h.SwapHandler.CreateSwapRequest)
	}

	transactions := v1.Group("/transactions")
	{
		transactions.GET("", h.TransactionHandler.GetTransactions)
	}

	// health check (no API key required)
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "ok",
		})
	})
}
