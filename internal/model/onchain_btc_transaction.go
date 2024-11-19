package model

type OnchainBtcTransaction struct {
	ID                   int             `json:"id"`
	InternalID           string          `json:"internal_id"`
	TransactionHash      string          `json:"transaction_hash"`
	TransactionTimestamp int64           `json:"transaction_timestamp"`
	Type                 TransactionType `json:"type"`
	Amount               string          `json:"amount"`
	SenderAddress        string          `json:"sender_address"`
	ReceiverAddress      string          `json:"receiver_address"`
	CreatedAt            int64           `json:"created_at"`
	UpdatedAt            int64           `json:"updated_at"`
}
