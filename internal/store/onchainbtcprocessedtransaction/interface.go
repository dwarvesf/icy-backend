package onchainbtcprocessedtransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IStore interface {
	// Create a new BTC processed transaction record
	Create(tx *gorm.DB, btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error)

	// Check if an ICY transaction has already been processed
	GetByIcyTransactionHash(tx *gorm.DB, icyTxHash string) (*model.OnchainBtcProcessedTransaction, error)

	// Update the status of a BTC processed transaction
	UpdateStatus(id int, status model.BtcProcessingStatus) error

	// UpdateToCompleted updates the status of a BTC processed transaction to processed
	UpdateToCompleted(id int, btcTxHash string) error

	// Get all pending BTC processed transactions
	GetPendingTransactions() ([]model.OnchainBtcProcessedTransaction, error)
}
