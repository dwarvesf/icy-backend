package btcrpc

type ChainStats struct {
	FundedTxoCount int `json:"funded_txo_count"`
	FundedTxoSum   int `json:"funded_txo_sum"`
	SpentTxoCount  int `json:"spent_txo_count"`
	SpentTxoSum    int `json:"spent_txo_sum"`
	TxCount        int `json:"tx_count"`
}

type MempoolStats struct {
	FundedTxoCount int `json:"funded_txo_count"`
	FundedTxoSum   int `json:"funded_txo_sum"`
	SpentTxoCount  int `json:"spent_txo_count"`
	SpentTxoSum    int `json:"spent_txo_sum"`
	TxCount        int `json:"tx_count"`
}

type GetBalanceResponse struct {
	Address      string       `json:"address"`
	ChainStats   ChainStats   `json:"chain_stats"`
	MempoolStats MempoolStats `json:"mempool_stats"`
}
