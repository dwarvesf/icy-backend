package transaction

import (
	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/model"
)

// TransactionHandler defines methods for handling transaction-related operations

type IHandler interface {
	// GetTransactions retrieves onchain processed transactions with optional filtering
	GetTransactions(c *gin.Context)
}

// GetTransactionsRequest represents the parameters for fetching transactions
type GetTransactionsRequest struct {
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	FromAddr  string `json:"from_addr"`
	ToAddr    string `json:"to_addr"`
	TxType    string `json:"tx_type"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

// GetTransactionsResponse contains the list of transactions and total count
type GetTransactionsResponse struct {
	Total        int64                                   `json:"total"`
	Transactions []*model.OnchainBtcProcessedTransaction `json:"data"`
}
