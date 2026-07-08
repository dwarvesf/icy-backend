package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// ---- test doubles -------------------------------------------------------

// runnerFunc adapts a func to the unexported settlementRunner interface so the
// cron-overlap behavior can be tested without a full telemetry stack.
type runnerFunc func() error

func (f runnerFunc) ProcessPendingBtcTransactions() error { return f() }

// mockBtcRpc counts Send calls so tests can assert exactly-once broadcast.
type mockBtcRpc struct {
	sendCount int32
	sendFn    func(addr string, amt *model.Web3BigInt) (string, int64, error)
}

func (m *mockBtcRpc) Send(addr string, amt *model.Web3BigInt) (string, int64, error) {
	atomic.AddInt32(&m.sendCount, 1)
	if m.sendFn != nil {
		return m.sendFn(addr, amt)
	}
	return "btc-tx-hash", 100, nil
}
func (m *mockBtcRpc) count() int32 { return atomic.LoadInt32(&m.sendCount) }

func (m *mockBtcRpc) CurrentBalance() (*model.Web3BigInt, error) {
	return &model.Web3BigInt{Value: "0", Decimal: 8}, nil
}
func (m *mockBtcRpc) GetTransactionsByAddress(address, fromTxId string) ([]model.OnchainBtcTransaction, error) {
	return nil, nil
}
func (m *mockBtcRpc) EstimateFees() (map[string]float64, error) { return map[string]float64{}, nil }
func (m *mockBtcRpc) GetSatoshiUSDPrice() (float64, error)      { return 0, nil }
func (m *mockBtcRpc) IsDust(address string, amount int64) bool  { return false }

// ---- fixtures -----------------------------------------------------------

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
	if err := db.Exec("DELETE FROM onchain_btc_processed_transactions").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func newTelemetry(t *testing.T, db *gorm.DB, btc *mockBtcRpc) *telemetry.Telemetry {
	t.Helper()
	return telemetry.New(db, store.New(db), &config.AppConfig{}, logger.New(environments.Test), btc, nil, nil)
}

func seed(t *testing.T, db *gorm.DB, status model.BtcProcessingStatus, subtotal, svcFee string) int {
	t.Helper()
	row := &model.OnchainBtcProcessedTransaction{
		BTCAddress: "bc1qexampleaddr",
		Subtotal:   subtotal,
		ServiceFee: svcFee,
		Status:     status,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
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

// ---- Item 2: cron overlap ----------------------------------------------

// A settlement tick that fires while the previous run is still in flight must
// be skipped, not run concurrently (SkipIfStillRunning).
func TestNewSettlementJob_SkipsOverlappingTick(t *testing.T) {
	var calls int32
	entered := make(chan struct{})
	release := make(chan struct{})

	runner := runnerFunc(func() error {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(entered)
			<-release // hold the first run in flight
		}
		return nil
	})

	job := newSettlementJob(runner, logger.New(environments.Test))

	go job.Run() // first run: enters and blocks
	<-entered    // guarantee the first run holds the slot

	job.Run() // second (overlapping) tick: must be skipped -> returns immediately

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("overlapping tick was not skipped: calls = %d, want 1", got)
	}

	close(release)
	time.Sleep(20 * time.Millisecond) // let the first run drain

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("unexpected extra run after release: calls = %d, want 1", got)
	}
}

// ---- Item 1 + 3: orchestrator exactly-once -----------------------------

// Happy path: one pending row is broadcast exactly once and marked completed.
func TestProcessPending_HappyPath_SendsOnceAndCompletes(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

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

// Re-run idempotency: a completed row is never re-broadcast on the next cycle.
func TestProcessPending_RerunAfterComplete_NoResend(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)
	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #1: %v", err)
	}
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #2: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count = %d, want 1 across two runs", btc.count())
	}
}

// Crash-between-broadcast-and-complete: a row stranded in "processing" (broadcast
// succeeded, then crash before the completion write) is NOT re-broadcast on the
// next settlement run. This is the exactly-once guarantee across a crash.
func TestProcessPending_CrashStrandedProcessing_NoResend(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)
	id := seed(t, db, model.BtcProcessingStatusProcessing, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 0 {
		t.Fatalf("stranded processing row re-broadcast: send count = %d, want 0", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusProcessing {
		t.Fatalf("status = %q, want processing (untouched)", got)
	}
}

// Two overlapping settlement cycles hitting the SAME pending row: even if the
// cron guard were bypassed, the atomic claim ensures exactly one broadcast.
func TestProcessPending_ConcurrentOverlap_SendsOnce(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)
	seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tel.ProcessPendingBtcTransactions()
		}()
	}
	wg.Wait()

	if btc.count() != 1 {
		t.Fatalf("overlap double-send: send count = %d, want 1", btc.count())
	}
}

// Negative-amount guard (the fix): subtotal below the service fee yields a
// negative payout. The old guard checked amount.Decimal (constant 8, never < 0)
// so it would have broadcast a negative amount. The fixed guard rejects it:
// zero sends, row marked failed.
func TestProcessPending_NegativeAmount_NoSendMarksFailed(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)
	id := seed(t, db, model.BtcProcessingStatusPending, "50", "100") // 50 - 100 = -50

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if btc.count() != 0 {
		t.Fatalf("negative amount was sent: send count = %d, want 0", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusFailed {
		t.Fatalf("status = %q, want failed", got)
	}
}
