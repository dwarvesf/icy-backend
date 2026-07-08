package server

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// SG-11 items 2 + 3: confirm-before-complete on the outgoing BTC side, and
// detect-and-reconcile for a stuck (never-confirming) send. Driven through the
// real orchestrator (ProcessPendingBtcTransactions) and the exported
// ConfirmBroadcastedBtcTransactions, mirroring how SG-05/07 tested settlement,
// because internal/telemetry's own test package does not build (pre-existing).

func telWithBtc(t *testing.T, db *gorm.DB, btc *mockBtcRpc, cfg *config.AppConfig) *telemetry.Telemetry {
	t.Helper()
	return telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
}

// seedBroadcasted inserts a payout already in the "broadcasted" state (broadcast
// succeeded, tx hash + fee recorded, processed_at set to `broadcastAt`).
func seedBroadcasted(t *testing.T, db *gorm.DB, subtotal, svcFee, btcTxHash string, broadcastAt time.Time) int {
	t.Helper()
	row := &model.OnchainBtcProcessedTransaction{
		BTCAddress:         "bc1qexampleaddr",
		Subtotal:           subtotal,
		ServiceFee:         svcFee,
		Status:             model.BtcProcessingStatusBroadcasted,
		BtcTransactionHash: btcTxHash,
		ProcessedAt:        &broadcastAt,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed broadcasted: %v", err)
	}
	return row.ID
}

// ---- Item 2: confirm-before-complete --------------------------------------

// CORE PROOF. A successful broadcast does NOT complete the payout: the row lands
// in "broadcasted" (not "completed") while its on-chain confirmations are below
// the threshold. Only once it reaches MinBtcConfirmations does it become
// completed. The BTC is sent exactly once across the whole flow.
func TestProcessPending_BroadcastNotConfirmed_StaysBroadcasted(t *testing.T) {
	db := newTestDB(t)
	// confHardcoded=true + confirmations=0 -> the just-broadcast tx is reported
	// as NOT yet confirmed.
	btc := &mockBtcRpc{confHardcoded: true, confirmations: 0}
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MinBtcConfirmations = 1
	cfg.Bitcoin.StuckTxTimeoutSeconds = 3600 // large: not stuck yet
	tel := telWithBtc(t, db, btc, cfg)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	// Tick 1: broadcast succeeds, but 0 confirmations -> broadcasted, NOT completed.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #1: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count = %d, want 1", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusBroadcasted {
		t.Fatalf("status = %q, want broadcasted (NOT completed before confirmation)", got)
	}

	// The tx now confirms. The next confirmation sweep promotes it to completed
	// and never re-broadcasts (send count stays 1).
	btc.confirmations = 3
	if err := tel.ConfirmBroadcastedBtcTransactions(); err != nil {
		t.Fatalf("confirm sweep: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed once confirmed", got)
	}
	if btc.count() != 1 {
		t.Fatalf("payout was re-sent during confirmation: send count = %d, want 1", btc.count())
	}
}

// NEGATIVE CONTROL for confirm-before-complete: with the mock reporting the tx as
// already confirmed (the default), the same happy-path row completes in one tick.
// This is the contrast to the test above: same shape, confirmed vs unconfirmed,
// completed vs broadcasted.
func TestProcessPending_BroadcastConfirmed_Completes(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{} // default: confirmations high -> confirmed
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MinBtcConfirmations = 1
	cfg.Bitcoin.StuckTxTimeoutSeconds = 3600
	tel := telWithBtc(t, db, btc, cfg)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed", got)
	}
	if btc.count() != 1 {
		t.Fatalf("send count = %d, want 1", btc.count())
	}
}

// Crash/return between broadcast and confirmation: a row stranded in
// "broadcasted" must NOT be lost, NOT re-broadcast, and NOT prematurely
// completed. ProcessPendingBtcTransactions never re-claims it (only pending rows
// are claimed), and with the tx still unconfirmed and not yet stuck it stays
// broadcasted for a later sweep. This is the confirm-side analogue of SG-05's
// crash-stranded-processing guarantee.
func TestProcessPending_CrashStrandedBroadcasted_NoResendNoComplete(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{confHardcoded: true, confirmations: 0}
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MinBtcConfirmations = 1
	cfg.Bitcoin.StuckTxTimeoutSeconds = 3600 // not stuck
	tel := telWithBtc(t, db, btc, cfg)
	id := seedBroadcasted(t, db, "1000", "100", "btc-tx-hash", time.Now())

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 0 {
		t.Fatalf("stranded broadcasted row was re-sent: send count = %d, want 0", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusBroadcasted {
		t.Fatalf("status = %q, want broadcasted (not lost, not completed)", got)
	}
}

// ---- Item 3: stuck-tx detect-and-reconcile (no auto-RBF) ------------------

// SAFETY PROOF (double-pay cannot happen). A broadcasted payout that has NOT
// confirmed past the stuck timeout is routed to needs_reconcile for a MANUAL
// fee-bump/replace. The handler must NEVER auto-send a replacement: Send is not
// called again, so no second, independent, competing BTC tx can ever be created
// (which is exactly the double-pay this sub-goal forbids). The original row's
// tx hash is preserved for the manual RBF.
func TestConfirmBroadcasted_StuckTx_RoutesNeedsReconcile_NoSecondSend(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{confHardcoded: true, confirmations: 0} // never confirms
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MinBtcConfirmations = 1
	cfg.Bitcoin.StuckTxTimeoutSeconds = 60 // stuck if broadcast > 60s ago
	tel := telWithBtc(t, db, btc, cfg)

	// Broadcast happened 10 minutes ago and still has 0 confirmations: stuck.
	id := seedBroadcasted(t, db, "1000", "100", "stuck-btc-tx", time.Now().Add(-10*time.Minute))

	if err := tel.ConfirmBroadcastedBtcTransactions(); err != nil {
		t.Fatalf("confirm sweep: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("status = %q, want needs_reconcile (stuck tx)", got)
	}
	// The CRITICAL safety assertion: no second BTC send was issued.
	if btc.count() != 0 {
		t.Fatalf("stuck-tx handler issued %d sends, want 0 (a second send would double-pay)", btc.count())
	}
	// The original tx hash is preserved so a human can RBF the SAME tx.
	var row model.OnchainBtcProcessedTransaction
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.BtcTransactionHash != "stuck-btc-tx" {
		t.Fatalf("btc_transaction_hash = %q, want stuck-btc-tx (preserved for manual RBF)", row.BtcTransactionHash)
	}
}

// NEGATIVE CONTROL for the stuck path: a broadcasted tx that is NOT past the
// timeout (recent broadcast) is left broadcasted, NOT reconciled, so a healthy
// pending-confirmation is never mistaken for a stuck one.
func TestConfirmBroadcasted_RecentUnconfirmed_StaysBroadcasted(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{confHardcoded: true, confirmations: 0}
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MinBtcConfirmations = 1
	cfg.Bitcoin.StuckTxTimeoutSeconds = 3600 // 1h timeout
	tel := telWithBtc(t, db, btc, cfg)

	// Broadcast 1 minute ago: unconfirmed but well within the timeout.
	id := seedBroadcasted(t, db, "1000", "100", "recent-btc-tx", time.Now().Add(-1*time.Minute))

	if err := tel.ConfirmBroadcastedBtcTransactions(); err != nil {
		t.Fatalf("confirm sweep: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusBroadcasted {
		t.Fatalf("status = %q, want broadcasted (not yet stuck)", got)
	}
	if btc.count() != 0 {
		t.Fatalf("send count = %d, want 0", btc.count())
	}
}
