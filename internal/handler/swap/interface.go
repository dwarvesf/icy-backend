package swap

import "github.com/gin-gonic/gin"

type IHandler interface {
	GenerateSignature(c *gin.Context)
	Info(c *gin.Context)
}
