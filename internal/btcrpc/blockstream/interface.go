package blockstream

type IBlockStream interface {
	BroadcastTx(txHex string) (hash string, err error)
	GetFeeEstimates() (map[string]float64, error)
	GetUTXOs(address string) ([]UTXO, error)
}
