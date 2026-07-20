package onchainbtcprocessedtransaction

import (
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type ListFilter struct {
	Limit      int
	Offset     int
	BTCAddress string
	EVMAddress string
	Status     string
}

type IStore interface {
	// Create a new BTC processed transaction record
	Create(tx *gorm.DB, btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error)

	// Check if an ICY transaction has already been processed
	GetByIcyTransactionHash(tx *gorm.DB, icyTxHash string) (*model.OnchainBtcProcessedTransaction, error)

	// GetBySwapTransactionHash returns the payout row for a given on-chain ICY
	// swap tx hash (the populated, DB-unique dedup key). Used by the indexer to
	// avoid minting a second BTC payout for a re-indexed / reorg-re-emitted swap.
	GetBySwapTransactionHash(tx *gorm.DB, swapTxHash string) (*model.OnchainBtcProcessedTransaction, error)

	// ClaimPendingTransaction atomically flips one pending row to "processing".
	// Returns true only for the caller that won the row (RowsAffected == 1);
	// the exactly-once gate that prevents double-spend under overlapping runs.
	ClaimPendingTransaction(tx *gorm.DB, id int) (bool, error)

	// Update the status of a BTC processed transaction
	UpdateStatus(tx *gorm.DB, id int, status model.BtcProcessingStatus) error
	// ReleaseForRetry returns a claimed row to pending and increments its
	// attempt count atomically, returning the new total.
	ReleaseForRetry(tx *gorm.DB, id int) (int, error)

	// UpdateToCompleted updates the status of a BTC processed transaction to processed
	UpdateToCompleted(tx *gorm.DB, id int, btcTxHash string, networkFee int64) error

	// UpdateToBroadcasted records a successful broadcast WITHOUT completing the
	// row (confirm-before-complete): it sets status "broadcasted", stores the
	// btc_transaction_hash + network_fee, and stamps processed_at with the
	// broadcast time. The row waits in this state until the confirmation sweep
	// promotes it to completed (>= MinBtcConfirmations) or needs_reconcile (stuck).
	UpdateToBroadcasted(tx *gorm.DB, id int, btcTxHash string, networkFee int64) error

	// GetBroadcastedTransactions returns every payout that has been broadcast but
	// not yet confirmed on-chain. The confirmation sweep iterates these.
	GetBroadcastedTransactions(tx *gorm.DB) ([]model.OnchainBtcProcessedTransaction, error)

	// Get all pending BTC processed transactions
	GetPendingTransactions(tx *gorm.DB) ([]model.OnchainBtcProcessedTransaction, error)

	// SumSentInWindow returns the total sendable amount (subtotal - service_fee),
	// in satoshi, of every payout that has (or may have) left the treasury since
	// `since`. It is the rolling-24h input for the SG-06 daily payout cap. Fails
	// closed on an unparseable amount so the caller never under-counts.
	SumSentInWindow(tx *gorm.DB, since time.Time) (int64, error)

	Find(db *gorm.DB, filter ListFilter) ([]*model.OnchainBtcProcessedTransaction, int64, error)
}
