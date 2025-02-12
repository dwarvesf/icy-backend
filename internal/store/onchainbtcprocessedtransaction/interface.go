package onchainbtcprocessedtransaction

import (
	"github.com/dwarvesf/icy-backend/internal/model"
)

type IStore interface {
	// Create a new BTC processed transaction record
	Create(btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error)

	// Check if an ICY transaction has already been processed
	GetByIcyTransactionHash(icyTxHash string) (*model.OnchainBtcProcessedTransaction, error)

	// Update the status of a BTC processed transaction
	UpdateStatus(id int, status model.BtcProcessingStatus) error
}
