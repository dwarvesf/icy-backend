package telemetry

import (
	"errors"
	"fmt"
	"slices"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
)

func (t *Telemetry) IndexBtcTransaction() error {
	t.logger.Info("[IndexBtcTransaction] Start indexing BTC transactions...")

	var latestTx *model.OnchainBtcTransaction
	latestTx, err := t.store.OnchainBtcTransaction.GetLatestTransaction(t.db)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.logger.Error("[IndexBtcTransaction][GetLatestTransaction]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
	}

	//TODO: Should add first transaction to db manually.
	txHash := ""
	if latestTx != nil {
		txHash = latestTx.TransactionHash
	}

	t.logger.Info(fmt.Sprintf("[IndexBtcTransaction] Latest BTC transaction: %s", txHash))

	markedTxHash := ""
	txs := []model.OnchainBtcTransaction{}
	for {
		markedTxs, err := t.btcRpc.GetTransactionsByAddress(t.appConfig.Blockchain.BTCTreasuryAddress, markedTxHash)
		if err != nil {
			t.logger.Error("[IndexBtcTransaction][GetTransactionsByAddress]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
		for i, tx := range markedTxs {
			if tx.TransactionHash == txHash {
				markedTxs = markedTxs[:i]
				break
			}
		}
		txs = append(txs, markedTxs...)
		if len(markedTxs) < 25 {
			break
		}
		markedTxHash = markedTxs[len(markedTxs)-1].TransactionHash
	}

	slices.Reverse(txs)

	return store.DoInTx(t.db, func(tx *gorm.DB) error {
		for _, onchainTx := range txs {
			_, err := t.store.OnchainBtcTransaction.Create(tx, &onchainTx)
			if err != nil {
				t.logger.Error("[IndexBtcTransaction][Create]", map[string]string{
					"error": err.Error(),
				})
				return err
			}
			t.logger.Info(fmt.Sprintf("Tx Hash: %s - Amount: %s [%s]", onchainTx.TransactionHash, onchainTx.Amount, onchainTx.Type))
		}
		return nil
	})
}

func (t *Telemetry) GetBtcTransactionByInternalID(internalID string) (*model.OnchainBtcTransaction, error) {
	return t.store.OnchainBtcTransaction.GetByInternalID(t.db, internalID)
}

func (t *Telemetry) ProcessPendingBtcTransactions() error {
	t.logger.Info("[ProcessPendingBtcTransactions] Start processing pending BTC transactions...")

	// Fetch all pending BTC processed transactions
	pendingTxs, err := t.store.OnchainBtcProcessedTransaction.GetPendingTransactions(t.db)
	if err != nil {
		t.logger.Error("[ProcessPendingBtcTransactions][GetPendingTransactions]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	if len(pendingTxs) == 0 {
		t.logger.Info("[ProcessPendingBtcTransactions] No pending transactions found.")
		return nil
	}

	t.logger.Info(fmt.Sprintf("[ProcessPendingBtcTransactions] Found %d pending transactions", len(pendingTxs)))

	for _, pendingTx := range pendingTxs {
		// TODO: Implement actual sending logic based on the existing Send method
		// This is a placeholder and needs to be replaced with actual implementation
		t.logger.Info(fmt.Sprintf("[ProcessPendingBtcTransactions] processing pending transaction: %v",
			pendingTx.ID))

		if pendingTx.BTCAddress == "" || pendingTx.Amount == "" {
			err = t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusFailed)
			if err != nil {
				t.logger.Error("[ProcessPendingBtcTransactions][UpdateStatus]", map[string]string{
					"error": err.Error(),
				})
			}
			continue
		}

		amount := &model.Web3BigInt{
			Value:   pendingTx.Amount,
			Decimal: consts.BTC_DECIMALS,
		}
		tx, networkFee, err := t.btcRpc.Send(pendingTx.BTCAddress, amount)
		if err != nil {
			t.logger.Error("[ProcessPendingBtcTransactions][Send]", map[string]string{
				"error": err.Error(),
			})
			continue
		}

		// update processed transaction
		err = t.store.OnchainBtcProcessedTransaction.UpdateToCompleted(t.db, pendingTx.ID, tx, networkFee)
		if err != nil {
			t.logger.Error("[ProcessPendingBtcTransactions][UpdateToCompleted]", map[string]string{
				"error": err.Error(),
			})
			continue
		}

		t.logger.Info(fmt.Sprintf("[ProcessPendingBtcTransactions] Transaction sent: %s", tx))
	}

	return nil
}
