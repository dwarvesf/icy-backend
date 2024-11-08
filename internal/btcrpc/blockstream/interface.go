package blockstream

type IBlockStream interface {
	BroadcastTx(txHex string) (hash string, err error)
	EstimateFees() (fees map[string]float64, err error)
	GetUTXOs(address string) ([]UTXO, error)
}
