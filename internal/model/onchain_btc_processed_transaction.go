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
	// BtcProcessingStatusRefused is a TERMINAL state for a payout this service
	// DEFINITIVELY never broadcast, because a policy control refused it (the
	// daily cap, or a cap that could not be evaluated). Distinct from
	// needs_reconcile, which means "a POST was attempted, BTC may be live".
	// Conflating the two made refusals count as outflow: a refusal added its own
	// amount to the rolling 24h total, which pushed the next payout over the cap,
	// which refused and added again. The cap ratcheted itself shut for 24 hours
	// without a satoshi leaving the treasury. Refused rows are excluded from the
	// sent-states sum for exactly that reason.
	BtcProcessingStatusRefused BtcProcessingStatus = "refused"
)

type OnchainBtcProcessedTransaction struct {
	ID                        int                       `json:"id"`
	IcyTransactionHash        *string                   `json:"icy_transaction_hash"`
	BtcTransactionHash        string                    `json:"btc_transaction_hash"`
	SwapTransactionHash       string                    `json:"swap_transaction_hash"`
	BTCAddress                string                    `json:"btc_address"`
	ProcessedAt               *time.Time                `json:"processed_at"`
	// Attempts counts how many times this payout has been released back to
	// pending after a definitely-not-broadcast failure. It is the liveness
	// bound on that retry: without it a permanently-unsendable row re-failed on
	// every settlement tick forever.
	Attempts                  int                       `json:"attempts"`
	Subtotal                  string                    `json:"subtotal"`
	Total                     string                    `json:"total"`
	Status                    BtcProcessingStatus       `json:"status"`
	OnchainIcySwapTransaction OnchainIcySwapTransaction `gorm:"foreignKey:TransactionHash;references:SwapTransactionHash" json:"icy_swap_tx"`
	CreatedAt                 time.Time                 `json:"created_at"`
	UpdatedAt                 time.Time                 `json:"updated_at"`
	NetworkFee                string                    `gorm:"column:network_fee" json:"network_fee"`
	ServiceFee                string                    `gorm:"column:service_fee" json:"service_fee"`
}
