package swap

import "github.com/gin-gonic/gin"

type IHandler interface {
	CreateSwapRequest(c *gin.Context)
	GenerateSignature(c *gin.Context)
	Info(c *gin.Context)
}
