package transaction

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
)

type transactionHandler struct {
	db                             *gorm.DB
	onchainbtcprocessedtransaction onchainbtcprocessedtransaction.IStore
}

// NewTransactionHandler creates a new instance of TransactionHandler
func NewTransactionHandler(
	db *gorm.DB,
	onchainbtcprocessedtransaction onchainbtcprocessedtransaction.IStore,
) IHandler {
	return &transactionHandler{
		db:                             db,
		onchainbtcprocessedtransaction: onchainbtcprocessedtransaction,
	}
}

// GetTransactions retrieves onchain processed transactions with optional filtering
func (h *transactionHandler) GetTransactions(c *gin.Context) {
	// Parse request parameters
	var req GetTransactionsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate and set default pagination
	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	// Prepare filter conditions
	filter := onchainbtcprocessedtransaction.ListFilter{
		Limit:     req.Limit,
		Offset:    req.Offset,
		FromAddr:  req.FromAddr,
		ToAddr:    req.ToAddr,
		TxType:    req.TxType,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	}

	// Fetch transactions
	transactions, total, err := h.onchainbtcprocessedtransaction.List(h.db, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch transactions"})
		return
	}

	// Prepare response
	c.JSON(http.StatusOK, GetTransactionsResponse{
		Total:        total,
		Transactions: transactions,
	})
}
