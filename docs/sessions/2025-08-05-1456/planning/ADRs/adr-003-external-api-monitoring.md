# ADR-003: External API Monitoring and Circuit Breaker Integration

**Date**: 2025-08-05  
**Status**: Proposed  
**Deciders**: Project Team  
**Context**: Phase 1 Monitoring Implementation  

## Context

The ICY Backend depends heavily on external APIs for cryptocurrency operations: Blockstream API for Bitcoin transactions and Base Chain RPC for ICY token interactions. These external dependencies are critical for system functionality and require robust monitoring, health checking, and circuit breaker patterns to ensure system resilience and proper error handling.

## Decision

### 1. Circuit Breaker Library Selection

**Decision**: Evaluate existing circuit breaker implementation or use `sony/gobreaker`

**Current State Analysis**:
- Research indicates possible existing circuit breaker implementation
- Need to assess current btcrpc and baserpc implementations
- If no existing implementation, implement using `sony/gobreaker`

**Rationale for sony/gobreaker**:
- **Production Proven**: Used in high-throughput production systems
- **Configurable**: Flexible threshold and timeout configuration
- **Lightweight**: Minimal performance overhead
- **Thread-Safe**: Safe for concurrent usage in Go
- **Monitoring**: Built-in state tracking for metrics

### 2. Circuit Breaker Configuration Strategy

**Configuration per External Service**:
```go
type CircuitBreakerConfig struct {
    MaxRequests      uint32        // Half-open state max requests
    Interval         time.Duration // Reset failure count interval
    Timeout          time.Duration // Open state duration  
    ConsecutiveFailureThreshold int // Failures before opening
}

var CircuitBreakerConfigs = map[string]CircuitBreakerConfig{
    "blockstream_api": {
        MaxRequests:      5,
        Interval:         30 * time.Second,
        Timeout:          60 * time.Second, 
        ConsecutiveFailureThreshold: 3,
    },
    "base_rpc": {
        MaxRequests:      3,
        Interval:         45 * time.Second,
        Timeout:          120 * time.Second,
        ConsecutiveFailureThreshold: 5,
    },
}
```

**State Definitions**:
- **Closed**: Normal operation, all requests allowed
- **Open**: Requests immediately failed after threshold reached
- **Half-Open**: Limited requests allowed to test service recovery

### 3. Monitoring Integration Strategy

**Health Check Integration**:
```go
func (h *HealthHandler) checkExternalAPIs(ctx context.Context) map[string]HealthCheck {
    checks := make(map[string]HealthCheck)
    
    // Check Bitcoin API with circuit breaker awareness
    checks["blockstream_api"] = h.checkBitcoinAPIHealth(ctx)
    
    // Check Base Chain RPC with circuit breaker awareness  
    checks["base_rpc"] = h.checkBaseRPCHealth(ctx)
    
    return checks
}

func (h *HealthHandler) checkBitcoinAPIHealth(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Check circuit breaker state first
    if h.btcCircuitBreaker.State() == gobreaker.StateOpen {
        return HealthCheck{
            Status: "unhealthy",
            Error:  "circuit breaker open",
            Metadata: map[string]interface{}{
                "circuit_state": "open",
            },
        }
    }
    
    // Use lightweight health check operation
    _, err := h.btcRPC.EstimateFees()
    if err != nil {
        return HealthCheck{
            Status:  "unhealthy",
            Error:   err.Error(),
            Latency: time.Since(start).Milliseconds(),
        }
    }
    
    return HealthCheck{
        Status:  "healthy",
        Latency: time.Since(start).Milliseconds(),
        Metadata: map[string]interface{}{
            "circuit_state": h.btcCircuitBreaker.State().String(),
        },
    }
}
```

### 4. Wrapper Implementation Pattern

