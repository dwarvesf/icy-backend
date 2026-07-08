package telemetry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/webhook"
)

// swapPayoutWebhookTimeout bounds the detached goroutine that posts the
// Discord notification (see emitSwapPayoutWebhook). It is intentionally
// separate from the settlement loop's own control flow so a slow/unreachable
// webhook endpoint can never delay or fail a payout state transition.
const swapPayoutWebhookTimeout = 10 * time.Second

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

// emitSwapPayoutWebhook notifies a Discord webhook that a BTC payout reached a
// terminal settlement state (completed / failed / needs_reconcile). This is
// detection, not prevention: it exists so a drain or anomaly is visible
// somewhere other than the in-page browser toast (SG-07).
//
// btcAmount is passed in by the caller rather than read off pendingTx.Total,
// because the callers that already have the fee-adjusted amount in hand (the
// Web3BigInt `amount` computed earlier in ProcessPendingBtcTransactions) is
// the exact figure that was (or would have been) sent, which is more
// trustworthy for an anomaly signal than re-deriving it here.
//
// Two things make this safe to call from inside the settlement loop:
//  1. If no webhook URL is configured, it is a no-op (graceful degrade).
//  2. The actual HTTP call runs in a detached goroutine with its own bounded
//     context, so a webhook failure or hang can never block or error the
//     caller's payout state transition (fire-and-forget with error logging;
//     see webhook.CallSwapPayoutWebhook, which itself never returns an error).
func (t *Telemetry) emitSwapPayoutWebhook(client *webhook.Client, pendingTx model.OnchainBtcProcessedTransaction, status model.BtcProcessingStatus, btcAmount, btcTxHash string) {
	webhookURL := t.appConfig.SwapPayoutWebhookURL
	if webhookURL == "" {
		return
	}

	// Best-effort ICY-amount enrichment: OnchainBtcProcessedTransaction does
	// not store it directly, it lives on the linked swap transaction. A lookup
	// failure (row not found, or a test DB without the table) must never block
	// the notification or the settlement transition, so it just logs and
	// leaves the field empty.
	icyAmount := ""
	if swapTx, err := t.store.OnchainIcySwapTransaction.GetByTransactionHash(t.db, pendingTx.SwapTransactionHash); err != nil {
		t.logger.Error("[ProcessPendingBtcTransactions][SwapPayoutWebhook][GetByTransactionHash]", map[string]string{
			"error": err.Error(),
			"id":    fmt.Sprintf("%d", pendingTx.ID),
		})
	} else {
		icyAmount = swapTx.IcyAmount
	}

	event := webhook.SwapPayoutEvent{
		Status:     string(status),
		IcyAmount:  icyAmount,
		BtcAmount:  btcAmount,
		BtcAddress: pendingTx.BTCAddress,
		BtcTxHash:  btcTxHash,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), swapPayoutWebhookTimeout)
		defer cancel()
		client.CallSwapPayoutWebhook(ctx, webhookURL, event)
	}()
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

	// One client for the whole batch: it is a stateless HTTP wrapper (see
	// webhook.New), so reuse across rows is just avoiding needless allocation.
	webhookClient := webhook.New(t.logger)

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
			t.emitSwapPayoutWebhook(webhookClient, pendingTx, model.BtcProcessingStatusFailed, pendingTx.Subtotal, "")
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
			t.emitSwapPayoutWebhook(webhookClient, pendingTx, model.BtcProcessingStatusFailed, amount.Value, "")
			continue
		}

		// --- Payout caps (SG-06): bound treasury outflow IN CODE, before any
		// broadcast. A leaked key or one bad authorization can drain at most a
		// single per-payout cap, and at most the daily cap across a rolling 24h.
		// The row is ALREADY claimed (processing) here, so refusing it to a
		// terminal state (failed / needs_reconcile) never leaves it re-claimable:
		// refused == NOT sent AND NOT left pending to auto-retry forever.

		// Per-payout cap: a payout larger than the cap can NEVER shrink under it,
		// so it is TERMINAL failed (not retried). 0 disables the cap.
		if maxPayout := t.appConfig.Bitcoin.MaxPayoutSatoshi; maxPayout > 0 && amtInt > maxPayout {
			t.logger.Error("[ProcessPendingBtcTransactions][PayoutCap] per-payout cap exceeded, refusing (marked failed, NOT sent)", map[string]string{
				"id":          fmt.Sprintf("%d", pendingTx.ID),
				"btc_address": pendingTx.BTCAddress,
				"amount":      fmt.Sprintf("%d", amtInt),
				"cap":         fmt.Sprintf("%d", maxPayout),
			})
			if uerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusFailed); uerr != nil {
				t.logger.Error("[ProcessPendingBtcTransactions][PayoutCap][UpdateStatus]", map[string]string{
					"error": uerr.Error(),
					"id":    fmt.Sprintf("%d", pendingTx.ID),
				})
			}
			t.emitSwapPayoutWebhook(webhookClient, pendingTx, model.BtcProcessingStatusFailed, amount.Value, "")
			continue
		}

		// Rolling daily cap: sum the sendable amount of the last 24h of sent
		// payouts and refuse THIS one if it would push the running total over the
		// cap. Not the payout's fault (its size is fine), so it is routed to
		// needs_reconcile for treasurer review rather than failed. 0 disables it.
		if maxDaily := t.appConfig.Bitcoin.MaxDailyPayoutSatoshi; maxDaily > 0 {
			sent, serr := t.store.OnchainBtcProcessedTransaction.SumSentInWindow(t.db, time.Now().Add(-24*time.Hour))
			if serr != nil {
				// FAIL CLOSED: if the rolling total cannot be computed, do NOT send.
				// Route to needs_reconcile so the daily ceiling can never be breached
				// on an unverified sum.
				t.logger.Error("[ProcessPendingBtcTransactions][DailyCap] rolling-sum query failed, refusing (needs_reconcile, NOT sent)", map[string]string{
					"error": serr.Error(),
					"id":    fmt.Sprintf("%d", pendingTx.ID),
				})
				if uerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusNeedsReconcile); uerr != nil {
					t.logger.Error("[ProcessPendingBtcTransactions][DailyCap][UpdateStatus]", map[string]string{
						"error": uerr.Error(),
						"id":    fmt.Sprintf("%d", pendingTx.ID),
					})
				}
				t.emitSwapPayoutWebhook(webhookClient, pendingTx, model.BtcProcessingStatusNeedsReconcile, amount.Value, "")
				continue
			}
			if sent+amtInt > maxDaily {
				t.logger.Error("[ProcessPendingBtcTransactions][DailyCap] rolling 24h cap would be crossed, refusing (needs_reconcile, NOT sent)", map[string]string{
					"id":          fmt.Sprintf("%d", pendingTx.ID),
					"btc_address": pendingTx.BTCAddress,
					"amount":      fmt.Sprintf("%d", amtInt),
					"sent_24h":    fmt.Sprintf("%d", sent),
					"cap":         fmt.Sprintf("%d", maxDaily),
				})
				if uerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, pendingTx.ID, model.BtcProcessingStatusNeedsReconcile); uerr != nil {
					t.logger.Error("[ProcessPendingBtcTransactions][DailyCap][UpdateStatus]", map[string]string{
						"error": uerr.Error(),
						"id":    fmt.Sprintf("%d", pendingTx.ID),
					})
				}
				t.emitSwapPayoutWebhook(webhookClient, pendingTx, model.BtcProcessingStatusNeedsReconcile, amount.Value, "")
				continue
			}
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
				t.emitSwapPayoutWebhook(webhookClient, pendingTx, model.BtcProcessingStatusNeedsReconcile, amount.Value, "")
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

		// Broadcast SUCCEEDED. Confirm-before-complete: the row is NOT marked
		// completed here. It moves to the intermediate "broadcasted" state
		// (recording the tx hash + fee + broadcast time); the confirmation sweep
		// promotes it to completed only once it reaches MinBtcConfirmations
		// on-chain. A crash between here and UpdateToBroadcasted leaves the row in
		// "processing" (never pending again), so it is never re-broadcast:
		// exactly-once holds even across a mid-payout crash. The worst case is a
		// stranded processing row for manual reconciliation, which is the safe
		// failure direction (never a double-send).

		// Log successful sends
		t.logger.Info("[ProcessPendingBtcTransactions] BTC sent successfully", map[string]string{
			"btc_address": pendingTx.BTCAddress,
			"amount":      amount.Value,
			"tx":          tx,
		})

		// Record the broadcast WITHOUT completing: confirm-before-complete. No
		// terminal webhook fires yet, "broadcasted" is not a settled state; the
		// completed webhook fires from the confirmation sweep once the tx is deep
		// enough (keeping SG-07's terminal-transition emits intact).
		err = t.store.OnchainBtcProcessedTransaction.UpdateToBroadcasted(t.db, pendingTx.ID, tx, networkFee)
		if err != nil {
			t.logger.Error("[ProcessPendingBtcTransactions][UpdateToBroadcasted]", map[string]string{
				"error": err.Error(),
			})
			continue
		}

		t.logger.Info(fmt.Sprintf("[ProcessPendingBtcTransactions] Transaction broadcast, awaiting confirmation: %s", tx))
	}

	// Confirm-before-complete sweep: promote any broadcasted rows that have
	// reached MinBtcConfirmations to completed (firing the completed webhook), and
	// route stuck (never-confirming) sends to needs_reconcile. Runs on the same
	// settlement tick so no extra cron wiring is needed. Errors are logged inside;
	// a sweep failure must not fail the pending-processing pass.
	if cerr := t.ConfirmBroadcastedBtcTransactions(); cerr != nil {
		t.logger.Error("[ProcessPendingBtcTransactions][ConfirmBroadcasted]", map[string]string{
			"error": cerr.Error(),
		})
	}

	return nil
}

