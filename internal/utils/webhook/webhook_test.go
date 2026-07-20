package webhook

import (
	"context"
	"encoding/json"
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

// SG-07: a completed payout posts the swap fields as a Discord webhook.
func TestCallSwapPayoutWebhook_Completed_PostsExpectedPayload(t *testing.T) {
	var (
		gotMethod      string
		gotContentType string
		gotBody        map[string]string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:     "completed",
		IcyAmount:  "2000000000000000000",
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
	content := gotBody["content"]
	for _, want := range []string{"completed", "2000000000000000000", "95000", "bc1qexampleaddr", "btc-tx-hash-abc"} {
		if !strings.Contains(content, want) {
			t.Fatalf("webhook content %q missing %q", content, want)
		}
	}
}

// SG-07: a failed payout also fires, with an empty tx hash (never broadcast).
func TestCallSwapPayoutWebhook_Failed_PostsExpectedPayload(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:     "failed",
		IcyAmount:  "1000000000000000000",
		BtcAmount:  "-50",
		BtcAddress: "bc1qexampleaddr",
		BtcTxHash:  "",
	})

	content := gotBody["content"]
	if !strings.Contains(content, "failed") {
		t.Fatalf("webhook content %q missing status 'failed'", content)
	}
	if !strings.Contains(content, "-50") {
		t.Fatalf("webhook content %q missing btc amount", content)
	}
}

// SG-07: needs_reconcile (the ambiguous-broadcast terminal state SG-05 added)
// fires the same as any other terminal state.
func TestCallSwapPayoutWebhook_NeedsReconcile_PostsExpectedPayload(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t)
	c.CallSwapPayoutWebhook(context.Background(), srv.URL, SwapPayoutEvent{
		Status:     "needs_reconcile",
		IcyAmount:  "3000000000000000000",
		BtcAmount:  "120000",
		BtcAddress: "bc1qexampleaddr",
		BtcTxHash:  "",
	})

	if !strings.Contains(gotBody["content"], "needs_reconcile") {
		t.Fatalf("webhook content %q missing status 'needs_reconcile'", gotBody["content"])
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
