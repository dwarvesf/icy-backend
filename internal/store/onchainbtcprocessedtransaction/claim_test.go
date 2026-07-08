package onchainbtcprocessedtransaction_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
)

// newTestDB spins up an isolated in-memory sqlite DB with the processed-tx
// table migrated. MaxOpenConns(1) pins a single connection so the in-memory
// database survives for the test and writes serialize (sqlite single-writer).
// The conditional-UPDATE claim semantics (RowsAffected) are portable to the
// production Postgres, so this exercises the exactly-once contract faithfully.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.OnchainBtcProcessedTransaction{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Start from a clean table (shared-cache DSN can retain state across opens).
	if err := db.Exec("DELETE FROM onchain_btc_processed_transactions").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func seedPending(t *testing.T, db *gorm.DB) int {
	t.Helper()
	row := &model.OnchainBtcProcessedTransaction{
		BTCAddress: "bc1qexampleaddr",
		Subtotal:   "1000",
		ServiceFee: "100",
		Status:     model.BtcProcessingStatusPending,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	return row.ID
}

func statusOf(t *testing.T, db *gorm.DB, id int) model.BtcProcessingStatus {
	t.Helper()
	var row model.OnchainBtcProcessedTransaction
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatalf("read status: %v", err)
	}
	return row.Status
}

// Item 1 (atomic claim): the first claim wins (RowsAffected == 1 -> true) and
// flips the row to processing; a second claim of the same row returns false
// without error. No lock-free SELECT-then-send window.
func TestClaimPendingTransaction_FirstWinsSecondSkips(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	id := seedPending(t, db)

	claimed, err := s.ClaimPendingTransaction(db, id)
	if err != nil {
		t.Fatalf("first claim err: %v", err)
	}
	if !claimed {
		t.Fatalf("first claim should win, got claimed=false")
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusProcessing {
		t.Fatalf("after claim, status = %q, want processing", got)
	}

	claimed2, err := s.ClaimPendingTransaction(db, id)
	if err != nil {
		t.Fatalf("second claim err: %v", err)
	}
	if claimed2 {
		t.Fatalf("second claim must lose (row no longer pending), got claimed=true")
	}
}

// Item 1 (atomic claim, concurrency): N goroutines race to claim the same
// pending row; exactly one may win. This is the core no-double-send invariant
// under overlapping cron cycles.
func TestClaimPendingTransaction_ConcurrentExactlyOneWins(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	id := seedPending(t, db)

	const workers = 12
	var wins int32
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < workers; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait() // release all goroutines at once
			ok, err := s.ClaimPendingTransaction(db, id)
			if err != nil {
				return
			}
			if ok {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	start.Done()
	done.Wait()

	if wins != 1 {
		t.Fatalf("exactly one claim must win, got %d wins", wins)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusProcessing {
		t.Fatalf("status = %q, want processing", got)
	}
}

// Item 3 (crash-between-broadcast-and-complete): once claimed to processing,
// the row is no longer returned by GetPendingTransactions and cannot be
// re-claimed, so a re-run after a crash sends zero additional broadcasts.
// Modeled with a sendCount that increments only on a winning claim.
func TestClaimPendingTransaction_CrashLeavesProcessing_NoResend(t *testing.T) {
	db := newTestDB(t)
	s := onchainbtcprocessedtransaction.New()
	id := seedPending(t, db)

	var sendCount int

	// Cycle 1: claim succeeds -> broadcast happens (sendCount++) -> CRASH here,
	// before UpdateToCompleted. Row is left in "processing".
	claimed, err := s.ClaimPendingTransaction(db, id)
	if err != nil {
		t.Fatalf("cycle1 claim err: %v", err)
	}
	if claimed {
		sendCount++ // the single legitimate broadcast
	}
	// (intentionally NOT calling UpdateToCompleted: simulate the crash)

	// Cycle 2: a fresh settlement run. The stranded row must not reappear as
	// pending, and must not be re-claimable.
	pending, err := s.GetPendingTransactions(db)
	if err != nil {
		t.Fatalf("cycle2 GetPendingTransactions err: %v", err)
	}
	for _, p := range pending {
		if p.ID == id {
			t.Fatalf("stranded processing row must not be returned as pending")
		}
	}
	claimed2, err := s.ClaimPendingTransaction(db, id)
	if err != nil {
		t.Fatalf("cycle2 claim err: %v", err)
	}
	if claimed2 {
		sendCount++ // would be a DOUBLE-SEND -- must never happen
	}

	if sendCount != 1 {
		t.Fatalf("exactly-once violated: sendCount = %d, want 1", sendCount)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusProcessing {
		t.Fatalf("crashed row status = %q, want processing (stranded, awaiting reconcile)", got)
	}
}
