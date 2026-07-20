package server

import (
	"testing"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
)

// A pre-POST failure releases the row to pending so a healthy later tick can
// retry. That is correct, but it had no bound: a permanently unsendable row
// (empty treasury, every endpoint down) re-failed on every tick forever, with
// the user's ICY already burned and no terminal state to alert on.
func TestProcessPending_RetryBudget_EventuallyGivesUp(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{
		// Always fails BEFORE the POST, so releasing is safe and the old code
		// would release forever.
		sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
			return "", 0, btcrpc.ErrNotBroadcast
		},
	}
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MaxBroadcastAttempts = 3
	tel := telWithBtc(t, db, btc, cfg)

	id := seed(t, db, model.BtcProcessingStatusPending, "50000", "3000")

	// Ticks 1 and 2 release for retry; tick 3 spends the budget.
	for i := 1; i <= 3; i++ {
		if err := tel.ProcessPendingBtcTransactions(); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		got := statusOf(t, db, id)
		if i < 3 && got != model.BtcProcessingStatusPending {
			t.Fatalf("tick %d: status = %q, want pending (budget not spent yet)", i, got)
		}
	}

	if got := statusOf(t, db, id); got != model.BtcProcessingStatusFailed {
		t.Fatalf("status = %q, want failed: the retry budget was spent and the row "+
			"must stop being retried, not spin forever", got)
	}

	// A terminal row is never picked up again, however many ticks run.
	before := btc.count()
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("post-terminal tick: %v", err)
	}
	if btc.count() != before {
		t.Fatalf("a failed row was retried: send count %d -> %d", before, btc.count())
	}
}

// The bound must not fire on a row that succeeds on a later attempt: a genuinely
// transient failure has to keep its retries.
func TestProcessPending_RetryBudget_TransientFailureStillRetries(t *testing.T) {
	db := newTestDB(t)
	calls := 0
	btc := &mockBtcRpc{
		sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
			calls++
			if calls == 1 {
				return "", 0, btcrpc.ErrNotBroadcast // one transient blip
			}
			return "btc-tx-hash", 100, nil
		},
	}
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MaxBroadcastAttempts = 3
	tel := telWithBtc(t, db, btc, cfg)

	id := seed(t, db, model.BtcProcessingStatusPending, "50000", "3000")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusPending {
		t.Fatalf("tick 1: status = %q, want pending", got)
	}
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("tick 2: status = %q, want completed: a transient failure must not "+
			"burn the row", got)
	}
}
