# Task 2: External API Monitoring Implementation Specification

**Date**: 2025-08-05  
**Task**: External API Monitoring (Synthetic Monitoring)  
**Priority**: High  
**Estimated Effort**: 4-5 days  

## Overview

Implement comprehensive monitoring for external APIs (Blockstream Bitcoin API and Base Chain RPC) with circuit breaker integration, timeout management, error classification, and metrics collection. This provides synthetic monitoring of critical external dependencies.

## Functional Requirements

### 1. Circuit Breaker Integration

**Purpose**: Prevent cascading failures from external API issues  
**Strategy**: Implement circuit breaker wrapper pattern around existing RPC interfaces  
**Library**: `github.com/sony/gobreaker` or evaluate existing implementation  

**Circuit Breaker States**:
- **Closed**: Normal operation, requests pass through
- **Open**: Requests fail fast after threshold reached  
- **Half-Open**: Limited requests allowed to test recovery

### 2. External API Health Integration

**Integration**: Extend health check endpoints from Task 1 to include circuit breaker state  
**Monitoring**: Real-time circuit breaker state reporting  
**Alerting**: Circuit breaker state changes logged and exposed via metrics  

### 3. Timeout and Error Management

**Layered Timeouts**:
- Connection timeout: 2-3 seconds
- Request timeout: 5-10 seconds  
- Health check timeout: 3-5 seconds
- Circuit breaker timeout: 60-120 seconds

**Error Classification**:
- Network errors (connection failures)
- Timeout errors (request timeouts)
- Server errors (5xx responses)
- Client errors (4xx responses)
- Circuit breaker errors (requests blocked)

## Technical Specification

### 1. Circuit Breaker Configuration

```go
package monitoring

import (
    "time"
    "github.com/sony/gobreaker"
)

type CircuitBreakerConfig struct {
    Name                        string
    MaxRequests                 uint32        // Half-open state max requests
    Interval                    time.Duration // Reset failure count interval
    Timeout                     time.Duration // Open state duration
    ConsecutiveFailureThreshold int           // Failures before opening
    ReadyToTrip                 func(gobreaker.Counts) bool
    OnStateChange               func(name string, from gobreaker.State, to gobreaker.State)
}

var DefaultCircuitBreakerConfigs = map[string]CircuitBreakerConfig{
    "blockstream_api": {
        Name:                        "blockstream_api",
        MaxRequests:                 5,
        Interval:                    30 * time.Second,
        Timeout:                     60 * time.Second,
        ConsecutiveFailureThreshold: 3,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 3
        },
    },
    "base_rpc": {
        Name:                        "base_rpc",
        MaxRequests:                 3,
        Interval:                    45 * time.Second,
        Timeout:                     120 * time.Second,
        ConsecutiveFailureThreshold: 5,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 5
        },
    },
}
```

### 2. Circuit Breaker Wrapper for BTC RPC

