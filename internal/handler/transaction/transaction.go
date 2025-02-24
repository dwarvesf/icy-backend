package transaction

import (
	"fmt"
	"net/http"
	"strconv"

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
	// Manually parse query parameters
	limit := parseIntParam(c, "limit", 5)
	offset := parseIntParam(c, "offset", 0)
	btcAddress := c.Query("btc_address")
	evmAddress := c.Query("evm_address")
	status := c.Query("status")

	// Log parsed parameters for debugging
	fmt.Printf("Parsed params: limit=%d, offset=%d, btc_address=%s, status=%s\n",
		limit, offset, btcAddress, status)

	// Validate pagination
	if limit > 100 {
		limit = 100
	}

	// Prepare filter conditions
	filter := onchainbtcprocessedtransaction.ListFilter{
		Limit:      limit,
		Offset:     offset,
		BTCAddress: btcAddress,
		EVMAddress: evmAddress,
		Status:     status,
	}

	// Fetch transactions
	transactions, total, err := h.onchainbtcprocessedtransaction.Find(h.db, filter)
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

// parseIntParam parses an integer query parameter with a default value
func parseIntParam(c *gin.Context, key string, defaultValue int) int {
	strValue := c.Query(key)
	if strValue == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(strValue)
	if err != nil {
		return defaultValue
	}

	return value
}
