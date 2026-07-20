package baserpc

import (
	"testing"
	"time"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// newFailoverRPC builds a BaseRPC with only the fields switchEndpoint touches,
// no ethclient / network. switchEndpoint is a pure state machine over the
// endpoint index + failedEndpoints map, so it is directly unit-testable.
func newFailoverRPC(endpoints []string) *BaseRPC {
	return &BaseRPC{
		logger:          logger.New(environments.Test),
		endpoints:       endpoints,
		currentEndpoint: 0,
		failedEndpoints: make(map[string]*endpointStatus),
	}
}

// The failover fix (FilterSwapEvents wrapping withRetry) depends on
// switchEndpoint actually advancing off a failed endpoint. This is the exact
// rotation that was silently NOT happening for the swap indexer.
func TestSwitchEndpoint_SkipsFailedEndpoint(t *testing.T) {
	b := newFailoverRPC([]string{"ep-a", "ep-b"})
	// ep-a failed and is not yet due for retry, so switchEndpoint must land on ep-b.
	b.failedEndpoints["ep-a"] = &endpointStatus{failedAt: time.Now(), retryAfter: time.Hour}

	if err := b.switchEndpoint(); err != nil {
		t.Fatalf("switchEndpoint: %v", err)
	}
	if got := b.endpoints[b.currentEndpoint]; got != "ep-b" {
		t.Fatalf("currentEndpoint = %q, want ep-b", got)
	}
}

// A failed endpoint whose retryAfter has elapsed is eligible again.
func TestSwitchEndpoint_RetriesExpiredFailure(t *testing.T) {
	b := newFailoverRPC([]string{"ep-a", "ep-b"})
	b.currentEndpoint = 1 // start on ep-b so the next index is ep-a
	// ep-a failed long ago; its retry window has passed, so it can be tried again.
	b.failedEndpoints["ep-a"] = &endpointStatus{failedAt: time.Now().Add(-time.Hour), retryAfter: time.Minute}

	if err := b.switchEndpoint(); err != nil {
		t.Fatalf("switchEndpoint: %v", err)
	}
	if got := b.endpoints[b.currentEndpoint]; got != "ep-a" {
		t.Fatalf("currentEndpoint = %q, want ep-a (retry window elapsed)", got)
	}
}

// All endpoints failed and none is due for retry: switchEndpoint still advances
// ("hope for the best") rather than deadlocking on the current one.
func TestSwitchEndpoint_AllFailed_StillAdvances(t *testing.T) {
	b := newFailoverRPC([]string{"ep-a", "ep-b"})
	now := time.Now()
	b.failedEndpoints["ep-a"] = &endpointStatus{failedAt: now, retryAfter: time.Hour}
	b.failedEndpoints["ep-b"] = &endpointStatus{failedAt: now, retryAfter: time.Hour}

	start := b.currentEndpoint
	if err := b.switchEndpoint(); err != nil {
		t.Fatalf("switchEndpoint: %v", err)
	}
	if b.currentEndpoint == start {
		t.Fatalf("currentEndpoint stayed at %d; must advance even when all endpoints are failed", start)
	}
}

// A single endpoint cannot fail over: there is nowhere to go.
func TestSwitchEndpoint_SingleEndpoint_Errors(t *testing.T) {
	b := newFailoverRPC([]string{"ep-only"})
	if err := b.switchEndpoint(); err == nil {
		t.Fatal("expected an error switching with a single endpoint, got nil")
	}
}
