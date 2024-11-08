package oracle

import "github.com/gin-gonic/gin"

type IHandler interface {
	GetCirculatedICY(c *gin.Context)
	GetTreasusyBTC(c *gin.Context)
	GetICYBTCRatio(c *gin.Context)
	GetICYBTCRatioCached(c *gin.Context)
}
