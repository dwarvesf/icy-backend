package server

import (
	"testing"
	"time"

	"github.com/dwarvesf/icy-backend/internal/model"
)

// The confirm sweep has to run on EVERY settlement tick, not only on ticks that
// happen to find pending work.
//
// This is the gap that let a production bug through on 2026-07-20. The sweep was
// tail-called at the bottom of processPendingBtcTransactions, below its
// "no pending transactions" early return, so a payout broadcast into a quiet
// period was never promoted. A live swap confirmed on chain in 4 minutes and was
// still reported as "sending" 3.6 hours later; the stuck-tx detector shares the
// same sweep, so it was blind under exactly the conditions it exists for.
//
// The existing suite could not catch it: every confirmation test calls
// ConfirmBroadcastedBtcTransactions() directly, which proves the sweep WORKS but
// never that it gets CALLED. These tests go through the public entry point the
// cron actually drives, with the pending queue deliberately empty.

// A broadcasted payout that has confirmed must reach completed on a tick where
// there is nothing pending, because that is the normal state between swaps.
func TestProcessPending_QuietQueue_StillCompletesConfirmedPayout(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{} // unset confirmations means deeply confirmed
	tel := newTelemetry(t, db, btc)

	id := seedBroadcasted(t, db, "72621", "3000", "btc-tx-hash", time.Now().Add(-4*time.Minute))

	// No pending rows seeded on purpose: this is the early-return path.
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("settlement tick: %v", err)
	}

	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed. The confirm sweep did not run on a "+
			"tick with no pending work, so a confirmed payout stays 'broadcasted' "+
			"until another swap happens to drag the sweep along.", got)
	}
	if btc.count() != 0 {
		t.Fatalf("send count = %d, want 0: a quiet tick must not broadcast anything", btc.count())
	}
}

// The other half of the same wiring: a broadcast that never confirms has to be
// routed to needs_reconcile on a quiet tick too. This is the safety net, and it
// was equally unreachable.
func TestProcessPending_QuietQueue_StillFlagsStuckPayout(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{confirmations: 0, confHardcoded: true} // never confirms
	tel := newTelemetry(t, db, btc)

	// Broadcast long enough ago to be past any stuck timeout.
	id := seedBroadcasted(t, db, "72621", "3000", "btc-tx-hash", time.Now().Add(-72*time.Hour))

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("settlement tick: %v", err)
	}

	if got := statusOf(t, db, id); got == model.BtcProcessingStatusBroadcasted {
		t.Fatalf("status = %q: an unconfirmed payout was never assessed on a quiet "+
			"tick, so the stuck-tx detector cannot fire without unrelated traffic", got)
	}
	if btc.count() != 0 {
		t.Fatalf("send count = %d, want 0: a stuck payout is flagged for manual RBF, never auto-resent", btc.count())
	}
}
