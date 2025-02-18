package blockstream

import "github.com/dwarvesf/icy-backend/internal/model"

// BroadcastTxError represents a detailed error when broadcasting a transaction
type BroadcastTxError struct {
	Message    string
	StatusCode int
	MinFee     int64 // Minimum fee required in satoshis
}

// Error implements the error interface
func (e *BroadcastTxError) Error() string {
	return e.Message
}

type IBlockStream interface {
	BroadcastTx(txHex string) (hash string, err error)
	EstimateFees() (fees map[string]float64, err error)
	GetUTXOs(address string) ([]UTXO, error)
	GetBTCBalance(address string) (balance *model.Web3BigInt, err error)
	GetTransactionsByAddress(address string, fromTxID string) ([]Transaction, error)
}
