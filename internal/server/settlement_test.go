package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
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

// ---- Double-send fix: conditional release on Send error --------------------

// CORE FIX. Send fails with an AMBIGUOUS / post-broadcast error (the signed tx
// may already be live: response lost / read timeout / 5xx after enqueue). The row
// must NOT be released to pending, and the next settlement tick must NOT re-send
// it. Before the fix the row was unconditionally released to pending, so the next
// tick rebuilt a fresh tx paying the same recipient again = BTC sent twice.
//
// Negative control (see docs/verification/settlement-idempotent.md §Double-send):
// reverting the else-branch to release-to-pending makes the row pending again and
// tick 2 re-sends (send count = 2) exactly the double-send this test forbids.
func TestProcessPending_AmbiguousError_NotReleased_NoResend(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
		// Plain error, NOT wrapped with btcrpc.ErrNotBroadcast -> ambiguous class.
		return "", 0, fmt.Errorf("broadcast response lost after node enqueue")
	}}
	tel := newTelemetry(t, db, btc)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	// Tick 1: ambiguous failure.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #1: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count after tick 1 = %d, want 1", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("status = %q, want needs_reconcile (NOT released to pending)", got)
	}

	// Tick 2: the row must not be re-claimed or re-broadcast.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #2: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("AMBIGUOUS broadcast was re-sent: send count = %d, want 1 (double-send)", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("status after tick 2 = %q, want needs_reconcile", got)
	}
}

// LIVENESS + contrast to the test above. Send fails with a DEFINITELY-not-sent
// error (wrapped btcrpc.ErrNotBroadcast, e.g. insufficient funds). No BTC left
// the treasury, so the row IS released to pending and a later healthy tick DOES
// retry (send count climbs 1 -> 2, ending completed). The count reaching 2 here
// is the negative control for the ambiguous test: same shape, opposite class,
// opposite outcome.
func TestProcessPending_NotBroadcastError_ReleasedForRetry(t *testing.T) {
	db := newTestDB(t)
	var failFirst int32 = 1
	btc := &mockBtcRpc{sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
		if atomic.LoadInt32(&failFirst) == 1 {
			return "", 0, fmt.Errorf("select utxos: insufficient funds: %w", btcrpc.ErrNotBroadcast)
		}
		return "btc-tx-hash", 100, nil
	}}
	tel := newTelemetry(t, db, btc)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	// Tick 1: not-broadcast error -> released to pending.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #1: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count after tick 1 = %d, want 1", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusPending {
		t.Fatalf("status = %q, want pending (released for retry)", got)
	}

	// Tick 2: the row is retried and now succeeds.
	atomic.StoreInt32(&failFirst, 0)
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #2: %v", err)
	}
	if btc.count() != 2 {
		t.Fatalf("released row was not retried: send count = %d, want 2", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status after retry = %q, want completed", got)
	}
}

// #3: an "already known" node response (the re-POST of THIS exact tx, already in
// the mempool / chain) is classified as SUCCESS at the blockstream layer:
// BroadcastTx returns ErrTxAlreadyKnown, which the caller (btcrpc.broadcast)
// turns into the locally-computed txid + nil error, so the settlement row
// completes and is never rebuilt-and-resent. Only UNAMBIGUOUS already-broadcast
// markers qualify here; bad-txns-inputs-missingorspent is deliberately excluded
// (see TestBroadcastTx_InputsMissingOrSpent_NotAlreadyKnown). The orchestrator's
// success -> completed half is covered by
// TestProcessPending_HappyPath_SendsOnceAndCompletes.
func TestBroadcastTx_AlreadyKnown_TreatedAsSuccess(t *testing.T) {
	bodies := []string{
		"sendrawtransaction RPC error -27: txn-already-known",
		"sendrawtransaction RPC error: transaction already in block chain",
	}
	for _, body := range bodies {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(body))
		}))
		bs := blockstream.NewWithURL(&config.AppConfig{}, logger.New(environments.Test), srv.URL)
		_, err := bs.BroadcastTx("00")
		srv.Close()
		if !errors.Is(err, blockstream.ErrTxAlreadyKnown) {
			t.Fatalf("body %q: got err %v, want ErrTxAlreadyKnown", body, err)
		}
	}
}

