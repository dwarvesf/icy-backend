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
	// GetTransactionConfirmations returns how many confirmations txID has (0 if it
	// is unconfirmed, not yet mined, or not found). confirmations = tipHeight -
	// blockHeight + 1, so a tx in the tip block has 1 confirmation. Used by the
	// confirm-before-complete sweep to decide when an outgoing payout is safe to
	// mark completed.
	GetTransactionConfirmations(txID string) (int64, error)
	// GetTransaction returns the full transaction (including its inputs) for txID,
	// or (nil, nil) if the node does not know it. Used to verify that an
	// unconfirmed treasury UTXO is the treasury's OWN change (the treasury is
	// among the tx inputs) before a payout is allowed to chain onto it.
	GetTransaction(txID string) (*Transaction, error)
}
