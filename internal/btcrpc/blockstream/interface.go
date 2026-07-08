package blockstream

import (
	"errors"

	"github.com/dwarvesf/icy-backend/internal/model"
)

// ErrTxAlreadyKnown signals that the node has ALREADY accepted this transaction
// (it is in the mempool or a block). It is a SUCCESS for settlement purposes: the
// BTC has left, so the caller must NOT rebuild-and-resend. BroadcastTx returns it
// (with an empty txid, since the node's error body carries no txid) so the caller
// can supply the real txid from the tx it already holds.
var ErrTxAlreadyKnown = errors.New("transaction already known to the node")

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
