package server

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// SG-07: a Discord webhook fires whenever a BTC payout reaches a terminal
// settlement state (completed / failed / needs_reconcile), carrying the swap
// fields, so a drain or anomaly is visible instead of silent. These tests
// exercise the real orchestrator (ProcessPendingBtcTransactions), mirroring
// how SG-05's settlement_test.go tests it, because internal/telemetry's own
// test package does not build (pre-existing breakage, unrelated to SG-07;
// see docs/verification/swap-payout-webhook.md).

// captureWebhookServer records every POSTed Discord payload onto a channel so
// the fire-and-forget goroutine's output can be asserted without a fixed
// sleep. The payload is a Discord embed; each POST is flattened to title +
// field name/value text so the assertions below stay layout-agnostic.
func captureWebhookServer() (*httptest.Server, chan string) {
	ch := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Embeds []struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Fields      []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"fields"`
			} `json:"embeds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		var b strings.Builder
		if len(payload.Embeds) > 0 {
			b.WriteString(payload.Embeds[0].Title + " " + payload.Embeds[0].Description)
			for _, f := range payload.Embeds[0].Fields {
				b.WriteString(" " + f.Name + " " + f.Value)
			}
		}
		ch <- b.String()
		w.WriteHeader(http.StatusNoContent)
	}))
	return srv, ch
}

func waitForPayoutWebhook(t *testing.T, ch chan string) string {
	t.Helper()
	select {
	case body := <-ch:
		return body
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the swap payout webhook to fire")
		return ""
	}
}

func assertNoPayoutWebhook(t *testing.T, ch chan string) {
	t.Helper()
	select {
	case body := <-ch:
		t.Fatalf("unexpected swap payout webhook fired: %v", body)
	case <-time.After(150 * time.Millisecond):
		// expected: nothing arrived
	}
}

// Completed payout: fires with status=completed, the fee-adjusted BTC amount,
// the destination address, and the broadcast tx hash.
func TestProcessPending_Completed_FiresPayoutWebhook(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed", got)
	}

	content := waitForPayoutWebhook(t, ch)
	// 900 sats sendable -> "0.000009" BTC; address + tx in the description.
	for _, want := range []string{"Swap completed", "0.000009", "bc1qexampleaddr", "btc-tx-hash"} {
		if !strings.Contains(content, want) {
			t.Fatalf("webhook content %q missing %q", content, want)
		}
	}
}

// The vault-balance enrichment: a completed payout's webhook carries the
// treasury balance, in sats and converted to BTC, fetched from btcRpc at post
// time. (Both the architecture and test-coverage review flagged this path as
// running-but-unasserted.)
func TestProcessPending_Completed_WebhookCarriesVaultBalance(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{balanceSats: "123456789"} // 1.23456789 BTC
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed", got)
	}

	content := waitForPayoutWebhook(t, ch)
	for _, want := range []string{"Vault", "1.23456789 ₿"} {
		if !strings.Contains(content, want) {
			t.Fatalf("webhook content %q missing vault-balance %q", content, want)
		}
	}
}

// If the balance lookup fails, the webhook still fires but omits the
// vault-balance field (best-effort enrichment never blocks the notification).
func TestProcessPending_Completed_BalanceError_OmitsVaultField(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{balanceErr: fmt.Errorf("blockstream unreachable")}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed", got)
	}

	content := waitForPayoutWebhook(t, ch)
	if !strings.Contains(content, "Swap completed") {
		t.Fatalf("webhook content %q missing status", content)
	}
	if strings.Contains(content, "Vault") {
		t.Fatalf("webhook content %q should omit vault balance when the lookup errored", content)
	}
}

// Completed-only (operator decision 2026-07-21): a FAILED payout (unpayable
// row, empty BTC address) settles but fires NO webhook.
func TestProcessPending_UnpayableRow_NoWebhook(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)

	row := &model.OnchainBtcProcessedTransaction{
		BTCAddress: "", // unpayable
		Subtotal:   "1000",
		ServiceFee: "100",
		Status:     model.BtcProcessingStatusPending,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, row.ID); got != model.BtcProcessingStatusFailed {
		t.Fatalf("status = %q, want failed", got)
	}
	assertNoPayoutWebhook(t, ch)
}

// Completed-only: a needs_reconcile terminal state (ambiguous broadcast) settles
// but fires NO webhook.
func TestProcessPending_AmbiguousError_NoWebhook(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
		return "", 0, fmt.Errorf("broadcast response lost after node enqueue")
	}}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusNeedsReconcile {
		t.Fatalf("status = %q, want needs_reconcile", got)
	}
	assertNoPayoutWebhook(t, ch)
}

// Non-terminal release-to-pending (NotBroadcast error) must NOT fire a
// webhook: the row is retried, not settled, so there is nothing to notify yet.
func TestProcessPending_ReleasedToPending_NoWebhook(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{sendFn: func(addr string, amt *model.Web3BigInt) (string, int64, error) {
		return "", 0, fmt.Errorf("insufficient funds: %w", btcrpc.ErrNotBroadcast)
	}}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), btc, nil, nil)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusPending {
		t.Fatalf("status = %q, want pending (released for retry)", got)
	}

	assertNoPayoutWebhook(t, ch)
}

// Completed-only (operator decision 2026-07-21): detecting a swap and creating
// its pending payout row fires NO webhook; only the later completed settlement
// does. The swap-detected "pending" emit was removed.
func TestCreateBtcPayout_Detected_NoWebhook(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xswapok": big.NewInt(1234)},
	}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), nil, base, nil)

	if err := tel.CreateBtcPayoutForSwap(db, swapEvent("0xswapok", "1234", "5000")); err != nil {
		t.Fatalf("create: %v", err)
	}

	assertNoPayoutWebhook(t, ch)
}

// Negative control: a REJECTED swap (short ICY deposit) creates no payout row,
// so no swap-detected notification fires.
func TestCreateBtcPayout_ShortDeposit_NoWebhook(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	base := &mockBaseRpc{
		treasury:  treasuryAddr,
		deposited: map[string]*big.Int{"0xshort": big.NewInt(500)}, // < required 1000
	}
	cfg := &config.AppConfig{SwapPayoutWebhookURL: srv.URL}
	tel := telemetry.New(db, store.New(db), cfg, logger.New(environments.Test), nil, base, nil)

	if err := tel.CreateBtcPayoutForSwap(db, swapEvent("0xshort", "1000", "5000")); err != nil {
		t.Fatalf("create: %v", err)
	}

	assertNoPayoutWebhook(t, ch)
}

// Graceful degrade: no SwapPayoutWebhookURL configured means settlement still
// completes normally and no request is ever made.
func TestProcessPending_NoWebhookURLConfigured_SettlesWithoutNotifying(t *testing.T) {
	srv, ch := captureWebhookServer()
	defer srv.Close()

	db := newTestDB(t)
	btc := &mockBtcRpc{}
	// Deliberately empty AppConfig: no SwapPayoutWebhookURL set.
	tel := telemetry.New(db, store.New(db), &config.AppConfig{}, logger.New(environments.Test), btc, nil, nil)
	id := seed(t, db, model.BtcProcessingStatusPending, "1000", "100")

	if err := tel.ProcessPendingBtcTransactions(); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := statusOf(t, db, id); got != model.BtcProcessingStatusCompleted {
		t.Fatalf("status = %q, want completed (settlement unaffected by missing webhook config)", got)
	}

	assertNoPayoutWebhook(t, ch)
}
