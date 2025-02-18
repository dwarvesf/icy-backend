package model

import "time"

type TransactionType string

const (
	Out      TransactionType = "out"
	In       TransactionType = "in"
	Transfer TransactionType = "transfer"
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
	FromAddress     string          `json:"from_address"`
	ToAddress       string          `json:"to_address"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}
