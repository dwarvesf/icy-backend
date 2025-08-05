# Rate Limiting Implementation Session - 2025-01-25 14:35

## Session Overview
**Start Time:** 2025-01-25 14:35  
**Session Type:** Implementation Planning & TDD Development  
**Focus:** HTTP 429 Rate Limiting Fix for ICY Backend Blockstream API Integration

## Goals
1. **Document comprehensive implementation plan** for fixing HTTP 429 rate limiting issues
2. **Implement TDD approach** for critical rate limiting components
3. **Ensure zero data loss** during rate limit scenarios
4. **Create robust monitoring and alerting** for production deployment

## Problem Context

### Current Issue
The ICY Backend is experiencing complete failure of Bitcoin transaction indexing when receiving HTTP 429 (Too Many Requests) errors from the Blockstream API. This critical issue causes:
- Complete system failure during background transaction processing
- Loss of financial transaction data integrity
- Service unreliability during peak usage periods

### Root Cause Analysis
Through detailed debugging, identified these specific issues:
1. **No 429-specific handling** - All HTTP errors treated equally
2. **Inadequate backoff strategy** - Linear 1s→2s→3s delays insufficient for rate limiting
3. **Aggressive API calling** - Rapid successive calls without throttling
4. **Complete failure propagation** - Rate limit errors terminate entire indexing process

## Detailed Implementation Plan

### PHASE 1: Immediate Rate Limit Handling (CRITICAL)

#### 1.1 Enhanced Retry Logic with 429 Handling
**File:** `internal/btcrpc/blockstream/blockstream.go:341-349`

**Current Implementation:**
```go
if resp.StatusCode != http.StatusOK {
    lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    // ... logging ...
    time.Sleep(time.Duration(attempt) * time.Second)  // LINEAR backoff: 1s, 2s, 3s
    continue
}
```

**Target Implementation:**
```go
if resp.StatusCode == http.StatusTooManyRequests {
    // Parse Retry-After header if present
    retryAfter := parseRetryAfterHeader(resp.Header.Get("Retry-After"))
    delay := max(retryAfter, calculateExponentialBackoff(attempt))
    
    c.logger.Warn("[GetTransactionsByAddress] Rate limit encountered", map[string]string{
        "attempt":    strconv.Itoa(attempt),
        "delay":      delay.String(),
        "statusCode": "429",
    })
    
    time.Sleep(delay)
    continue
} else if resp.StatusCode != http.StatusOK {
    // Regular error handling with shorter delays
    lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    time.Sleep(time.Duration(attempt) * time.Second)
    continue
}
```

**Exponential Backoff Strategy:**
- Attempt 1: 30 seconds
- Attempt 2: 60 seconds  
- Attempt 3: 120 seconds
- Attempt 4: 240 seconds
- Attempt 5: 300 seconds (max)

#### 1.2 Request-Level Throttling
**File:** `internal/btcrpc/blockstream/blockstream.go`

**Add Rate Limiter Structure:**
```go
type RateLimiter struct {
    lastRequest       time.Time
    requestDelay      time.Duration
    consecutiveErrors int
    circuitOpen       bool
    circuitOpenTime   time.Time
    mu                sync.RWMutex
}

func (rl *RateLimiter) Wait() {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    if rl.circuitOpen {
        if time.Since(rl.circuitOpenTime) > 10*time.Minute {
            rl.circuitOpen = false
            rl.consecutiveErrors = 0
        } else {
            // Circuit is open, reject request
            return errors.New("circuit breaker open")
        }
    }
    
    elapsed := time.Since(rl.lastRequest)
    if elapsed < rl.requestDelay {
        time.Sleep(rl.requestDelay - elapsed)
    }
    rl.lastRequest = time.Now()
}
```

#### 1.3 Graceful Degradation in Indexing
**File:** `internal/telemetry/btc.go:39-58`

**Current Loop (Failure-Prone):**
```go
for {
    markedTxs, err := t.btcRpc.GetTransactionsByAddress(address, markedTxHash)
    if err != nil {
        t.logger.Error("[IndexBtcTransaction][GetTransactionsByAddress]", map[string]string{
            "error": err.Error(),
        })
        return err  // COMPLETE FAILURE
    }
    // ... processing
}
```

**Target Implementation (Resilient):**
```go
for {
    markedTxs, err := t.btcRpc.GetTransactionsByAddress(address, markedTxHash)
    if err != nil {
        if isRateLimitError(err) {
            t.logger.Warn("[IndexBtcTransaction] Rate limit encountered, continuing with partial data", map[string]string{
                "error": err.Error(),
                "processed_so_far": strconv.Itoa(len(txs)),
            })
            break // Continue with what we have
        }
        // For non-rate-limit errors, still fail hard to prevent data corruption
        t.logger.Error("[IndexBtcTransaction][GetTransactionsByAddress]", map[string]string{
            "error": err.Error(),
        })
        return err
    }
    // ... processing continues
}
```

### PHASE 2: Configuration & Monitoring Foundation

#### 2.1 Configuration Structure
**File:** `internal/utils/config/config.go`

