//go:build integration

// Real-Postgres proof for the SG-06 daily-cap cross-process TOCTOU fix.
//
// The daily cap (SumSentInWindow -> compare -> send -> record) is a read-then-
// write that is only atomic WITHIN one process. Under multiple replicas two
// settlement passes could both read the same window sum, both pass the cap, and
// both send: the "true ceiling" is breached. The fix wraps the whole settlement
// pass in a cluster-wide Postgres advisory lock (settlementAdvisoryLockKey) so at
// most one pass runs at a time across the fleet.
//
// pg_advisory_lock is a Postgres feature that the sqlite unit tests cannot
// exercise, so this proof runs against a real Postgres. It is gated behind the
// `integration` build tag and skips unless DATABASE_URL is set, so the DB-free CI
// still compiles and runs the sqlite suite.
//
//	Positive : two settlement passes (the REAL guarded ProcessPendingBtcTransactions)
//	           race against the same DB -> total BTC dispensed NEVER exceeds the cap.
//	Negative : the SAME check-then-send critical section WITHOUT the advisory lock,
//	           forced to interleave with a barrier -> the overshoot reproduces.
//
// Run: DATABASE_URL='postgres://icy:icy@localhost:55432/icy?sslmode=disable' \
//	    go test -tags integration ./internal/server/ -run DailyCap -count=1

package server

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
)