```go
package monitoring

import (
    "context"
    "fmt"
    "time"
    
    "github.com/sony/gobreaker"
    "github.com/prometheus/client_golang/prometheus"
    
    "github.com/dwarvesf/icy-backend/internal/btcrpc"
    "github.com/dwarvesf/icy-backend/internal/model"
    "github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type CircuitBreakerBtcRPC struct {
    btcrpc.IBtcRpc
    circuitBreaker *gobreaker.CircuitBreaker
    logger         *logger.Logger
    metrics        *ExternalAPIMetrics
    config         CircuitBreakerConfig
}

func NewCircuitBreakerBtcRPC(
    rpc btcrpc.IBtcRpc,
    config CircuitBreakerConfig,
    logger *logger.Logger,
    metrics *ExternalAPIMetrics,
) *CircuitBreakerBtcRPC {
    
    settings := gobreaker.Settings{
        Name:        config.Name,
        MaxRequests: config.MaxRequests,
        Interval:    config.Interval,
        Timeout:     config.Timeout,
        ReadyToTrip: config.ReadyToTrip,
        OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
            // Update metrics
            metrics.circuitBreakerState.WithLabelValues("btc_rpc").Set(float64(to))
            
            // Log state change
            logger.Info("Circuit breaker state change", map[string]string{
                "service": name,
                "from":    from.String(),
                "to":      to.String(),
            })
        },
    }
    
    return &CircuitBreakerBtcRPC{
        IBtcRpc:        rpc,
        circuitBreaker: gobreaker.NewCircuitBreaker(settings),
        logger:         logger,
        metrics:        metrics,
        config:         config,
    }
}

func (cb *CircuitBreakerBtcRPC) Send(receiverAddress string, amount *model.Web3BigInt) (string, int64, error) {
    start := time.Now()
    operation := "send"
    
    result, err := cb.executeWithCircuitBreaker(operation, func() (interface{}, error) {
        return cb.IBtcRpc.Send(receiverAddress, amount)
    })
    
    duration := time.Since(start)
    cb.recordMetrics(operation, duration, err)
    
    if err != nil {
        return "", 0, err
    }
    
    // Type assertion - adjust based on actual return type
    sendResult := result.([]interface{})
    return sendResult[0].(string), sendResult[1].(int64), nil
}

func (cb *CircuitBreakerBtcRPC) CurrentBalance() (*model.Web3BigInt, error) {
    start := time.Now()
    operation := "current_balance"
    
    result, err := cb.executeWithCircuitBreaker(operation, func() (interface{}, error) {
        return cb.IBtcRpc.CurrentBalance()
    })
    
    duration := time.Since(start)
    cb.recordMetrics(operation, duration, err)
    
    if err != nil {
        return nil, err
    }
    
    return result.(*model.Web3BigInt), nil
}

func (cb *CircuitBreakerBtcRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error) {
    start := time.Now()
    operation := "get_transactions"
    
    result, err := cb.executeWithCircuitBreaker(operation, func() (interface{}, error) {
        return cb.IBtcRpc.GetTransactionsByAddress(address, fromTxId)
    })
    
    duration := time.Since(start)
    cb.recordMetrics(operation, duration, err)
    
    if err != nil {
        return nil, err
    }
    
    return result.([]model.OnchainBtcTransaction), nil
}

func (cb *CircuitBreakerBtcRPC) EstimateFees() (map[string]float64, error) {
    start := time.Now()
    operation := "estimate_fees"
    
    result, err := cb.executeWithCircuitBreaker(operation, func() (interface{}, error) {
        return cb.IBtcRpc.EstimateFees()
    })
    
    duration := time.Since(start)
    cb.recordMetrics(operation, duration, err)
    
    if err != nil {
        return nil, err
    }
    
    return result.(map[string]float64), nil
}

func (cb *CircuitBreakerBtcRPC) GetSatoshiUSDPrice() (float64, error) {
    start := time.Now()
    operation := "get_price"
    
    result, err := cb.executeWithCircuitBreaker(operation, func() (interface{}, error) {
        return cb.IBtcRpc.GetSatoshiUSDPrice()
    })
    
    duration := time.Since(start)
    cb.recordMetrics(operation, duration, err)
    
    if err != nil {
        return 0, err
    }
    
    return result.(float64), nil
}

func (cb *CircuitBreakerBtcRPC) IsDust(address string, amount int64) bool {
    // This method doesn't make external calls, so no circuit breaker needed
    return cb.IBtcRpc.IsDust(address, amount)
}

func (cb *CircuitBreakerBtcRPC) executeWithCircuitBreaker(operation string, fn func() (interface{}, error)) (interface{}, error) {
    result, err := cb.circuitBreaker.Execute(fn)
    
    if err != nil {
        // Log the error with operation context
        errorType := cb.classifyError(err)
        cb.logger.Error("BTC RPC operation failed", map[string]string{
            "operation":           operation,
            "error":              err.Error(),
            "error_type":         string(errorType),
            "circuit_state":      cb.circuitBreaker.State().String(),
        })
    }
    
    return result, err
}

func (cb *CircuitBreakerBtcRPC) recordMetrics(operation string, duration time.Duration, err error) {
    // Record duration
    cb.metrics.externalAPIDuration.WithLabelValues("btc_rpc", operation).Observe(duration.Seconds())
    
    // Record call count
    status := "success"
    errorType := ""
    
    if err != nil {
        status = "error"
        errorType = string(cb.classifyError(err))
    }
    
    cb.metrics.externalAPICalls.WithLabelValues("btc_rpc", status, errorType).Inc()
}

func (cb *CircuitBreakerBtcRPC) classifyError(err error) APIErrorType {
    if err == nil {
        return ""
    }
    
    errStr := err.Error()
    switch {
    case strings.Contains(errStr, "timeout"), strings.Contains(errStr, "deadline exceeded"):
        return ErrorTypeTimeout
    case strings.Contains(errStr, "connection"), strings.Contains(errStr, "network"):
        return ErrorTypeNetworkError
    case strings.Contains(errStr, "circuit breaker is open"):
        return ErrorTypeCircuitBreakerOpen
    case strings.Contains(errStr, "5xx"), strings.Contains(errStr, "server error"):
        return ErrorTypeServerError
    case strings.Contains(errStr, "4xx"), strings.Contains(errStr, "client error"):
        return ErrorTypeClientError
    default:
        return ErrorTypeUnknown
    }
}

func (cb *CircuitBreakerBtcRPC) GetCircuitBreakerState() gobreaker.State {
    return cb.circuitBreaker.State()
}

func (cb *CircuitBreakerBtcRPC) GetCircuitBreakerCounts() gobreaker.Counts {
    return cb.circuitBreaker.Counts()
}
```

