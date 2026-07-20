package onchainbtcprocessedtransaction_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
)

// seedRowAt inserts a payout row with a deterministic processed_at (nil-able) and
// updated_at, bypassing gorm's autoUpdateTime stamping via a raw UPDATE so the
// window boundary can be tested exactly. Returns the row id.
func seedRowAt(t *testing.T, db *gorm.DB, status model.BtcProcessingStatus, subtotal, svcFee string, processedAt *time.Time, updatedAt time.Time) int {
	t.Helper()
	row := &model.OnchainBtcProcessedTransaction{
		BTCAddress: "bc1qexampleaddr",
		Subtotal:   subtotal,
		ServiceFee: svcFee,
		Status:     status,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if processedAt != nil {
		if err := db.Exec(`UPDATE onchain_btc_processed_transactions SET processed_at = ?, updated_at = ? WHERE id = ?`, *processedAt, updatedAt, row.ID).Error; err != nil {
			t.Fatalf("stamp row: %v", err)
		}
	} else {
		if err := db.Exec(`UPDATE onchain_btc_processed_transactions SET processed_at = NULL, updated_at = ? WHERE id = ?`, updatedAt, row.ID).Error; err != nil {
			t.Fatalf("stamp row: %v", err)
		}
	}
	return row.ID
}

// A completed and a broadcasted payout inside the 24h window are both counted,
// each contributing subtotal - service_fee. This is the rolling-sum input the
// SG-06 daily cap enforces against.
func TestSumSentInWindow_CountsSentStatesInWindow(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	recent := now.Add(-1 * time.Hour)

	seedRowAt(t, db, model.BtcProcessingStatusCompleted, "1000", "100", &recent, recent)   // 900
	seedRowAt(t, db, model.BtcProcessingStatusBroadcasted, "2000", "100", &recent, recent) // 1900

	got, err := s.SumSentInWindow(db, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SumSentInWindow: %v", err)
	}
	if got != 2800 {
		t.Fatalf("sum = %d, want 2800 (900 + 1900)", got)
	}
}

// A payout sent OUTSIDE the 24h window (processed_at 25h ago) is excluded, so it
// no longer counts against today's rolling cap.
func TestSumSentInWindow_ExcludesRowsOutsideWindow(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	old := now.Add(-25 * time.Hour)

	seedRowAt(t, db, model.BtcProcessingStatusCompleted, "5000", "100", &old, old)

	got, err := s.SumSentInWindow(db, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SumSentInWindow: %v", err)
	}
	if got != 0 {
		t.Fatalf("sum = %d, want 0 (row is older than the window)", got)
	}
}

// Non-sent states (pending / processing / failed), even when recent, are NOT
// counted: no BTC left the treasury for them. Negative control for the counted
// states above.
func TestSumSentInWindow_ExcludesNonSentStates(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	recent := now.Add(-1 * time.Hour)

	seedRowAt(t, db, model.BtcProcessingStatusPending, "1000", "100", &recent, recent)
	seedRowAt(t, db, model.BtcProcessingStatusProcessing, "1000", "100", &recent, recent)
	seedRowAt(t, db, model.BtcProcessingStatusFailed, "1000", "100", &recent, recent)

	got, err := s.SumSentInWindow(db, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SumSentInWindow: %v", err)
	}
	if got != 0 {
		t.Fatalf("sum = %d, want 0 (pending/processing/failed are not sent)", got)
	}
}

// An AMBIGUOUS needs_reconcile row with NO processed_at (the ambiguous-send path
// only stamps status) still counts, via its updated_at, when recent: the daily
// total must include maybe-sent BTC (conservative ceiling). An old one is
// excluded.
func TestSumSentInWindow_NeedsReconcileCountedViaUpdatedAt(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()

	seedRowAt(t, db, model.BtcProcessingStatusNeedsReconcile, "1000", "100", nil, now.Add(-2*time.Hour))  // 900, recent
	seedRowAt(t, db, model.BtcProcessingStatusNeedsReconcile, "3000", "100", nil, now.Add(-30*time.Hour)) // old, excluded

	got, err := s.SumSentInWindow(db, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SumSentInWindow: %v", err)
	}
	if got != 900 {
		t.Fatalf("sum = %d, want 900 (recent needs_reconcile via updated_at; old excluded)", got)
	}
}

// seedRowWithNetFee is seedRowAt plus a recorded network_fee, so the daily-cap
// sum's inclusion of miner fees can be asserted.
func seedRowWithNetFee(t *testing.T, db *gorm.DB, status model.BtcProcessingStatus, subtotal, svcFee, netFee string, at time.Time) int {
	t.Helper()
	row := &model.OnchainBtcProcessedTransaction{
		BTCAddress: "bc1qexampleaddr",
		Subtotal:   subtotal,
		ServiceFee: svcFee,
		NetworkFee: netFee,
		Status:     status,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if err := db.Exec(`UPDATE onchain_btc_processed_transactions SET processed_at = ?, updated_at = ? WHERE id = ?`, at, at, row.ID).Error; err != nil {
		t.Fatalf("stamp row: %v", err)
	}
	return row.ID
}

// The rolling sum counts the network (miner) fee as BTC that left the treasury:
// contribution is (subtotal - service_fee) + network_fee.
func TestSumSentInWindow_IncludesNetworkFee(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	recent := now.Add(-1 * time.Hour)

	// subtotal 1000 - fee 100 = 900 sendable, plus 50 network fee = 950.
	seedRowWithNetFee(t, db, model.BtcProcessingStatusCompleted, "1000", "100", "50", recent)

	got, err := s.SumSentInWindow(db, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SumSentInWindow: %v", err)
	}
	if got != 950 {
		t.Fatalf("sum = %d, want 950 (900 payout + 50 network fee)", got)
	}
}

// Negative control for the network-fee inclusion: the SAME payout with NO
// recorded network fee contributes only its sendable amount (900). If the fee
// term were mishandled (e.g. a parse error on empty), this would fail.
func TestSumSentInWindow_NoNetworkFee_OnlySendableCounted(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	recent := now.Add(-1 * time.Hour)

	seedRowWithNetFee(t, db, model.BtcProcessingStatusCompleted, "1000", "100", "", recent)

	got, err := s.SumSentInWindow(db, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SumSentInWindow: %v", err)
	}
	if got != 900 {
		t.Fatalf("sum = %d, want 900 (payout only, no network fee recorded)", got)
	}
}

// Fail-closed: an unparseable network_fee returns an error rather than silently
// under-counting the daily total.
func TestSumSentInWindow_UnparseableNetworkFeeFailsClosed(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	recent := now.Add(-1 * time.Hour)

	seedRowWithNetFee(t, db, model.BtcProcessingStatusCompleted, "1000", "100", "not-a-number", recent)

	if _, err := s.SumSentInWindow(db, now.Add(-24*time.Hour)); err == nil {
		t.Fatal("want an error for an unparseable network_fee, got nil (would under-count)")
	}
}

// Fail-closed: an unparseable amount returns an error rather than silently
// under-counting the daily total (which would weaken the cap).
func TestSumSentInWindow_UnparseableAmountFailsClosed(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	now := time.Now()
	recent := now.Add(-1 * time.Hour)

	seedRowAt(t, db, model.BtcProcessingStatusCompleted, "not-a-number", "0", &recent, recent)

	if _, err := s.SumSentInWindow(db, now.Add(-24*time.Hour)); err == nil {
		t.Fatal("want an error for an unparseable amount, got nil (would under-count)")
	}
}
