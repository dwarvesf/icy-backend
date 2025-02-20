package onchainbtcprocessedtransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type ListFilter struct {
	Limit      int
	Offset     int
	BTCAddress string
	Status     string
}

type IStore interface {
	// Create a new BTC processed transaction record
	Create(tx *gorm.DB, btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error)

	// Check if an ICY transaction has already been processed
	GetByIcyTransactionHash(tx *gorm.DB, icyTxHash string) (*model.OnchainBtcProcessedTransaction, error)

	// Update the status of a BTC processed transaction
	UpdateStatus(tx *gorm.DB, id int, status model.BtcProcessingStatus) error

	// UpdateToCompleted updates the status of a BTC processed transaction to processed
	UpdateToCompleted(tx *gorm.DB, id int, btcTxHash string) error

	// Get all pending BTC processed transactions
	GetPendingTransactions(tx *gorm.DB) ([]model.OnchainBtcProcessedTransaction, error)

	List(db *gorm.DB, filter ListFilter) ([]*model.OnchainBtcProcessedTransaction, int64, error)
}
