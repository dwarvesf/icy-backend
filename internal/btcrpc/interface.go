package btcrpc

import "github.com/dwarvesf/icy-backend/internal/model"

type IBtcRpc interface {
	Send(receiverAddress string, amount *model.Web3BigInt) (string, int64, error)
	CurrentBalance() (*model.Web3BigInt, error)
	GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error)
	EstimateFees() (map[string]float64, error)
	GetSatoshiUSDPrice() (float64, error)
	IsDust(address string, amount int64) bool
	// GetTransactionConfirmations returns the number of on-chain confirmations for
	// txHash (0 if unconfirmed / not yet mined / not found). The confirm-before-
	// complete sweep uses it to decide when an outgoing payout is safe to mark
	// completed, and to detect a stuck (never-confirming) send.
	GetTransactionConfirmations(txHash string) (int64, error)
}
