package blockstream

import "github.com/dwarvesf/icy-backend/internal/model"

type IBlockStream interface {
	BroadcastTx(txHex string) (hash string, err error)
	EstimateFees() (fees map[string]float64, err error)
	GetUTXOs(address string) ([]UTXO, error)
	GetBTCBalance(address string) (balance *model.Web3BigInt, err error)
}