// Negative control for #3: a GENUINE broadcast rejection (not an already-known
// case) must NOT be misclassified as success, or a real failure would silently
// look like a completed payout.
func TestBroadcastTx_GenuineError_NotAlreadyKnown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("sendrawtransaction RPC error -25: bad-txns-in-belowout"))
	}))
	defer srv.Close()
	bs := blockstream.NewWithURL(&config.AppConfig{}, logger.New(environments.Test), srv.URL)
	_, err := bs.BroadcastTx("00")
	if err == nil {
		t.Fatal("want a broadcast error, got nil")
	}
	if errors.Is(err, blockstream.ErrTxAlreadyKnown) {
		t.Fatalf("genuine rejection misclassified as already-known: %v", err)
	}
}

// HIGH fix (blockstream half): "bad-txns-inputs-missingorspent" must NOT be
// classified as broadcast-success. The node reply "inputs already spent" is true
// both when OUR identical tx confirmed AND when a DIFFERENT tx (external spend,
// RBF, or an indexer-lag race between UTXO selection and broadcast) spent the
// inputs, in which case our tx can never confirm. Treating it as success would
// mark the row completed with a locally-derived txid that does not exist
// on-chain: a silent terminal mis-settle of real BTC. So BroadcastTx must return
// a real broadcast error (NOT ErrTxAlreadyKnown) for this marker, letting it fall
// through to the ambiguous path that the orchestrator routes to needs_reconcile.
//
// NEGATIVE CONTROL: restoring "bad-txns-inputs-missingorspent" to
// alreadyBroadcastMarkers makes BroadcastTx return ErrTxAlreadyKnown, which
// btcrpc.broadcast converts to a locally-computed txid + nil error, so the row
// would be wrongly marked completed with a fake txid (the mis-settle this fix
// removes). This test then fails on the errors.Is assertion below.
func TestBroadcastTx_InputsMissingOrSpent_NotAlreadyKnown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("sendrawtransaction RPC error -25: bad-txns-inputs-missingorspent"))
	}))
	defer srv.Close()
	bs := blockstream.NewWithURL(&config.AppConfig{}, logger.New(environments.Test), srv.URL)
	_, err := bs.BroadcastTx("00")
	if err == nil {
		t.Fatal("want a broadcast error, got nil")
	}
	if errors.Is(err, blockstream.ErrTxAlreadyKnown) {
		t.Fatalf("inputs-missingorspent misclassified as already-known (would mis-settle as completed): %v", err)
	}
}

// HIGH fix (orchestrator half): once BroadcastTx no longer flags
// inputs-missingorspent as already-known, the error propagates out of Send
// UNWRAPPED (not btcrpc.ErrNotBroadcast) as an ambiguous broadcast error. The
// orchestrator must route the row to needs_reconcile, NEVER completed, and never
// re-send it. This is the same ambiguous-class routing proven by
// TestProcessPending_AmbiguousError_NotReleased_NoResend, asserted here for the
// concrete inputs-missingorspent payload so the mis-settle is nailed end-to-end.
func TestProcessPending_MissingOrSpent_RoutesNeedsReconcile(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
		// What Send returns after the fix: a plain broadcast error, unwrapped
		// (ambiguous class), carrying the node's inputs-missingorspent body.
		return "", 0, fmt.Errorf("status code: 400, failed to broadcast transaction: sendrawtransaction RPC error -25: bad-txns-inputs-missingorspent")
	}}
	tel := newTelemetry(t, db, btc)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	// Tick 1: ambiguous inputs-missingorspent failure.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #1: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("send count after tick 1 = %d, want 1", btc.count())
	}
	if got := statusOf(t, db, id); got == model.BtcProcessingStatusCompleted {
		t.Fatalf("mis-settle: row wrongly marked completed for inputs-missingorspent")
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("status = %q, want needs_reconcile (NOT completed)", got)
	}

	// Tick 2: the needs_reconcile row must not be re-claimed or re-broadcast.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process #2: %v", err)
	}
	if btc.count() != 1 {
		t.Fatalf("needs_reconcile row was re-sent: send count = %d, want 1", btc.count())
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("status after tick 2 = %q, want needs_reconcile", got)
	}
}
