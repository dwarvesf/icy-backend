package health

import (
	"errors"
	"testing"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
)

// stubOracle implements just enough of oracle.IOracle to drive checkSwapRate.
type stubOracle struct {
	oracle.IOracle // embedded: only the method under test is overridden
	err            error
}

func (s *stubOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &model.Web3BigInt{Value: "14394960000000000000000", Decimal: 18}, nil
}

// The regression this exists for: on 2026-07-20 /health/external reported
// healthy for over twenty minutes while /swap/info returned partial_data with
// no rate and swapping was paused. base_rpc passed because it makes ONE call;
// GetCirculatedICY needs eleven and was failing.
func TestCheckSwapRate_UnhealthyWhenRateUnavailable(t *testing.T) {
	h := &HealthHandler{
		oracle: &stubOracle{err: errors.New("circuit breaker is open")},
	}

	got := h.checkSwapRate()

	if got.Status != "unhealthy" {
		t.Fatalf("status = %q, want unhealthy: a health check that passes while swapping is paused suppresses the alarm", got.Status)
	}
	if got.Error == "" {
		t.Fatal("expected the underlying error to be surfaced")
	}
	if got.Metadata["impact"] == nil {
		t.Fatal("expected the user-facing impact to be stated")
	}
}

func TestCheckSwapRate_HealthyWhenRateAvailable(t *testing.T) {
	h := &HealthHandler{oracle: &stubOracle{}}

	if got := h.checkSwapRate(); got.Status != "healthy" {
		t.Fatalf("status = %q, want healthy", got.Status)
	}
}

// Never claim health we cannot observe.
func TestCheckSwapRate_UnknownWhenOracleMissing(t *testing.T) {
	h := &HealthHandler{}

	got := h.checkSwapRate()
	if got.Status == "healthy" {
		t.Fatal("a nil oracle must not report healthy")
	}
	if got.Status != "unknown" {
		t.Fatalf("status = %q, want unknown", got.Status)
	}
}
