package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	return New(logger.New(environments.Test))
}

// embedText flattens a swap-payout webhook body (a single Discord embed) into
// one searchable string: title + every field name/value. Lets the assertions
// below stay "does the message mention X" without coupling to embed layout.
func embedText(t *testing.T, body []byte) string {
	t.Helper()
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
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode embed body: %v (body=%s)", err, body)
	}
	if len(payload.Embeds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(payload.Embeds[0].Title + " " + payload.Embeds[0].Description)
	for _, f := range payload.Embeds[0].Fields {
		b.WriteString(" " + f.Name + " " + f.Value)
	}
	return b.String()
}

// SG-07: a completed payout posts the swap fields as a Discord webhook.
func TestCallSwapPayoutWebhook_Completed_PostsExpectedPayload(t *testing.T) {
	var (
		gotMethod      string
		gotContentType string
		gotBody        []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:     "completed",
		IcyAmount:  "2000000000000000000", // 2 ICY
		BtcAmount:  "95000",
		BtcAddress: "bc1qexampleaddr",
		BtcTxHash:  "btc-tx-hash-abc",
	})

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotContentType)
	}
	content := embedText(t, gotBody)
	// Compact: "Swap completed" title, formatted ICY (2e18 -> "2") and BTC
	// (95000 sats -> "0.00095"), the destination, and the tx in the mempool link.
	for _, want := range []string{"Swap completed", "2", "0.00095", "bc1qexampleaddr", "btc-tx-hash-abc"} {
		if !strings.Contains(content, want) {
			t.Fatalf("webhook embed %q missing %q", content, want)
		}
	}
}

// With no broadcast tx yet (an empty hash), the description carries no mempool
// link. The renderer is only ever handed completed events (the status gate lives
// in Telemetry.fireSwapPayoutWebhook), so it does not branch on status.
func TestCallSwapPayoutWebhook_EmptyTxHash_OmitsMempoolLink(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:     "completed",
		IcyAmount:  "2000000000000000000",
		BtcAmount:  "95000",
		BtcAddress: "bc1qexampleaddr",
		BtcTxHash:  "",
	})

	content := embedText(t, gotBody)
	for _, want := range []string{"Swap completed", "bc1qexampleaddr"} {
		if !strings.Contains(content, want) {
			t.Fatalf("webhook embed %q missing %q", content, want)
		}
	}
	if strings.Contains(content, "mempool.space") {
		t.Fatalf("embed %q should omit the mempool link when there is no tx hash", content)
	}
}

// Graceful degrade: no URL configured means no request is made and no error
// is raised, matching CallUptimeWebhook's existing contract.
func TestCallSwapPayoutWebhook_EmptyURL_NoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), "", SwapPayoutEvent{Status: "completed"})

	if called {
		t.Fatal("request was made despite empty webhook URL")
	}
}

// Failure isolation: an unreachable webhook endpoint must not panic or error
// out of the call; CallSwapPayoutWebhook has no error return by design (the
// caller's fire-and-forget guarantee depends on this never surfacing).
func TestCallSwapPayoutWebhook_UnreachableEndpoint_DoesNotPanic(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CallSwapPayoutWebhook panicked on unreachable endpoint: %v", r)
		}
	}()
	c.CallSwapPayoutWebhook(ctx, "http://127.0.0.1:1/unreachable", SwapPayoutEvent{Status: "completed"})
}

// A heartbeat ping that the monitor REJECTS must not look like a success. A
// swallowed 404 is how a dead monitor keeps looking healthy: the job pings, the
// monitor never registers it, and nobody learns the difference until an
// incident. The call still must not be fatal to the caller.
func TestCallUptimeWebhook_NonSuccessStatus_DoesNotPanic(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusForbidden} {
		var hit bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hit = true
			w.WriteHeader(code)
		}))

		// Must return normally: a monitor outage cannot break settlement.
		newTestClient(t).CallUptimeWebhook(context.Background(), srv.URL)

		if !hit {
			t.Fatalf("code %d: webhook was never called", code)
		}
		srv.Close()
	}
}

// The happy path still has to work: a 2xx registers and is not treated as a
// rejection.
func TestCallUptimeWebhook_Success_SendsGet(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	newTestClient(t).CallUptimeWebhook(context.Background(), srv.URL)

	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
}

// An unset URL is a no-op, not an error: monitoring is opt-in per deployment.
func TestCallUptimeWebhook_EmptyURL_NoRequest(t *testing.T) {
	newTestClient(t).CallUptimeWebhook(context.Background(), "")
}

func TestCallSwapPayoutWebhook_VaultBalance_AppendsFieldWithBtc(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:           "completed",
		BtcAmount:        "95000",
		VaultBalanceSats: "28768896",
	})

	content := embedText(t, gotBody)
	// 28768896 sats / 1e8 = 0.28768896 BTC, trailing zeros trimmed.
	for _, want := range []string{"Vault", "0.28768896 ₿"} {
		if !strings.Contains(content, want) {
			t.Fatalf("webhook embed %q missing %q", content, want)
		}
	}
}

func TestCallSwapPayoutWebhook_NoVaultBalance_OmitsField(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:    "completed",
		BtcAmount: "95000",
	})

	if strings.Contains(embedText(t, gotBody), "Vault") {
		t.Fatalf("webhook embed should omit the vault-balance field: %s", gotBody)
	}
}

func TestFormatUnits_ThousandsSeparators(t *testing.T) {
	cases := []struct{ raw string; dec int; want string }{
		{"2000000000000000000000", 18, "2,000"},
		{"1234567000000000000000000", 18, "1,234,567"},
		{"999000000000000000000", 18, "999"},
		{"123456789", 8, "1.23456789"},
		{"-50", 8, "-0.0000005"},
	}
	for _, c := range cases {
		if got := formatUnits(c.raw, c.dec); got != c.want {
			t.Fatalf("formatUnits(%q,%d) = %q, want %q", c.raw, c.dec, got, c.want)
		}
	}
}

func TestCallSwapPayoutWebhook_ShowsSwapperWallet(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	newTestClient(t).CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:      "completed",
		IcyAmount:   "2000000000000000000000",
		BtcAmount:   "123900",
		FromAddress: "0x1234567890abcdef1234567890abcdef12345678",
		BtcAddress:  "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq",
		BtcTxHash:   "abc",
	})
	content := embedText(t, gotBody)
	// commas on ICY, truncated EVM wallet, basescan link, truncated BTC dest.
	for _, want := range []string{"2,000", "0x123456", "basescan.org/address/0x1234567890abcdef", "bc1qar0"} {
		if !strings.Contains(content, want) {
			t.Fatalf("embed %q missing %q", content, want)
		}
	}
}
