package model

import (
	"time"
)

type BtcProcessingStatus string

const (
	BtcProcessingStatusPending    BtcProcessingStatus = "pending"
	BtcProcessingStatusProcessing BtcProcessingStatus = "processing"
	// BtcProcessingStatusBroadcasted is the intermediate state between a
	// successful broadcast and on-chain confirmation (confirm-before-complete).
	// The signed tx is live (btc_transaction_hash + network_fee are recorded and
	// processed_at marks the broadcast time), but it has NOT yet reached
	// MinBtcConfirmations, so it is NOT completed. GetPendingTransactions never
	// re-picks a broadcasted row, so it is never re-broadcast (no double-send); a
	// crash while in this state simply leaves it for the next confirmation sweep.
	// The confirmation sweep promotes it to "completed" once it is deep enough, or
	// to "needs_reconcile" if it never confirms (stuck / too-low fee).
	BtcProcessingStatusBroadcasted BtcProcessingStatus = "broadcasted"
	BtcProcessingStatusCompleted   BtcProcessingStatus = "completed"
	BtcProcessingStatusFailed      BtcProcessingStatus = "failed"
	// BtcProcessingStatusNeedsReconcile is a TERMINAL state for a row whose
	// broadcast outcome is AMBIGUOUS: Send returned an error at or after the
	// POST, so the signed tx may already be live (mempool / on the wire). Such a
	// row is never auto-released back to pending (that would risk a double-send),
	// it waits for manual/automated reconciliation. GetPendingTransactions never
	// picks it up. Contrast with "failed" (definitely-not-sent, safe) and
	// "processing" (claimed / crash-stranded).
	BtcProcessingStatusNeedsReconcile BtcProcessingStatus = "needs_reconcile"
)

type OnchainBtcProcessedTransaction struct {
	ID                        int                       `json:"id"`
	IcyTransactionHash        *string                   `json:"icy_transaction_hash"`
	BtcTransactionHash        string                    `json:"btc_transaction_hash"`
	SwapTransactionHash       string                    `json:"swap_transaction_hash"`
	BTCAddress                string                    `json:"btc_address"`
	ProcessedAt               *time.Time                `json:"processed_at"`
	Subtotal                  string                    `json:"subtotal"`
	Total                     string                    `json:"total"`
	Status                    BtcProcessingStatus       `json:"status"`
	OnchainIcySwapTransaction OnchainIcySwapTransaction `gorm:"foreignKey:TransactionHash;references:SwapTransactionHash" json:"icy_swap_tx"`
	CreatedAt                 time.Time                 `json:"created_at"`
	UpdatedAt                 time.Time                 `json:"updated_at"`
	NetworkFee                string                    `gorm:"column:network_fee" json:"network_fee"`
	ServiceFee                string                    `gorm:"column:service_fee" json:"service_fee"`
}
