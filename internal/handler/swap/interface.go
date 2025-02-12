package swap

import "github.com/gin-gonic/gin"

type IHandler interface {
	TriggerSwap(c *gin.Context)
}
