package server

import (
	"testing"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// newTelemetryWithCaps builds a Telemetry whose settlement path enforces the
// SG-06 per-payout and rolling-daily caps at the given satoshi thresholds (0
// disables a cap).
func newTelemetryWithCaps(t *testing.T, db *gorm.DB, btc *mockBtcRpc, maxPayout, maxDaily int64) *telemetry.Telemetry {
	t.Helper()
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MaxPayoutSatoshi = maxPayout
	cfg.Bitcoin.MaxDailyPayoutSatoshi = maxDaily
	return telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
}

func countStatus(t *testing.T, db *gorm.DB, status model.BtcProcessingStatus) int {
	t.Helper()
	var n int64
	if err := db.Model(&model.OnchainBtcProcessedTransaction{}).Where("status = ?", status).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", status, err)
	}
	return int(n)
}

// A payout comfortably under BOTH caps proceeds normally: broadcast once, marked
// completed. This is the positive control the two refusal tests contrast against.
func TestProcessPending_UnderBothCaps_Proceeds(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetryWithCaps(t, db, btc, 5_000_000, 25_000_000)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100") // 900 sat

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count = %d, want 1", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed", got)
	}
}

// PER-PAYOUT CAP. A single payout whose sendable amount (900 sat) exceeds the
// per-payout cap (500 sat) is REFUSED before any broadcast and marked failed
// (terminal: a fixed-size payout can never shrink under the cap). Zero sends.
//
// Negative control: TestProcessPending_UnderBothCaps_Proceeds is the same row
// shape with a cap above the amount; it sends once and completes.
func TestProcessPending_OverPerPayoutCap_RefusedFailed(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetryWithCaps(t, db, btc, 500, 25_000_000) // cap below the 900-sat payout
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 0 {
		t.Fatalf("over-cap payout was sent: send count = %d, want 0", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusFailed {
		t.Fatalf("status = %q, want failed", got)
	}
}

// DAILY CAP (single crosser). With 900 sat already sent in the window (a recent
// completed row counted via its updated_at) and a daily cap of 1000 sat, the next
// 900-sat payout would push the rolling total to 1800 > 1000, so it is REFUSED
// and routed to needs_reconcile (not the payout's fault; treasurer review). Zero
// new sends; the already-sent row is untouched.
func TestProcessPending_DailyCapCrossed_RefusesCrosser(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetryWithCaps(t, db, btc, 5_000_000, 1000)

	alreadySent := seed(t, db, model.BtcProcessingStatusCompleted, "1000", "100") // 900 sat, recent
	crosser := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")       // +900 -> 1800 > 1000

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 0 {
		t.Fatalf("daily-cap crosser was sent: send count = %d, want 0", btc.count())
	}
	if got := statusOf(t, db, crosser); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("crosser status = %q, want needs_reconcile", got)
	}
	if got := statusOf(t, db, alreadySent); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("already-sent row disturbed: status = %q, want completed", got)
	}
}

// DAILY CAP (sequence). Three identical 900-sat payouts under a 2000-sat daily
// cap in ONE settlement pass: the first two send (900, then 1800, both <= 2000),
// the third would reach 2700 > 2000 and is refused to needs_reconcile. Proves the
// rolling sum re-evaluates within the loop (each broadcast counts toward the next
// row's check) so exactly the crossing payout is stopped.
func TestProcessPending_SequenceCrossesDailyCap_RefusesOnlyCrosser(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetryWithCaps(t, db, btc, 5_000_000, 2000)

	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")
	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")
	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 2 {
		t.Fatalf("send count = %d, want 2 (first two under cap, third refused)", btc.count())
	}
	if got := countStatus(t, db, model.BtcProcessingStatusCompleted); got != 2 {
		t.Fatalf("completed rows = %d, want 2", got)
	}
	if got := countStatus(t, db, model.BtcProcessingStatusNeedsReconcile); got != 1 {
		t.Fatalf("needs_reconcile rows = %d, want 1 (the crosser)", got)
	}
}

// DAILY CAP (negative control). Same shape as the crosser test but with a daily
// cap high enough (5000) that 900 already-sent + 900 new = 1800 stays under it:
// the payout proceeds and completes. Same inputs, cap moved, opposite outcome.
func TestProcessPending_UnderDailyCap_Proceeds(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetryWithCaps(t, db, btc, 5_000_000, 5000)

	seed(t, db, model.BtcProcessingStatusCompleted, "1000", "100") // 900 already sent
	next := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count = %d, want 1 (under the daily cap)", btc.count())
	}
	if got := statusOf(t, db, next); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed", got)
	}
}