**Add to AppConfig:**
```go
type AppConfig struct {
    // ... existing fields ...
    RateLimit RateLimitConfig
}

type RateLimitConfig struct {
    RequestDelayMs       int     `env:"BTC_API_REQUEST_DELAY_MS"`
    MaxRetryDelayMs      int     `env:"BTC_API_MAX_RETRY_DELAY_MS"`
    RetryMultiplier      float64 `env:"BTC_API_RETRY_MULTIPLIER"`
    EnableCircuitBreaker bool    `env:"BTC_API_CIRCUIT_BREAKER"`
    CircuitBreakerTimeout int    `env:"BTC_API_CIRCUIT_TIMEOUT_MINUTES"`
}
```

**Environment Variables:**
```bash
# Rate limiting configuration
BTC_API_REQUEST_DELAY_MS=1000
BTC_API_MAX_RETRY_DELAY_MS=300000
BTC_API_RETRY_MULTIPLIER=2.0
BTC_API_CIRCUIT_BREAKER=true
BTC_API_CIRCUIT_TIMEOUT_MINUTES=10
```

#### 2.2 Enhanced Logging & Metrics
**Add Structured Logging:**
```go
// Rate limit event logging
t.logger.Info("[RateLimit] Event occurred", map[string]string{
    "event_type":    "rate_limit_encountered",
    "endpoint":      endpoint,
    "attempt":       strconv.Itoa(attempt),
    "backoff_delay": delay.String(),
    "timestamp":     time.Now().ISO8601(),
})

// Circuit breaker state changes
t.logger.Warn("[CircuitBreaker] State changed", map[string]string{
    "previous_state": "closed",
    "new_state":     "open",
    "trigger_error": err.Error(),
    "consecutive_failures": strconv.Itoa(failures),
})
```

### PHASE 3: Testing Strategy

#### 3.1 Test Structure
```
internal/btcrpc/blockstream/
├── blockstream_test.go
├── rate_limiter_test.go
├── test_helpers.go
└── mock_server_test.go

internal/telemetry/
├── btc_test.go
├── integration_test.go
└── rate_limit_scenarios_test.go
```

#### 3.2 Key Test Cases

**Rate Limiting Tests:**
1. **Single 429 Response**: Verify exponential backoff applied
2. **Multiple 429 Responses**: Verify progressive backoff increases
3. **Retry-After Header**: Verify server retry timing respected
4. **Circuit Breaker**: Verify opens after consecutive failures
5. **Circuit Recovery**: Verify auto-recovery after timeout

**Integration Tests:**
1. **Graceful Degradation**: IndexBtcTransaction continues with partial data
2. **Mixed Error Types**: 429 vs 500 vs network errors handled differently
3. **Configuration Loading**: All rate limit settings load correctly
4. **Monitoring Integration**: Metrics and logs generated properly

### PHASE 4: Deployment & Monitoring

#### 4.1 Deployment Strategy
```
Environment Flow:
Development → Staging → Production Canary → Full Production
     ↓           ↓           ↓                    ↓
Unit Tests   Load Tests   24hr Monitor      Gradual Rollout
Integration  Stress Test  Error Tracking    Full Monitoring
```

#### 4.2 Monitoring Metrics
**Key Performance Indicators:**
- API request success rate (target: >99%)
- Rate limit incident frequency (target: <5 per day)
- Mean time to recovery from rate limits (target: <5 minutes)
- Transaction indexing completeness (target: 100%)
- Circuit breaker activation count

**Alerting Thresholds:**
- Critical: Circuit breaker open >30 minutes
- Warning: Rate limit incidents >5 per hour
- Info: Unusual API response time patterns

## TDD Implementation Plan

### Test-First Development Approach
1. **Write failing test** for 429 response handling
2. **Implement minimal code** to make test pass
3. **Refactor** while keeping tests green
4. **Add next test case** and repeat

### First Test Case: Basic 429 Handling
```go
func TestBlockstream_GetTransactionsByAddress_RateLimitHandling(t *testing.T) {
    // Setup mock server that returns 429 on first call, 200 on second
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Implementation details in test
    }))
    
    // Test that 429 triggers exponential backoff and eventual success
    // Verify timing and retry behavior
}
```

## Progress Tracking

### Completed
- [x] Debug analysis and root cause identification
- [x] Comprehensive implementation plan creation
- [x] Session documentation setup

### In Progress
- [ ] TDD implementation starting with rate limit handling tests

### Next Steps
1. Begin TDD implementation of 429-specific retry logic
2. Create mock server for rate limit testing
3. Implement basic exponential backoff mechanism
4. Add configuration management
5. Create integration tests for telemetry layer

## Implementation Notes

### Critical Considerations
- **Financial Data Integrity**: Never compromise on transaction data completeness
- **Zero Downtime**: All changes must be backward compatible
- **Monitoring First**: Comprehensive observability before production deployment
- **Gradual Rollout**: Staged deployment with rollback capabilities

### Success Criteria
- Zero complete indexing failures due to rate limiting
- Sub-second recovery from temporary rate limit incidents
- Comprehensive monitoring and alerting in place
- Maintainable and testable codebase

---

**Session Status:** Active - Ready for TDD Implementation  
**Next Action:** Begin test-driven development of rate limiting components