**Circuit Breaker Wrapper for existing interfaces**:
```go
type CircuitBreakerBtcRPC struct {
    btcrpc.IBtcRpc
    circuitBreaker *gobreaker.CircuitBreaker
    metrics        *ExternalAPIMetrics
}

func NewCircuitBreakerBtcRPC(rpc btcrpc.IBtcRpc, config CircuitBreakerConfig) *CircuitBreakerBtcRPC {
    settings := gobreaker.Settings{
        Name:        "btc_rpc",
        MaxRequests: config.MaxRequests,
        Interval:    config.Interval,
        Timeout:     config.Timeout,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= config.ConsecutiveFailureThreshold
        },
        OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
            metrics.circuitBreakerState.WithLabelValues("btc_rpc").Set(float64(to))
            logger.Info("Circuit breaker state change", 
                map[string]string{
                    "service": name,
                    "from": from.String(),
                    "to": to.String(),
                })
        },
    }
    
    return &CircuitBreakerBtcRPC{
        IBtcRpc:        rpc,
        circuitBreaker: gobreaker.NewCircuitBreaker(settings),
        metrics:        NewExternalAPIMetrics(),
    }
}

func (cb *CircuitBreakerBtcRPC) Send(receiverAddress string, amount *model.Web3BigInt) (string, int64, error) {
    start := time.Now()
    defer func() {
        cb.metrics.externalAPIDuration.WithLabelValues("btc_rpc", "send").Observe(time.Since(start).Seconds())
    }()
    
    result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
        return cb.IBtcRpc.Send(receiverAddress, amount)
    })
    
    if err != nil {
        cb.metrics.externalAPICalls.WithLabelValues("btc_rpc", "error").Inc()
        return "", 0, err
    }
    
    cb.metrics.externalAPICalls.WithLabelValues("btc_rpc", "success").Inc()
    
    // Type assertion for the result
    sendResult := result.(SendResult) // Assuming SendResult struct
    return sendResult.TxHash, sendResult.Fee, nil
}
```

### 5. Timeout and Context Management

**Layered Timeout Strategy**:
```go
type TimeoutConfig struct {
    ConnectionTimeout time.Duration // TCP connection timeout
    RequestTimeout    time.Duration // Complete request timeout
    HealthCheckTimeout time.Duration // Health check specific timeout
}

var TimeoutConfigs = map[string]TimeoutConfig{
    "blockstream_api": {
        ConnectionTimeout:  2 * time.Second,
        RequestTimeout:     5 * time.Second,
        HealthCheckTimeout: 3 * time.Second,
    },
    "base_rpc": {
        ConnectionTimeout:  3 * time.Second,
        RequestTimeout:     10 * time.Second,
        HealthCheckTimeout: 5 * time.Second,
    },
}

func (cb *CircuitBreakerBtcRPC) executeWithTimeout(operation string, fn func() (interface{}, error)) (interface{}, error) {
    config := TimeoutConfigs["blockstream_api"]
    timeout := config.RequestTimeout
    
    if operation == "health_check" {
        timeout = config.HealthCheckTimeout
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    resultChan := make(chan interface{}, 1)
    errorChan := make(chan error, 1)
    
    go func() {
        result, err := fn()
        if err != nil {
            errorChan <- err
            return
        }
        resultChan <- result
    }()
    
    select {
    case result := <-resultChan:
        return result, nil
    case err := <-errorChan:
        return nil, err
    case <-ctx.Done():
        return nil, fmt.Errorf("operation timeout after %v", timeout)
    }
}
```

### 6. Error Classification and Logging

**Error Categories**:
```go
type APIErrorType string

const (
    ErrorTypeTimeout     APIErrorType = "timeout"
    ErrorTypeNetworkError APIErrorType = "network"
    ErrorTypeServerError  APIErrorType = "server_error"
    ErrorTypeClientError  APIErrorType = "client_error"
    ErrorTypeUnknown     APIErrorType = "unknown"
)

func classifyError(err error) APIErrorType {
    if err == nil {
        return ""
    }
    
    switch {
    case strings.Contains(err.Error(), "timeout"):
        return ErrorTypeTimeout
    case strings.Contains(err.Error(), "network"):
        return ErrorTypeNetworkError
    case strings.Contains(err.Error(), "5xx"):
        return ErrorTypeServerError
    case strings.Contains(err.Error(), "4xx"):
        return ErrorTypeClientError
    default:
        return ErrorTypeUnknown
    }
}

func (cb *CircuitBreakerBtcRPC) logAPICall(operation string, duration time.Duration, err error) {
    errorType := classifyError(err)
    
    logFields := map[string]string{
        "service":    "btc_rpc",
        "operation":  operation,
        "duration":   duration.String(),
        "cb_state":   cb.circuitBreaker.State().String(),
    }
    
    if err != nil {
        logFields["error"] = err.Error()
        logFields["error_type"] = string(errorType)
        cb.logger.Error("External API call failed", logFields)
    } else {
        cb.logger.Info("External API call successful", logFields)
    }
}
```