// ConfirmBroadcastedBtcTransactions is the confirm-before-complete + stuck-tx
// sweep. For every payout in the intermediate "broadcasted" state it queries the
// live on-chain confirmation count and:
//
//   - conf >= MinBtcConfirmations  -> mark completed, fire the completed webhook.
//     This is the ONLY path to "completed": a payout is never completed on the
//     strength of a successful broadcast alone, only once it is confirmed.
//   - conf below threshold, and the broadcast is older than StuckTxTimeoutSeconds
//     -> route to needs_reconcile and fire that webhook. This is STUCK-TX
//     handling by detect-and-reconcile, NOT auto-RBF: it never re-sends. A safe
//     RBF would have to rebuild the SAME tx over the SAME UTXOs with a higher fee
//     (BIP-125), but btcrpc.Send re-selects UTXOs freshly every call, so an
//     automatic "bump" here would be a second INDEPENDENT tx that could confirm
//     alongside the original = a double-pay. So a stuck send is flagged for a
//     manual fee-bump/replace by the treasurer instead (see
//     docs/verification/confirmation-depth.md).
//   - otherwise (still maturing, not yet stuck) -> left broadcasted for a later
//     sweep.
//
// A per-row confirmation-query error is logged and skipped (retried next tick),
// never completed and never reconciled on a transient read failure. MinBtcConfir-
// mations is floored at 1 (a payout can never complete before it is at least
// mined). Idempotent: re-running over an already-terminal row is a no-op because
// GetBroadcastedTransactions only returns rows still in "broadcasted".
func (t *Telemetry) ConfirmBroadcastedBtcTransactions() error {
	broadcastedTxs, err := t.store.OnchainBtcProcessedTransaction.GetBroadcastedTransactions(t.db)
	if err != nil {
		t.logger.Error("[ConfirmBroadcastedBtcTransactions][GetBroadcastedTransactions]", map[string]string{
			"error": err.Error(),
		})
		return err
	}
	if len(broadcastedTxs) == 0 {
		return nil
	}

	minConf := t.appConfig.Bitcoin.MinBtcConfirmations
	if minConf < 1 {
		minConf = 1 // safety floor: never complete a not-yet-mined payout.
	}
	stuckTimeout := time.Duration(t.appConfig.Bitcoin.StuckTxTimeoutSeconds) * time.Second

	webhookClient := webhook.New(t.logger)

	for _, row := range broadcastedTxs {
		conf, err := t.btcRpc.GetTransactionConfirmations(row.BtcTransactionHash)
		if err != nil {
			// Transient read failure: leave broadcasted, retry next tick. Never
			// complete or reconcile on an unverified confirmation count.
			t.logger.Error("[ConfirmBroadcastedBtcTransactions][GetTransactionConfirmations]", map[string]string{
				"error":  err.Error(),
				"id":     fmt.Sprintf("%d", row.ID),
				"btc_tx": row.BtcTransactionHash,
			})
			continue
		}

		// The fee-adjusted amount that was actually sent (subtotal - service fee),
		// re-derived for webhook parity with SG-07's completed/failed emits.
		amount := (&model.Web3BigInt{Value: row.Subtotal, Decimal: consts.BTC_DECIMALS}).
			Sub(&model.Web3BigInt{Value: row.ServiceFee, Decimal: consts.BTC_DECIMALS})

		if conf >= minConf {
			if uerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, row.ID, model.BtcProcessingStatusCompleted); uerr != nil {
				t.logger.Error("[ConfirmBroadcastedBtcTransactions][UpdateStatus][Completed]", map[string]string{
					"error": uerr.Error(),
					"id":    fmt.Sprintf("%d", row.ID),
				})
				continue
			}
			t.logger.Info("[ConfirmBroadcastedBtcTransactions] payout confirmed and completed", map[string]string{
				"id":            fmt.Sprintf("%d", row.ID),
				"btc_tx":        row.BtcTransactionHash,
				"confirmations": fmt.Sprintf("%d", conf),
			})
			t.emitSwapPayoutWebhook(webhookClient, row, model.BtcProcessingStatusCompleted, amount.Value, row.BtcTransactionHash)
			continue
		}

		// Not confirmed yet. Stuck-tx detection: a broadcast that has not
		// confirmed within the timeout is flagged for manual RBF (NOT auto-sent).
		if row.ProcessedAt != nil && time.Since(*row.ProcessedAt) > stuckTimeout {
			if uerr := t.store.OnchainBtcProcessedTransaction.UpdateStatus(t.db, row.ID, model.BtcProcessingStatusNeedsReconcile); uerr != nil {
				t.logger.Error("[ConfirmBroadcastedBtcTransactions][UpdateStatus][NeedsReconcile]", map[string]string{
					"error": uerr.Error(),
					"id":    fmt.Sprintf("%d", row.ID),
				})
				continue
			}
			t.logger.Error("[ConfirmBroadcastedBtcTransactions][StuckTx] broadcast unconfirmed past timeout, routed to needs_reconcile (manual RBF, NOT auto-resent)", map[string]string{
				"id":            fmt.Sprintf("%d", row.ID),
				"btc_tx":        row.BtcTransactionHash,
				"confirmations": fmt.Sprintf("%d", conf),
				"broadcast_at":  row.ProcessedAt.String(),
				"stuck_timeout": stuckTimeout.String(),
			})
			t.emitSwapPayoutWebhook(webhookClient, row, model.BtcProcessingStatusNeedsReconcile, amount.Value, row.BtcTransactionHash)
			continue
		}

		t.logger.Info("[ConfirmBroadcastedBtcTransactions] still maturing", map[string]string{
			"id":            fmt.Sprintf("%d", row.ID),
			"btc_tx":        row.BtcTransactionHash,
			"confirmations": fmt.Sprintf("%d", conf),
			"min_conf":      fmt.Sprintf("%d", minConf),
		})
	}

	return nil
}
