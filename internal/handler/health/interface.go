package health

import "github.com/gin-gonic/gin"

// IHealthHandler defines the interface for health check handlers
type IHealthHandler interface {
	Basic(c *gin.Context)
	Database(c *gin.Context)
	External(c *gin.Context)
	Jobs(c *gin.Context)
}