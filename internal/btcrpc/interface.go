package btcrpc

import "github.com/dwarvesf/icy-backend/internal/model"

type IBtcRpc interface {
	Send(receiverAddress string, amount *model.Web3BigInt) (string, int64, error)
	CurrentBalance() (*model.Web3BigInt, error)
	GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error)
	EstimateFees() (map[string]float64, error)
	GetSatoshiUSDPrice() (float64, error)
}
