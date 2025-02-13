package model

import "time"

type BtcProcessingStatus string

const (
	BtcProcessingStatusPending   BtcProcessingStatus = "pending"
	BtcProcessingStatusCompleted BtcProcessingStatus = "completed"
	BtcProcessingStatusFailed    BtcProcessingStatus = "failed"
)

type OnchainBtcProcessedTransaction struct {
	ID                  int                 `json:"id"`
	IcyTransactionHash  string              `json:"icy_transaction_hash"`
	BtcTransactionHash  string              `json:"btc_transaction_hash"`
	SwapTransactionHash string              `json:"swap_transaction_hash"`
	BTCAddress          string              `json:"btc_address"`
	ProcessedAt         *time.Time          `json:"processed_at"`
	Amount              string              `json:"amount"`
	Status              BtcProcessingStatus `json:"status"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}
