package btcrpc

import (
	"fmt"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
)

const (
	p2wpkhInputSize  = 68 // SegWit P2WPKH input size
	p2wpkhOutputSize = 31 // SegWit P2WPKH output size
	txOverhead       = 10 // Transaction overhead
)

// CalculateTxFee estimates the transaction fee based on current network conditions
func (b *BtcRpc) CalculateTxFee(numInputs, numOutputs, targetBlocks int) (int64, error) {

	// Get current fee rate from mempool
	estimates, err := b.blockstream.GetFeeEstimates()
	if err != nil {
		return 0, err
	}

	// Get fee rate for target blocks
	target := fmt.Sprintf("%d", targetBlocks)
	feeRate, ok := estimates[target]
	if !ok {
		return 0, fmt.Errorf("no fee rate available for target %d blocks", targetBlocks)
	}

	// Calculate total transaction size
	txSize := calculateTxSize(numInputs, numOutputs)

	// Calculate fee in satoshis
	fee := int64(float64(txSize) * feeRate)

	// Return minimum 1000 sats if calculated fee is too low
	if fee < 1000 {
		return 1000, nil
	}

	return fee, nil
}

// calculateTxSize calculates the total transaction size in bytes
func calculateTxSize(numInputs, numOutputs int) int {
	return txOverhead + (numInputs * p2wpkhInputSize) + (numOutputs * p2wpkhOutputSize)
}

// GetLatestUTXO returns the most recent unspent transaction output for an address
func (b *BtcRpc) GetLatestUTXO(address string) (*blockstream.UTXO, error) {
	utxos, err := b.blockstream.GetUTXOs(address)
	if err != nil {
		return nil, err
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("no UTXOs found for address")
	}

	// Return the first confirmed UTXO
	for _, utxo := range utxos {
		if utxo.Status.Confirmed {
			return &utxo, nil
		}
	}

	return nil, fmt.Errorf("no confirmed UTXOs found")
}
