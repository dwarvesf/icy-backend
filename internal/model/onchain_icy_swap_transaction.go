package model

import "time"

type OnchainIcySwapTransaction struct {
	ID              int       `json:"id"`
	TransactionHash string    `json:"transaction_hash"`
	BlockNumber     uint64    `json:"block_number"`
	IcyAmount       string    `json:"icy_amount"`
	FromAddress     string    `json:"from_address"`
	BtcAddress      string    `json:"btc_address"`
	BtcAmount       string    `json:"btc_amount"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