### 3. Circuit Breaker Wrapper for Base RPC

```go
package monitoring

type CircuitBreakerBaseRPC struct {
    baserpc.IBaseRPC
    circuitBreaker *gobreaker.CircuitBreaker
    logger         *logger.Logger
    metrics        *ExternalAPIMetrics
    config         CircuitBreakerConfig
}

func NewCircuitBreakerBaseRPC(
    rpc baserpc.IBaseRPC,
    config CircuitBreakerConfig,
    logger *logger.Logger,
    metrics *ExternalAPIMetrics,
) *CircuitBreakerBaseRPC {
    
    settings := gobreaker.Settings{
        Name:        config.Name,
        MaxRequests: config.MaxRequests,
        Interval:    config.Interval,
        Timeout:     config.Timeout,
        ReadyToTrip: config.ReadyToTrip,
        OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
            metrics.circuitBreakerState.WithLabelValues("base_rpc").Set(float64(to))
            
            logger.Info("Circuit breaker state change", map[string]string{
                "service": name,
                "from":    from.String(),
                "to":      to.String(),
            })
        },
    }
    
    return &CircuitBreakerBaseRPC{
        IBaseRPC:       rpc,
        circuitBreaker: gobreaker.NewCircuitBreaker(settings),
        logger:         logger,
        metrics:        metrics,
        config:         config,
    }
}

// Implement all IBaseRPC methods with circuit breaker pattern
// Similar to BTC RPC implementation above
func (cb *CircuitBreakerBaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
    start := time.Now()
    operation := "icy_balance_of"
    
    result, err := cb.executeWithCircuitBreaker(operation, func() (interface{}, error) {
        return cb.IBaseRPC.ICYBalanceOf(address)
    })
    
    duration := time.Since(start)
    cb.recordMetrics(operation, duration, err)
    
    if err != nil {
        return nil, err
    }
    
    return result.(*model.Web3BigInt), nil
}

// ... implement other methods with similar pattern
```

### 4. External API Metrics

