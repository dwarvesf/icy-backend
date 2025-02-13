package telemetry

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
)

func (t *Telemetry) IndexIcyTransaction() error {
	t.logger.Info("[IndexIcyTransaction] Start indexing ICY transactions...")

	var latestTx *model.OnchainIcyTransaction
	latestTx, err := t.store.OnchainIcyTransaction.GetLatestTransaction(t.db)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.logger.Error("[IndexIcyTransaction][GetLatestTransaction]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
		t.logger.Info("[IndexIcyTransaction] No previous transactions found. Starting from the beginning.")
	}

	// Determine the starting block
	startBlock := uint64(0)
	if latestTx != nil && latestTx.TransactionHash != "" {
		t.logger.Info(fmt.Sprintf("[IndexIcyTransaction] Latest ICY transaction: %s", latestTx.TransactionHash))
		receipt, err := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(latestTx.TransactionHash))
		if err != nil {
			t.logger.Error("[IndexIcyTransaction][LastTransactionReceipt]", map[string]string{
				"txHash": latestTx.TransactionHash,
				"error":  err.Error(),
			})
		} else {
			startBlock = receipt.BlockNumber.Uint64() + 1
		}
	}

	fromTxId := t.appConfig.Blockchain.InitialICYTransactionHash
	if latestTx != nil {
		fromTxId = latestTx.TransactionHash
	}

	// Fetch all transactions for the ICY contract
	allTxs, err := t.baseRpc.GetTransactionsByAddress(t.appConfig.Blockchain.ICYContractAddr, fromTxId)
	if err != nil {
		t.logger.Error("[IndexIcyTransaction][GetTransactionsByAddress]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Filter and prepare transactions to store
	var txsToStore []model.OnchainIcyTransaction
	for _, tx := range allTxs {
		receipt, err := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(tx.TransactionHash))
		if err != nil {
			t.logger.Error("[IndexIcyTransaction][TransactionReceipt]", map[string]string{
				"txHash": tx.TransactionHash,
				"error":  err.Error(),
			})
			continue
		}

		// Only add transactions after the last known transaction
		if receipt.BlockNumber.Uint64() >= startBlock {
			txsToStore = append(txsToStore, tx)
		}
	}

	// Sort transactions by block number to maintain order
	slices.SortFunc(txsToStore, func(a, b model.OnchainIcyTransaction) int {
		receiptA, _ := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(a.TransactionHash))
		receiptB, _ := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(b.TransactionHash))
		return int(receiptA.BlockNumber.Int64() - receiptB.BlockNumber.Int64())
	})

	// Store transactions
	if len(txsToStore) > 0 {
		err = store.DoInTx(t.db, func(tx *gorm.DB) error {
			for _, onchainTx := range txsToStore {
				_, err := t.store.OnchainIcyTransaction.Create(tx, &onchainTx)
				if err != nil {
					return err
				}
				t.logger.Info(fmt.Sprintf("Tx Hash: %s - Amount: %s [%s]", onchainTx.TransactionHash, onchainTx.Amount, onchainTx.Type))
			}
			return nil
		})
		if err != nil {
			t.logger.Error("[IndexIcyTransaction][CreateTransactions]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
	}

	t.logger.Info(fmt.Sprintf("[IndexIcyTransaction] Processed %d new transactions", len(txsToStore)))
	return nil
}

func (t *Telemetry) GetIcyTransactionByHash(txHash string) (*model.OnchainIcyTransaction, error) {
	return t.store.OnchainIcyTransaction.GetByTransactionHash(t.db, txHash)
}
