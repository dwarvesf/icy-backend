package btcrpc

// ChainStats represents the statistics of the blockchain referring to the transactions that have been committed to the blockchain.
type ChainStats struct {
	FundedTxoCount int `json:"funded_txo_count"`
	FundedTxoSum   int `json:"funded_txo_sum"`
	SpentTxoCount  int `json:"spent_txo_count"`
	SpentTxoSum    int `json:"spent_txo_sum"`
	TxCount        int `json:"tx_count"`
}

// MempoolStats represents memory pool referring to the transactions that is in the memory
// of the node but has not been committed to the blockchain in the block yet.
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