```go
package monitoring

type APIErrorType string

const (
    ErrorTypeTimeout           APIErrorType = "timeout"
    ErrorTypeNetworkError      APIErrorType = "network"
    ErrorTypeServerError       APIErrorType = "server_error"
    ErrorTypeClientError       APIErrorType = "client_error"
    ErrorTypeCircuitBreakerOpen APIErrorType = "circuit_breaker_open"
    ErrorTypeUnknown           APIErrorType = "unknown"
)

type ExternalAPIMetrics struct {
    externalAPIDuration   *prometheus.HistogramVec
    externalAPICalls      *prometheus.CounterVec
    circuitBreakerState   *prometheus.GaugeVec
    timeouts              *prometheus.CounterVec
    circuitBreakerTrips   *prometheus.CounterVec
}

func NewExternalAPIMetrics() *ExternalAPIMetrics {
    return &ExternalAPIMetrics{
        externalAPIDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_external_api_duration_seconds",
                Help: "External API call duration in seconds",
                Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
            },
            []string{"api_name", "operation"},
        ),
        externalAPICalls: prometheus.NewCounterVec(
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
            []string{"api_name", "operation"},
        ),
        circuitBreakerTrips: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_circuit_breaker_trips_total",
                Help: "Total circuit breaker trips",
            },
            []string{"api_name"},
        ),
    }
}

func (m *ExternalAPIMetrics) MustRegister(registry *prometheus.Registry) {
    registry.MustRegister(
        m.externalAPIDuration,
        m.externalAPICalls,
        m.circuitBreakerState,
        m.timeouts,
        m.circuitBreakerTrips,
    )
}
```

### 5. Enhanced Health Check Integration

Update health handler from Task 1 to include circuit breaker state:

```go
func (h *HealthHandler) checkBitcoinAPI(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Get circuit breaker wrapper if available
    if cbRPC, ok := h.btcRPC.(*monitoring.CircuitBreakerBtcRPC); ok {
        cbState := cbRPC.GetCircuitBreakerState()
        cbCounts := cbRPC.GetCircuitBreakerCounts()
        
        // Check circuit breaker state first
        if cbState == gobreaker.StateOpen {
            return HealthCheck{
                Status: string(HealthStatusUnhealthy),
                Error:  "circuit breaker open",
                Metadata: map[string]interface{}{
                    "circuit_state":      cbState.String(),
                    "failure_count":      cbCounts.TotalFailures,
                    "success_count":      cbCounts.TotalSuccesses,
                    "consecutive_failures": cbCounts.ConsecutiveFailures,
                },
            }
        }
        
        // Perform lightweight health check
        done := make(chan error, 1)
        go func() {
            _, err := h.btcRPC.EstimateFees()
            done <- err
        }()
        
        select {
        case err := <-done:
            latency := time.Since(start).Milliseconds()
            
            if err != nil {
                return HealthCheck{
                    Status:  string(HealthStatusUnhealthy),
                    Error:   err.Error(),
                    Latency: latency,
                    Metadata: map[string]interface{}{
                        "endpoint":               "blockstream.info",
                        "circuit_state":          cbState.String(),
                        "consecutive_failures":   cbCounts.ConsecutiveFailures,
                    },
                }
            }
            
            return HealthCheck{
                Status:  string(HealthStatusHealthy),
                Latency: latency,
                Metadata: map[string]interface{}{
                    "endpoint":       "blockstream.info",
                    "circuit_state":  cbState.String(),
                    "last_success":   time.Now().Format(time.RFC3339),
                    "success_count":  cbCounts.TotalSuccesses,
                },
            }
            
        case <-ctx.Done():
            return HealthCheck{
                Status:  string(HealthStatusUnhealthy),
                Error:   "health check timeout",
                Latency: time.Since(start).Milliseconds(),
                Metadata: map[string]interface{}{
                    "endpoint":      "blockstream.info",
                    "circuit_state": cbState.String(),
                },
            }
        }
    }
    
    // Fallback to original implementation if no circuit breaker
    return h.checkBitcoinAPIOriginal(ctx)
}
```

## Integration Requirements

### 1. Server Initialization

Update server initialization to use circuit breaker wrappers:

