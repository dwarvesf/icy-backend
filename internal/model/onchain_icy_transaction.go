package model

import "time"

type TransactionType string

const (
	Out TransactionType = "out"
	In  TransactionType = "in"
)

type OnchainIcyTransaction struct {
	ID              int             `json:"id"`
	InternalID      string          `json:"internal_id"`
	TransactionHash string          `json:"transaction_hash"`
	BlockTime       int64           `json:"block_time"`
	BlockNumber     uint64          `json:"block_number"`
	Type            TransactionType `json:"type"`
	Amount          string          `json:"amount"`
	Fee             string          `json:"fee"`
	OtherAddress    string          `json:"other_address"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}
