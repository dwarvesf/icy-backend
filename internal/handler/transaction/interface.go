package transaction

import (
	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IHandler interface {
	// GetTransactions retrieves onchain processed transactions with optional filtering
	GetTransactions(c *gin.Context)
}

type GetTransactionsRequest struct {
	Limit      int    `form:"limit" json:"limit"`
	Offset     int    `form:"offset" json:"offset"`
	BTCAddress string `form:"btc_address" json:"btc_address"`
	Status     string `form:"status" json:"status"`
}

type GetTransactionsResponse struct {
	Total        int64                                   `json:"total"`
	Transactions []*model.OnchainBtcProcessedTransaction `json:"transactions"`
}