```go
// In internal/server/server.go
func Init() {
    // ... existing initialization
    
    // Create external API metrics
    externalAPIMetrics := monitoring.NewExternalAPIMetrics()
    
    // Create base RPC instances
    btcRpc := btcrpc.New(appConfig, logger)
    baseRpc, err := baserpc.New(appConfig, logger)
    if err != nil {
        logger.Error("[Init][baserpc.New] failed to init base rpc", map[string]string{
            "error": err.Error(),
        })
        return
    }
    
    // Wrap with circuit breakers
    btcRpcWithCB := monitoring.NewCircuitBreakerBtcRPC(
        btcRpc,
        monitoring.DefaultCircuitBreakerConfigs["blockstream_api"],
        logger,
        externalAPIMetrics,
    )
    
    baseRpcWithCB := monitoring.NewCircuitBreakerBaseRPC(
        baseRpc,
        monitoring.DefaultCircuitBreakerConfigs["base_rpc"],
        logger,
        externalAPIMetrics,
    )
    
    // Use wrapped versions in downstream components
    oracle := oracle.New(db, s, appConfig, logger, btcRpcWithCB, baseRpcWithCB)
    
    telemetryInstance := telemetry.New(
        db,
        s,
        appConfig,
        logger,
        btcRpcWithCB,
        baseRpcWithCB,
        oracle,
    )
    
    // ... rest of initialization
    httpServer := http.NewHttpServer(appConfig, logger, oracle, baseRpcWithCB, btcRpcWithCB, db)
    httpServer.Run()
}
```

### 2. Configuration Management

Add circuit breaker configuration to app config:

```go
// In internal/utils/config/config.go
type CircuitBreakerSettings struct {
    MaxRequests                 uint32        `json:"max_requests"`
    IntervalSeconds             int           `json:"interval_seconds"`
    TimeoutSeconds              int           `json:"timeout_seconds"`
    ConsecutiveFailureThreshold int           `json:"consecutive_failure_threshold"`
}

type ExternalAPIConfig struct {
    BlockstreamAPI CircuitBreakerSettings `json:"blockstream_api"`
    BaseRPC        CircuitBreakerSettings `json:"base_rpc"`
}

type AppConfig struct {
    // ... existing fields
    ExternalAPIs ExternalAPIConfig `json:"external_apis"`
}
```

## Testing Requirements

### 1. Unit Tests

**File**: `internal/monitoring/circuit_breaker_test.go`

```go
func TestCircuitBreakerBtcRPC_NormalOperation(t *testing.T) {
    // Test normal operation with circuit breaker closed
}

func TestCircuitBreakerBtcRPC_FailureThreshold(t *testing.T) {
    // Test circuit breaker opens after consecutive failures
}

func TestCircuitBreakerBtcRPC_Recovery(t *testing.T) {
    // Test circuit breaker transitions from open to half-open to closed
}

func TestCircuitBreakerBtcRPC_MetricsCollection(t *testing.T) {
    // Test metrics are properly recorded
}

func TestCircuitBreakerBtcRPC_ErrorClassification(t *testing.T) {
    // Test error types are properly classified
}
```

### 2. Integration Tests

**File**: `internal/monitoring/circuit_breaker_integration_test.go`

Test circuit breaker behavior with real external API calls and network failures.

### 3. Load Tests

Test circuit breaker behavior under high load and verify metrics accuracy.

## Performance Requirements

- Circuit breaker decision overhead: < 0.1ms per request
- Metrics collection overhead: < 0.1ms per external API call
- Memory usage: < 1MB for circuit breaker state and metrics
- No impact on existing API response times under normal conditions

## Documentation Requirements

### 1. Operational Documentation

- Circuit breaker configuration guide
- Troubleshooting guide for circuit breaker states
- Metrics interpretation guide
- Alert configuration recommendations

### 2. API Documentation

Update health check endpoint documentation to include circuit breaker information.

## Acceptance Criteria

- [ ] Circuit breaker wrappers implemented for both BTC and Base RPC
- [ ] Circuit breaker state transitions work correctly (closed -> open -> half-open -> closed)
- [ ] External API calls are properly instrumented with metrics
- [ ] Health check endpoints report circuit breaker status
- [ ] Error classification works for different error types
- [ ] Configuration allows tuning circuit breaker parameters
- [ ] Performance overhead meets requirements
- [ ] Comprehensive logging for debugging
- [ ] Unit test coverage > 90%
- [ ] Integration tests validate circuit breaker behavior
- [ ] Load tests confirm performance under stress