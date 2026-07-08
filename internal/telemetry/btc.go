package telemetry

import (
	"errors"
	"fmt"
	"slices"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
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
		t.logger.Info(fmt.Sprintf("[ProcessPendingBtcTransactions] processing pending transaction: %v",
			pendingTx.ID))

		// Atomic claim: flip this row pending -> processing in one conditional
		// UPDATE. Only the caller that wins the row (claimed == true) may
		// broadcast. If another cron cycle already claimed/sent it, or it is no
		// longer pending, we skip WITHOUT sending. This is the exactly-once
		// gate: it makes a lock-free SELECT-then-send double-spend impossible.
		claimed, err := t.store.OnchainBtcProcessedTransaction.ClaimPendingTransaction(t.db, pendingTx.ID)
		if err != nil {
			t.logger.Error("[ProcessPendingBtcTransactions][ClaimPendingTransaction]", map[string]string{
				"error": err.Error(),
				"id":    fmt.Sprintf("%d", pendingTx.ID),
			})
			continue
		}
		if !claimed {
			// Lost the race (or already advanced past pending). Do NOT broadcast.
			t.logger.Info(fmt.Sprintf("[ProcessPendingBtcTransactions] transaction %d already claimed, skipping", pendingTx.ID))
			continue
		}

		if pendingTx.BTCAddress == "" || pendingTx.Subtotal == "" {
			// Terminal: unpayable row. Mark failed so it is not re-claimed.
			err = t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusFailed)
			if err != nil {
				t.logger.Error("[ProcessPendingBtcTransactions][UpdateStatus]", map[string]string{
					"error": err.Error(),
				})
			}
			continue
		}

		amount := &model.Web3BigInt{
			Value:   pendingTx.Subtotal,
			Decimal: consts.BTC_DECIMALS,
		}
		svcFee := &model.Web3BigInt{
			Value:   pendingTx.ServiceFee,
			Decimal: consts.BTC_DECIMALS,
		}
		amount = amount.Sub(svcFee)
		// Negative-amount guard. The old code checked amount.Decimal (the fixed
		// BTC_DECIMALS constant, never < 0) instead of the numeric VALUE, so it
		// never actually fired. Check the parsed value so a subtotal below the
		// service fee is rejected. Terminal (mark failed): a negative amount can
		// never become sendable, and the row is already claimed to processing.
		amtInt, ok := amount.Int64()
		if !ok || amtInt < 0 {
			t.logger.Error("[ProcessPendingBtcTransactions]", map[string]string{
				"error":  "Amount is negative or unparseable",
				"id":     fmt.Sprintf("%d", pendingTx.ID),
				"amount": amount.Value,
			})
			if uerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusFailed); uerr != nil {
				t.logger.Error("[ProcessPendingBtcTransactions][UpdateStatus]", map[string]string{
					"error": uerr.Error(),
				})
			}
			continue
		}
		// Only log sending details when not hitting circuit breaker repeatedly
		tx, networkFee, err := t.btcRpc.Send(pendingTx.BTCAddress, amount)
		if err != nil {
			// CONDITIONAL release. The failure mode decides whether it is safe to
			// retry. This is the double-send fix: releasing on EVERY error let the
			// next cron tick rebuild-and-resend a tx that Send only FAILED TO
			// CONFIRM, not failed to broadcast (lost response / read timeout / 5xx
			// after the node enqueued it), sending the BTC twice.
			if errors.Is(err, btcrpc.ErrNotBroadcast) {
				// DEFINITELY not on the wire (pre-POST error, or a clean
				// min-relay-fee rejection). No BTC left the treasury: release the
				// claim (processing -> pending) so a healthy later cycle retries.
				if rerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusPending); rerr != nil {
					t.logger.Error("[ProcessPendingBtcTransactions][ReleaseClaim]", map[string]string{
						"error": rerr.Error(),
						"id":    fmt.Sprintf("%d", pendingTx.ID),
					})
				}
			} else {
				// AMBIGUOUS: a POST was attempted and its outcome is unknown, the
				// signed tx may already be live. NEVER release to pending (that
				// risks a double-send). Move to the terminal needs_reconcile state
				// so it is never auto-re-sent; a human/automated reconcile inspects
				// the treasury's on-chain history to settle it. Fail-safe: the cost
				// is liveness (a stranded row), never a double payout.
				if rerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusNeedsReconcile); rerr != nil {
					t.logger.Error("[ProcessPendingBtcTransactions][NeedsReconcile]", map[string]string{
						"error": rerr.Error(),
						"id":    fmt.Sprintf("%d", pendingTx.ID),
					})
				}
				t.logger.Error("[ProcessPendingBtcTransactions][Send][AMBIGUOUS] left as needs_reconcile, NOT re-sent", map[string]string{
					"error":       err.Error(),
					"id":          fmt.Sprintf("%d", pendingTx.ID),
					"btc_address": pendingTx.BTCAddress,
					"amount":      amount.Value,
				})
				continue
			}
			// Only log circuit breaker errors occasionally to reduce spam
			if err.Error() != "circuit breaker is open" || pendingTx.ID%10 == 0 {
				t.logger.Error("[ProcessPendingBtcTransactions][Send]", map[string]string{
					"error":       err.Error(),
					"btc_address": pendingTx.BTCAddress,
					"amount":      amount.Value,
				})
			}
			continue
		}

		// Broadcast SUCCEEDED. A crash between here and UpdateToCompleted leaves
		// the row in "processing" (never pending again), so it is never
		// re-broadcast: exactly-once holds even across a mid-payout crash. The
		// worst case is a stranded processing row for manual reconciliation,
		// which is the safe failure direction (never a double-send).

		// Log successful sends
		t.logger.Info("[ProcessPendingBtcTransactions] BTC sent successfully", map[string]string{
			"btc_address": pendingTx.BTCAddress,
			"amount":      amount.Value,
			"tx":          tx,
		})

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