### 7. Metrics Integration

**External API Metrics**:
```go
type ExternalAPIMetrics struct {
    apiDuration      *prometheus.HistogramVec
    apiCalls         *prometheus.CounterVec
    circuitBreakerState *prometheus.GaugeVec
    timeouts         *prometheus.CounterVec
}

func NewExternalAPIMetrics() *ExternalAPIMetrics {
    return &ExternalAPIMetrics{
        apiDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_external_api_duration_seconds",
                Help: "External API call duration in seconds",
                Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
            },
            []string{"api_name", "operation"},
        ),
        apiCalls: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_external_api_calls_total",
                Help: "Total external API calls",
            },
            []string{"api_name", "status", "error_type"},
        ),
        circuitBreakerState: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_circuit_breaker_state",
                Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
            },
            []string{"api_name"},
        ),
        timeouts: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_external_api_timeouts_total",
                Help: "Total external API timeouts",
            },
            []string{"api_name", "timeout_type"}, // connection, request, health_check
        ),
    }
}
```

## Implementation Strategy

### 1. Integration Approach

**Wrapper Pattern Implementation**:
1. Create circuit breaker wrappers for existing `btcrpc.IBtcRpc` and `baserpc.IBaseRPC`
2. Replace direct interface usage with wrapped versions
3. Maintain backward compatibility with existing code
4. Add configuration options for circuit breaker parameters

**Dependency Injection**:
```go
// In server.go initialization
btcRpc := btcrpc.New(appConfig, logger)
btcRpcWithCB := NewCircuitBreakerBtcRPC(btcRpc, CircuitBreakerConfigs["blockstream_api"])

baseRpc, err := baserpc.New(appConfig, logger)
baseRpcWithCB := NewCircuitBreakerBaseRPC(baseRpc, CircuitBreakerConfigs["base_rpc"])

// Pass wrapped versions to handlers and services
oracle := oracle.New(db, s, appConfig, logger, btcRpcWithCB, baseRpcWithCB)
```

### 2. Testing Strategy

**Circuit Breaker Testing**:
- Unit tests for state transitions
- Integration tests with mock external APIs
- Chaos engineering tests with network failures
- Load testing with circuit breaker under stress

**Health Check Testing**:  
- Test health checks with various external API states
- Validate timeout handling and error responses
- Test circuit breaker state reporting in health endpoints

### 3. Configuration Management

**Environment-based Configuration**:
```go
type ExternalAPIConfig struct {
    CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
    Timeouts       TimeoutConfig        `json:"timeouts"`
    HealthCheck    HealthCheckConfig    `json:"health_check"`
}

// In config.go
type AppConfig struct {
    // ... existing fields
    ExternalAPIs map[string]ExternalAPIConfig `json:"external_apis"`
}
```

## Consequences

### Positive  
- **Resilience**: System continues operating during external API failures
- **Observability**: Comprehensive monitoring of external dependencies
- **Performance**: Circuit breakers prevent cascading failures and timeouts
- **Debugging**: Detailed logging and metrics for troubleshooting
- **Compatibility**: Wrapper pattern maintains existing interface contracts

### Negative
- **Complexity**: Additional code and configuration complexity
- **Testing**: More complex testing scenarios for circuit breaker states
- **Configuration**: Need to tune circuit breaker parameters for each API

### Risks and Mitigations
- **False Positives**: Circuit breakers might open unnecessarily
  - *Mitigation*: Careful threshold tuning and monitoring
- **Configuration Errors**: Wrong settings could impact functionality
  - *Mitigation*: Configuration validation and testing
- **Monitoring Noise**: Too many metrics and alerts
  - *Mitigation*: Thoughtful alert thresholds and metric grouping

## Alternatives Considered

1. **Hystrix-Go**
   - Pros: Netflix proven, feature rich
   - Cons: More complex, potentially overkill

2. **Custom Implementation**  
   - Pros: Full control, minimal dependencies
   - Cons: High development cost, potential bugs

3. **No Circuit Breakers**
   - Pros: Simpler implementation
   - Cons: No resilience against external failures

## References
- [Sony gobreaker Documentation](https://github.com/sony/gobreaker)
- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [Microservices Resilience Patterns](https://docs.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker)