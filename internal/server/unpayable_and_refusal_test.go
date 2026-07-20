package server

import (
	"testing"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
)

// A payout whose net amount is dust can never be broadcast: UTXO selection
// fails with fee > amount, which is an ErrNotBroadcast, which RELEASES the row
// back to pending. Nothing bounded that, so the row re-failed on every tick
// forever while the user's ICY stayed burned. It has to end terminally instead.
func TestProcessPending_DustAmount_TerminalNotReleased(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)

	// subtotal 3200, fee 3000 -> net 200 sat, under every dust limit.
	id := seed(t, db, model.BtcProcessingStatusPending, "3200", "3000")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("settlement tick: %v", err)
	}

	got := statusOf(t, db, id)
	if got == model.BtcProcessingStatusPending {
		t.Fatalf("status = %q: an unpayable row was released back to pending, so it "+
			"will re-fail on every tick forever", got)
	}
	if got != model.BtcProcessingStatusFailed {
		t.Fatalf("status = %q, want failed (terminal)", got)
	}
	if btc.count() != 0 {
		t.Fatalf("send count = %d, want 0: a dust payout must never reach Send", btc.count())
	}
}

// Zero specifically: the old guard was `amtInt < 0`, so a net of exactly zero
// sailed past it. subtotal == fee produces exactly that.
func TestProcessPending_ZeroNetAmount_TerminalNotReleased(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	tel := newTelemetry(t, db, btc)

	id := seed(t, db, model.BtcProcessingStatusPending, "3000", "3000") // net 0

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("settlement tick: %v", err)
	}

	if got := statusOf(t, db, id); got != model.BtcProcessingStatusFailed {
		t.Fatalf("status = %q, want failed: a zero payout passed the `< 0` guard "+
			"and reached Send", got)
	}
	if btc.count() != 0 {
		t.Fatalf("send count = %d, want 0", btc.count())
	}
}

// A payout refused by the daily cap was definitively never broadcast, so it
// must NOT count toward the rolling total. It used to be written as
// needs_reconcile, which IS counted, so each refusal inflated the very sum that
// caused it and the cap ratcheted itself shut for 24 hours with nothing sent.
func TestProcessPending_CapRefusal_DoesNotCountAsOutflow(t *testing.T) {
	db := newTestDB(t)
	btc := &mockBtcRpc{}
	cfg := &config.AppConfig{}
	cfg.Bitcoin.MaxDailyPayoutSatoshi = 10_000
	tel := telWithBtc(t, db, btc, cfg)

	// Over the cap on its own: refused, nothing sent.
	refused := seed(t, db, model.BtcProcessingStatusPending, "50000", "3000")
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if got := statusOf(t, db, refused); got != model.BtcProcessingStatusRefused {
		t.Fatalf("status = %q, want refused (a cap refusal is definitively not sent)", got)
	}
	if btc.count() != 0 {
		t.Fatalf("send count = %d, want 0", btc.count())
	}

	// The refusal must not have consumed any of the daily budget. A payout that
	// fits under the cap has to go through on the next tick.
	ok := seed(t, db, model.BtcProcessingStatusPending, "8000", "3000") // net 5000 < 10000
	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if got := statusOf(t, db, ok); got == model.BtcProcessingStatusRefused {
		t.Fatalf("a payout under the cap was refused: the earlier refusal was counted "+
			"as outflow, so the cap is ratcheting itself shut (status = %q)", got)
	}
	if btc.count() != 1 {
		t.Fatalf("send count = %d, want 1: the under-cap payout should have been sent", btc.count())
	}
}