// newPostgresDB connects to the DATABASE_URL Postgres, migrates the processed-tx
// table, and truncates it. Unlike the sqlite newTestDB it does NOT pin
// MaxOpenConns(1): concurrent settlement passes must get real, separate backend
// sessions or the cross-process race cannot be exercised.
func newPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping real-Postgres daily-cap integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if got := db.Dialector.Name(); got != "postgres" {
		t.Fatalf("dialector = %q, want postgres", got)
	}
	if err := db.AutoMigrate(&model.OnchainBtcProcessedTransaction{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec("TRUNCATE onchain_btc_processed_transactions RESTART IDENTITY").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

// dispensedInWindow sums the sendable amount (subtotal - service_fee) of the rows
// that reflect an ACTUAL send (completed / broadcasted) inside the window. This is
// the true BTC dispensed. It deliberately EXCLUDES needs_reconcile: a cap-refused
// crosser is routed there without being sent, and counting it would mask an
// overshoot behind the conservative over-count that SumSentInWindow uses.
func dispensedInWindow(t *testing.T, db *gorm.DB, since time.Time) int64 {
	t.Helper()
	var rows []model.OnchainBtcProcessedTransaction
	err := db.Where("status IN ?", []string{
		string(model.BtcProcessingStatusCompleted),
		string(model.BtcProcessingStatusBroadcasted),
	}).Find(&rows).Error
	if err != nil {
		t.Fatalf("dispensed query: %v", err)
	}
	var total int64
	for _, r := range rows {
		at := r.UpdatedAt
		if r.ProcessedAt != nil {
			at = *r.ProcessedAt
		}
		if at.Before(since) {
			continue
		}
		sub, _ := strconv.ParseInt(r.Subtotal, 10, 64)
		var fee int64
		if r.ServiceFee != "" {
			fee, _ = strconv.ParseInt(r.ServiceFee, 10, 64)
		}
		if amt := sub - fee; amt > 0 {
			total += amt
		}
	}
	return total
}

// slowSend returns a mock Send that blocks for d before succeeding. A real
// btcRpc.Send takes network time (hundreds of ms), which is what makes the
// cross-process daily-cap window WIDE in production. Modeling that delay here is
// what makes this a load-bearing test: with the delay, an UNGUARDED second pass
// would read the stale window sum mid-send and overshoot (proven by the neuter
// negative control in docs/verification/custody-caps.md); the advisory lock keeps
// it from ever running.
func slowSend(d time.Duration) func(string, *model.Web3BigInt) (string, int64, error) {
	return func(string, *model.Web3BigInt) (string, int64, error) {
		time.Sleep(d)
		return "btc-tx-hash", 100, nil
	}
}

// TestDailyCapAtomic_UnderConcurrency (POSITIVE). Baseline 900 sat already sent;
// two 900-sat payouts pending; daily cap 2000. Exactly ONE more payout fits
// (900+900=1800 <= 2000; a second would be 2700 > 2000). Two independent
// Telemetry instances (two "pods", same DB, same advisory-lock key) run the REAL
// guarded settlement pass concurrently, each with a SLOW send that holds the race
// window open. The advisory lock makes settlement fleet-singleton: the pass that
// loses the lock skips its whole tick, so total BTC dispensed stays at 1800 <= cap
// and exactly one payout is sent across BOTH processes.
//
// NEGATIVE CONTROL (documented + reproducible): bypassing withSettlementLock
// (return fn() at its top) makes BOTH passes run; with the slow send they both
// read the 900 baseline before either records, both send, and dispensed climbs to
// 2700 > cap. See docs/verification/custody-caps.md. TestDailyCapNoLock_Overshoots
// reproduces that same overshoot deterministically at the store level.
func TestDailyCapAtomic_UnderConcurrency(t *testing.T) {
	db := newPostgresDB(t)
	const cap int64 = 2000
	since := time.Now().Add(-24 * time.Hour)

	seed(t, db, model.BtcProcessingStatusCompleted, "1000", "100") // baseline 900 sent
	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")   // 900
	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")   // 900

	btc1 := &mockBtcRpc{sendFn: slowSend(250 * time.Millisecond)}
	btc2 := &mockBtcRpc{sendFn: slowSend(250 * time.Millisecond)}
	tel1 := newTelemetryWithCaps(t, db, btc1, 5_000_000, cap)
	tel2 := newTelemetryWithCaps(t, db, btc2, 5_000_000, cap)

	// start gate: release both passes at the same instant so they genuinely race.
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-start; _ = tel1.ProcessPendingBtcTransactions() }()
	go func() { defer wg.Done(); <-start; _ = tel2.ProcessPendingBtcTransactions() }()
	close(start)
	wg.Wait()

	sends := btc1.count() + btc2.count()
	if sends != 1 {
		t.Fatalf("total sends across both pods = %d, want 1 (the advisory lock must serialize settlement)", sends)
	}
	got := dispensedInWindow(t, db, since)
	if got > cap {
		t.Fatalf("BTC dispensed in window = %d, EXCEEDS daily cap %d (TOCTOU not closed)", got, cap)
	}
	if got != 1800 {
		t.Fatalf("BTC dispensed = %d, want 1800 (baseline 900 + exactly one 900 payout)", got)
	}
}

// TestDailyCapNoLock_Overshoots (NEGATIVE CONTROL). The SAME scenario, but the
// two racers execute the check-then-send critical section WITHOUT the advisory
// lock, forced to interleave with a barrier so both read the window sum BEFORE
// either records its send. This is exactly what the pre-fix settlement pass did
// per process. On real Postgres both pass the cap on the stale sum and both send:
// dispensed climbs to 2700 > 2000. Proving the overshoot reproduces here is what
// makes the positive test above meaningful: the advisory lock is the only thing
// standing between these two outcomes.
func TestDailyCapNoLock_Overshoots(t *testing.T) {
	db := newPostgresDB(t)
	const cap int64 = 2000
	since := time.Now().Add(-24 * time.Hour)

	seed(t, db, model.BtcProcessingStatusCompleted, "1000", "100") // baseline 900 sent
	idA := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")
	idB := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	s := onchainbtcprocessedtransaction.New()
	btc := &mockBtcRpc{}

	// 2-way barrier: both goroutines read the window sum, then wait here so
	// neither has recorded its send yet when the other evaluates the cap.
	var readBarrier sync.WaitGroup
	readBarrier.Add(2)

	// unguardedSettleOnce mirrors btc.go's pre-lock per-payout critical section:
	// claim -> SumSentInWindow -> (barrier) -> check cap -> Send -> record. No
	// advisory lock, so nothing serializes the aggregate sum across the two.
	unguardedSettleOnce := func(id int) {
		if ok, err := s.ClaimPendingTransaction(db, id); err != nil || !ok {
			t.Errorf("claim %d: ok=%v err=%v", id, ok, err)
			return
		}
		sent, err := s.SumSentInWindow(db, since)
		if err != nil {
			t.Errorf("sum %d: %v", id, err)
			return
		}
		readBarrier.Done()
		readBarrier.Wait() // both have read the (stale) sum before either sends

		const payout int64 = 900
		if sent+payout > cap {
			return // would refuse -- but with the stale sum, neither does
		}
		if _, _, serr := btc.Send("bc1qexampleaddr", &model.Web3BigInt{Value: "900", Decimal: 8}); serr != nil {
			t.Errorf("send %d: %v", id, serr)
			return
		}
		if uerr := s.UpdateToBroadcasted(db, id, "btc-tx-hash", 100); uerr != nil {
			t.Errorf("record %d: %v", id, uerr)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); unguardedSettleOnce(idA) }()
	go func() { defer wg.Done(); unguardedSettleOnce(idB) }()
	wg.Wait()

	if got := btc.count(); got != 2 {
		t.Fatalf("unguarded sends = %d, want 2 (both racers passed the stale-sum cap)", got)
	}
	got := dispensedInWindow(t, db, since)
	if got <= cap {
		t.Fatalf("dispensed = %d, expected OVERSHOOT above cap %d without the lock", got, cap)
	}
	if got != 2700 {
		t.Fatalf("dispensed = %d, want 2700 (baseline 900 + two 900 payouts) -- the reproduced overshoot", got)
	}
}
