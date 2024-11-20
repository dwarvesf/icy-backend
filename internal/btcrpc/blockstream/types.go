package blockstream

type UTXO struct {
	TxID   string `json:"txid"`
	Vout   uint32 `json:"vout"`
	Value  int64  `json:"value"`
	Status struct {
		Confirmed bool `json:"confirmed"`
	} `json:"status"`
}

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

// Transaction represents a Bitcoin transaction from the Esplora API
type Transaction struct {
	TxID     string   `json:"txid"`
	Version  int32    `json:"version"`
	Locktime uint32   `json:"locktime"`
	Size     uint32   `json:"size"`
	Weight   uint32   `json:"weight"`
	Fee      int64    `json:"fee"`
	Vin      []Input  `json:"vin"`
	Vout     []Output `json:"vout"`
	Status   TxStatus `json:"status"`
}

// Input represents a transaction input
type Input struct {
	TxID         string   `json:"txid"`
	Vout         uint32   `json:"vout"`
	Prevout      *Output  `json:"prevout"`
	ScriptSig    string   `json:"scriptsig"`
	ScriptSigAsm string   `json:"scriptsig_asm"`
	Witness      []string `json:"witness"`
	IsCoinbase   bool     `json:"is_coinbase"`
	Sequence     uint32   `json:"sequence"`
}

// Output represents a transaction output
type Output struct {
	ScriptPubKey        string `json:"scriptpubkey"`
	ScriptPubKeyAsm     string `json:"scriptpubkey_asm"`
	ScriptPubKeyType    string `json:"scriptpubkey_type"`
	ScriptPubKeyAddress string `json:"scriptpubkey_address"`
	Value               int64  `json:"value"`
}

// TxStatus represents the status of a transaction
type TxStatus struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight uint32 `json:"block_height,omitempty"`
	BlockHash   string `json:"block_hash,omitempty"`
	BlockTime   int64  `json:"block_time,omitempty"`
}
