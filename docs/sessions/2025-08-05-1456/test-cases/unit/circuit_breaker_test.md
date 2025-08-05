# Circuit Breaker Unit Test Cases

**File**: `internal/monitoring/circuit_breaker_test.go`  
**Package**: `monitoring`  
**Target Coverage**: >95%  

## Test Suite Overview

Comprehensive unit tests for circuit breaker implementation covering state transitions, timeout handling, error classification, metrics integration, and wrapper functionality for external API calls. Focus on resilience patterns and cryptocurrency system reliability.

## Test Cases

### 1. Circuit Breaker State Transitions

#### TestCircuitBreaker_InitialState
```go
func TestCircuitBreaker_InitialState(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    mockBtcRPC := setupMockBtcRPC(t)
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Act & Assert
    assert.Equal(t, gobreaker.StateClosed, cb.circuitBreaker.State())
    
    // Verify initial metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_circuit_breaker_state" {
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(gobreaker.StateClosed), metric.GetGauge().GetValue())
        }
    }
}
```

#### TestCircuitBreaker_ClosedToOpen
```go
func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("API unavailable"))
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Act - Trigger consecutive failures
    var lastErr error
    for i := 0; i < 3; i++ {
        _, _, lastErr = cb.Send("test_address", createWeb3BigInt("1000"))
        assert.Error(t, lastErr)
    }
    
    // Assert - Circuit breaker should be open
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
    
    // Verify metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    errorCountFound := false
    stateFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_external_api_calls_total":
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "status") == "error" {
                    errorCountFound = true
                    assert.Equal(t, float64(3), metric.GetCounter().GetValue())
                }
            }
        case "icy_backend_circuit_breaker_state":
            stateFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(gobreaker.StateOpen), metric.GetGauge().GetValue())
        }
    }
    
    assert.True(t, errorCountFound, "Error count metric not found")
    assert.True(t, stateFound, "Circuit breaker state metric not found")
}
```

#### TestCircuitBreaker_OpenToHalfOpen
```go
func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    2,
        Interval:                      30 * time.Second,
        Timeout:                       100 * time.Millisecond, // Short timeout for testing
        ConsecutiveFailureThreshold:   2,
    }
    
    metrics := NewExternalAPIMetrics()
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("API unavailable"))
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Force circuit breaker to open
    for i := 0; i < 2; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
    
    // Act - Wait for timeout to transition to half-open
    time.Sleep(150 * time.Millisecond) // Wait longer than timeout
    
    // Trigger a call to test half-open state
    _, _, _ = cb.Send("test_address", createWeb3BigInt("1000"))
    
    // Assert - Should be in half-open state now
    assert.Equal(t, gobreaker.StateHalfOpen, cb.circuitBreaker.State())
}
```

#### TestCircuitBreaker_HalfOpenToClosed
```go
func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    2,
        Interval:                      30 * time.Second,
        Timeout:                       100 * time.Millisecond,
        ConsecutiveFailureThreshold:   2,
    }
    
    metrics := NewExternalAPIMetrics()
    
    // Setup mock that fails first, then succeeds
    mockBtcRPC := setupMockBtcRPCWithPattern(t, []error{
        errors.New("fail"), errors.New("fail"), // Force open
        nil, nil, // Success to close circuit
    })
    
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Force to open state
    for i := 0; i < 2; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
    
    // Wait for half-open transition
    time.Sleep(150 * time.Millisecond)
    
    // Act - Successful calls in half-open state
    for i := 0; i < 2; i++ {
        txHash, fee, err := cb.Send("test_address", createWeb3BigInt("1000"))
        assert.NoError(t, err)
        assert.NotEmpty(t, txHash)
        assert.True(t, fee > 0)
    }
    
    // Assert - Should return to closed state
    assert.Equal(t, gobreaker.StateClosed, cb.circuitBreaker.State())
}
```

#### TestCircuitBreaker_HalfOpenToOpen
```go
func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    2,
        Interval:                      30 * time.Second,
        Timeout:                       100 * time.Millisecond,
        ConsecutiveFailureThreshold:   2,
    }
    
    metrics := NewExternalAPIMetrics()
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("still failing"))
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Force to open state
    for i := 0; i < 2; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
    
    // Wait for half-open transition
    time.Sleep(150 * time.Millisecond)
    
    // Act - Fail in half-open state
    _, _, err := cb.Send("test_address", createWeb3BigInt("1000"))
    assert.Error(t, err)
    
    // Assert - Should return to open state
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
}
```

### 2. Circuit Breaker Integration Tests

#### TestCircuitBreakerBtcRPC_Send_Success
```go
func TestCircuitBreakerBtcRPC_Send_Success(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    expectedTxHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    expectedFee := int64(1500)
    
    mockBtcRPC := setupMockSuccessfulBtcRPC(t, expectedTxHash, expectedFee)
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Act
    start := time.Now()
    txHash, fee, err := cb.Send("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", createWeb3BigInt("100000"))
    duration := time.Since(start)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, expectedTxHash, txHash)
    assert.Equal(t, expectedFee, fee)
    assert.Equal(t, gobreaker.StateClosed, cb.circuitBreaker.State())
    
    // Verify metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    successCountFound := false
    durationFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_external_api_calls_total":
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "status") == "success" {
                    successCountFound = true
                    assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                }
            }
        case "icy_backend_external_api_duration_seconds":
            durationFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
            assert.True(t, metric.GetHistogram().GetSampleSum() > 0)
        }
    }
    
    assert.True(t, successCountFound, "Success count metric not found")
    assert.True(t, durationFound, "Duration metric not found")
}
```

#### TestCircuitBreakerBtcRPC_EstimateFees_CircuitOpen
```go
func TestCircuitBreakerBtcRPC_EstimateFees_CircuitOpen(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   2,
    }
    
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("network error"))
    metrics := NewExternalAPIMetrics()
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Force circuit breaker to open
    for i := 0; i < 2; i++ {
        cb.EstimateFees()
    }
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
    
    // Act - Call when circuit is open
    fees, err := cb.EstimateFees()
    
    // Assert - Should fail immediately with circuit breaker error
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "circuit breaker is open")
    assert.Nil(t, fees)
}
```

#### TestCircuitBreakerBaseRPC_ICYTotalSupply_Success
```go
func TestCircuitBreakerBaseRPC_ICYTotalSupply_Success(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,  
        Timeout:                       120 * time.Second,
        ConsecutiveFailureThreshold:   5,
    }
    
    expectedSupply := createWeb3BigInt("1000000000000000000000000") // 1M ICY tokens
    mockBaseRPC := setupMockSuccessfulBaseRPC(t, expectedSupply)
    metrics := NewExternalAPIMetrics()
    
    cb := NewCircuitBreakerBaseRPC(mockBaseRPC, config, metrics, setupTestLogger())
    
    // Act
    supply, err := cb.ICYTotalSupply()
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, expectedSupply.String(), supply.String())
    assert.Equal(t, gobreaker.StateClosed, cb.circuitBreaker.State())
}
```

### 3. Timeout Handling Tests

#### TestCircuitBreaker_RequestTimeout
```go
func TestCircuitBreaker_RequestTimeout(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    timeoutConfig := TimeoutConfig{
        ConnectionTimeout:  2 * time.Second,
        RequestTimeout:     500 * time.Millisecond, // Short timeout
        HealthCheckTimeout: 3 * time.Second,
    }
    
    // Mock that takes longer than request timeout
    mockBtcRPC := setupMockSlowBtcRPC(t, 1*time.Second)
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    cb := NewCircuitBreakerBtcRPCWithTimeout(mockBtcRPC, config, timeoutConfig, metrics, setupTestLogger())
    
    // Act
    start := time.Now()
    _, _, err := cb.Send("test_address", createWeb3BigInt("1000"))
    duration := time.Since(start)
    
    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
    assert.True(t, duration >= 500*time.Millisecond && duration < 800*time.Millisecond,
        "Request did not timeout correctly: %v", duration)
    
    // Verify timeout metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_external_api_timeouts_total" {
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetCounter().GetValue())
            
            labels := metric.GetLabel()
            assert.Equal(t, "btc_rpc", getLabelValue(labels, "api_name"))
            assert.Equal(t, "request", getLabelValue(labels, "timeout_type"))
        }
    }
}
```

#### TestCircuitBreaker_HealthCheckTimeout
```go
func TestCircuitBreaker_HealthCheckTimeout(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    timeoutConfig := TimeoutConfig{
        ConnectionTimeout:  2 * time.Second,
        RequestTimeout:     5 * time.Second,
        HealthCheckTimeout: 200 * time.Millisecond, // Short health check timeout
    }
    
    mockBtcRPC := setupMockSlowBtcRPC(t, 500*time.Millisecond) // Slower than health check timeout
    metrics := NewExternalAPIMetrics()
    cb := NewCircuitBreakerBtcRPCWithTimeout(mockBtcRPC, config, timeoutConfig, metrics, setupTestLogger())
    
    // Act - Use health check operation
    start := time.Now()
    _, err := cb.executeWithTimeout("health_check", func() (interface{}, error) {
        return cb.EstimateFees()
    })
    duration := time.Since(start)
    
    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
    assert.True(t, duration >= 200*time.Millisecond && duration < 400*time.Millisecond,
        "Health check did not timeout correctly: %v", duration)
}
```

### 4. Error Classification Tests

#### TestErrorClassification_NetworkErrors
```go
func TestErrorClassification_NetworkErrors(t *testing.T) {
    tests := []struct {
        name          string
        error         error
        expectedType  APIErrorType
    }{
        {
            name:         "Timeout error",
            error:        errors.New("request timeout after 5s"),
            expectedType: ErrorTypeTimeout,
        },
        {
            name:         "Network error",
            error:        errors.New("network unreachable"),
            expectedType: ErrorTypeNetworkError,
        },
        {
            name:         "Server error",
            error:        errors.New("HTTP 500 Internal Server Error"),
            expectedType: ErrorTypeServerError,
        },
        {
            name:         "Client error",
            error:        errors.New("HTTP 400 Bad Request"),
            expectedType: ErrorTypeClientError,
        },
        {
            name:         "Unknown error",
            error:        errors.New("unexpected error occurred"),
            expectedType: ErrorTypeUnknown,
        },
        {
            name:         "Nil error",
            error:        nil,
            expectedType: "",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := classifyError(tt.error)
            assert.Equal(t, tt.expectedType, result)
        })
    }
}
```

#### TestCircuitBreaker_ErrorLogging
```go
func TestCircuitBreaker_ErrorLogging(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    testLogger := setupTestLoggerWithCapture(t)
    mockBtcRPC := setupMockBtcRPCWithSpecificError(t, errors.New("API rate limit exceeded"))
    metrics := NewExternalAPIMetrics()
    
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, testLogger)
    
    // Act
    _, _, err := cb.Send("test_address", createWeb3BigInt("1000"))
    
    // Assert
    assert.Error(t, err)
    
    // Verify logging
    logEntries := testLogger.GetCapturedLogs()
    assert.True(t, len(logEntries) > 0)
    
    errorLogFound := false
    for _, entry := range logEntries {
        if entry.Level == "ERROR" && strings.Contains(entry.Message, "External API call failed") {
            errorLogFound = true
            assert.Contains(t, entry.Fields, "service")
            assert.Contains(t, entry.Fields, "operation")
            assert.Contains(t, entry.Fields, "duration")
            assert.Contains(t, entry.Fields, "error")
            assert.Contains(t, entry.Fields, "error_type")
            assert.Contains(t, entry.Fields, "cb_state")
            
            assert.Equal(t, "btc_rpc", entry.Fields["service"])
            assert.Equal(t, "client_error", entry.Fields["error_type"]) // Rate limit is client error
            break
        }
    }
    
    assert.True(t, errorLogFound, "Error log entry not found")
}
```

### 5. Concurrent Access Tests

#### TestCircuitBreaker_ConcurrentCalls
```go
func TestCircuitBreaker_ConcurrentCalls(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    10,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   5,
    }
    
    mockBtcRPC := setupMockSuccessfulBtcRPC(t, "test_tx_hash", 1000)
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    const numGoroutines = 50
    const callsPerGoroutine = 10
    
    var wg sync.WaitGroup
    successCount := int64(0)
    errorCount := int64(0)
    
    // Act - Concurrent calls
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(goroutineID int) {
            defer wg.Done()
            
            for j := 0; j < callsPerGoroutine; j++ {
                address := fmt.Sprintf("test_address_%d_%d", goroutineID, j)
                _, _, err := cb.Send(address, createWeb3BigInt("1000"))
                
                if err != nil {
                    atomic.AddInt64(&errorCount, 1)
                } else {
                    atomic.AddInt64(&successCount, 1)
                }
            }
        }(i)
    }
    
    wg.Wait()
    
    // Assert
    totalCalls := numGoroutines * callsPerGoroutine
    assert.Equal(t, int64(totalCalls), successCount+errorCount)
    assert.True(t, successCount > 0, "No successful calls")
    
    // Circuit breaker should still be closed (no failures)
    assert.Equal(t, gobreaker.StateClosed, cb.circuitBreaker.State())
    
    // Verify metrics consistency
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_external_api_calls_total" {
            totalMetricCalls := float64(0)
            for _, metric := range mf.GetMetric() {
                totalMetricCalls += metric.GetCounter().GetValue()
            }
            assert.Equal(t, float64(totalCalls), totalMetricCalls,
                "Metrics count doesn't match actual calls")
        }
    }
}
```

#### TestCircuitBreaker_ConcurrentStateTransitions
```go
func TestCircuitBreaker_ConcurrentStateTransitions(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    3,
        Interval:                      30 * time.Second,
        Timeout:                       100 * time.Millisecond,
        ConsecutiveFailureThreshold:   2,
    }
    
    // Mock that fails initially, then succeeds
    callCount := int64(0)
    mockBtcRPC := setupMockBtcRPCWithDynamicBehavior(t, func() error {
        count := atomic.AddInt64(&callCount, 1)
        if count <= 4 { // First 4 calls fail
            return errors.New("API unavailable")
        }
        return nil // Subsequent calls succeed
    })
    
    metrics := NewExternalAPIMetrics()
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    const numGoroutines = 20
    var wg sync.WaitGroup
    
    // Act - Force state transitions under concurrent load
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            // Multiple calls per goroutine
            for j := 0; j < 5; j++ {
                cb.Send("test_address", createWeb3BigInt("1000"))
                time.Sleep(10 * time.Millisecond) // Small delay
            }
        }()
    }
    
    wg.Wait()
    
    // Wait for potential half-open transition
    time.Sleep(200 * time.Millisecond)
    
    // Make a few more calls to potentially close the circuit
    for i := 0; i < 5; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
    
    // Assert - Final state should be reasonable (closed or half-open)
    finalState := cb.circuitBreaker.State()
    assert.True(t, 
        finalState == gobreaker.StateClosed || finalState == gobreaker.StateHalfOpen,
        "Unexpected final state: %v", finalState)
}
```

### 6. Configuration Tests

#### TestCircuitBreakerConfig_Validation
```go
func TestCircuitBreakerConfig_Validation(t *testing.T) {
    tests := []struct {
        name      string
        config    CircuitBreakerConfig
        shouldErr bool
    }{
        {
            name: "Valid configuration",
            config: CircuitBreakerConfig{
                MaxRequests:                    5,
                Interval:                      30 * time.Second,
                Timeout:                       60 * time.Second,
                ConsecutiveFailureThreshold:   3,
            },
            shouldErr: false,
        },
        {
            name: "Zero max requests",
            config: CircuitBreakerConfig{
                MaxRequests:                    0,
                Interval:                      30 * time.Second,
                Timeout:                       60 * time.Second,
                ConsecutiveFailureThreshold:   3,
            },
            shouldErr: true,
        },
        {
            name: "Zero failure threshold",
            config: CircuitBreakerConfig{
                MaxRequests:                    5,
                Interval:                      30 * time.Second,
                Timeout:                       60 * time.Second,
                ConsecutiveFailureThreshold:   0,
            },
            shouldErr: true,
        },
        {
            name: "Negative timeout",
            config: CircuitBreakerConfig{
                MaxRequests:                    5,
                Interval:                      30 * time.Second,
                Timeout:                       -1 * time.Second,
                ConsecutiveFailureThreshold:   3,
            },
            shouldErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateCircuitBreakerConfig(tt.config)
            
            if tt.shouldErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### TestCircuitBreakerConfig_DefaultValues
```go
func TestCircuitBreakerConfig_DefaultValues(t *testing.T) {
    // Test that default configurations are applied correctly
    configs := map[string]CircuitBreakerConfig{
        "blockstream_api": CircuitBreakerConfigs["blockstream_api"],
        "base_rpc":       CircuitBreakerConfigs["base_rpc"],
    }
    
    for serviceName, config := range configs {
        t.Run(serviceName, func(t *testing.T) {
            assert.True(t, config.MaxRequests > 0, "MaxRequests should be positive")
            assert.True(t, config.Interval > 0, "Interval should be positive")
            assert.True(t, config.Timeout > 0, "Timeout should be positive")
            assert.True(t, config.ConsecutiveFailureThreshold > 0, "ConsecutiveFailureThreshold should be positive")
            
            // Service-specific assertions
            switch serviceName {
            case "blockstream_api":
                assert.Equal(t, uint32(5), config.MaxRequests)
                assert.Equal(t, 30*time.Second, config.Interval)
                assert.Equal(t, 60*time.Second, config.Timeout)
                assert.Equal(t, 3, config.ConsecutiveFailureThreshold)
                
            case "base_rpc":
                assert.Equal(t, uint32(3), config.MaxRequests)
                assert.Equal(t, 45*time.Second, config.Interval)
                assert.Equal(t, 120*time.Second, config.Timeout)
                assert.Equal(t, 5, config.ConsecutiveFailureThreshold)
            }
        })
    }
}
```

### 7. Metrics Integration Tests

#### TestCircuitBreaker_MetricsRecording
```go
func TestCircuitBreaker_MetricsRecording(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   2,
    }
    
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    // Setup mock that alternates between success and failure
    callCount := 0
    mockBtcRPC := setupMockBtcRPCWithPattern(t, []error{
        nil, errors.New("error"), nil, errors.New("error"), errors.New("error"),
    })
    
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Act - Make calls to trigger various metrics
    for i := 0; i < 5; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
    
    // Assert - Check all expected metrics are recorded
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    expectedMetrics := map[string]bool{
        "icy_backend_external_api_duration_seconds": false,
        "icy_backend_external_api_calls_total":     false,
        "icy_backend_circuit_breaker_state":        false,
    }
    
    for _, mf := range metricFamilies {
        if _, exists := expectedMetrics[mf.GetName()]; exists {
            expectedMetrics[mf.GetName()] = true
            
            switch mf.GetName() {
            case "icy_backend_external_api_calls_total":
                // Should have both success and error metrics
                successFound := false
                errorFound := false
                
                for _, metric := range mf.GetMetric() {
                    labels := metric.GetLabel()
                    status := getLabelValue(labels, "status")
                    
                    if status == "success" {
                        successFound = true
                        assert.True(t, metric.GetCounter().GetValue() > 0)
                    } else if status == "error" {
                        errorFound = true
                        assert.True(t, metric.GetCounter().GetValue() > 0)
                    }
                }
                
                assert.True(t, successFound, "Success metrics not found")
                assert.True(t, errorFound, "Error metrics not found")
                
            case "icy_backend_external_api_duration_seconds":
                // Should have recorded durations for all calls
                for _, metric := range mf.GetMetric() {
                    assert.Equal(t, uint64(5), metric.GetHistogram().GetSampleCount())
                    assert.True(t, metric.GetHistogram().GetSampleSum() > 0)
                }
                
            case "icy_backend_circuit_breaker_state":
                // Should record state (likely open due to consecutive failures)
                metric := mf.GetMetric()[0]
                state := metric.GetGauge().GetValue()
                assert.True(t, state >= 0 && state <= 2) // Valid state range
            }
        }
    }
    
    // Verify all expected metrics were found
    for metricName, found := range expectedMetrics {
        assert.True(t, found, "Expected metric not found: %s", metricName)
    }
}
```

### 8. Health Check Integration Tests

#### TestCircuitBreaker_HealthCheckIntegration
```go
func TestCircuitBreaker_HealthCheckIntegration(t *testing.T) {
    // Arrange
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   2,
    }
    
    metrics := NewExternalAPIMetrics()
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("service unavailable"))
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Force circuit to open
    for i := 0; i < 2; i++ {
        cb.EstimateFees()
    }
    assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())
    
    // Act - Health check should detect open circuit
    healthCheck := checkBitcoinAPIWithCircuitBreaker(cb)
    
    // Assert
    assert.Equal(t, "unhealthy", healthCheck.Status)
    assert.Contains(t, healthCheck.Error, "circuit breaker open")
    assert.Contains(t, healthCheck.Metadata, "circuit_state")
    assert.Equal(t, "open", healthCheck.Metadata["circuit_state"])
}
```

## Performance Benchmarks

```go
func BenchmarkCircuitBreaker_SuccessfulCalls(b *testing.B) {
    config := CircuitBreakerConfig{
        MaxRequests:                    100,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   10,
    }
    
    mockBtcRPC := setupMockSuccessfulBtcRPC(nil, "test_hash", 1000)
    metrics := NewExternalAPIMetrics()
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
}

func BenchmarkCircuitBreaker_StateTransitions(b *testing.B) {
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      1 * time.Millisecond,
        Timeout:                       1 * time.Millisecond,
        ConsecutiveFailureThreshold:   2,
    }
    
    callCount := int64(0)
    mockBtcRPC := setupMockBtcRPCWithDynamicBehavior(nil, func() error {
        count := atomic.AddInt64(&callCount, 1)
        if count%3 == 0 {
            return errors.New("intermittent failure")
        }
        return nil
    })
    
    metrics := NewExternalAPIMetrics()
    cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cb.Send("test_address", createWeb3BigInt("1000"))
    }
}
```

## Test Helper Functions

```go
func setupMockBtcRPC(t *testing.T) btcrpc.IBtcRpc {
    // Implementation for basic mock
}

func setupMockSuccessfulBtcRPC(t *testing.T, txHash string, fee int64) btcrpc.IBtcRpc {
    // Implementation for successful mock
}

func setupMockFailingBtcRPC(t *testing.T, err error) btcrpc.IBtcRpc {
    // Implementation for failing mock
}

func setupMockSlowBtcRPC(t *testing.T, delay time.Duration) btcrpc.IBtcRpc {
    // Implementation for slow mock
}

func setupMockBtcRPCWithPattern(t *testing.T, errors []error) btcrpc.IBtcRpc {
    // Implementation for pattern-based mock
}

func setupMockBtcRPCWithDynamicBehavior(t *testing.T, behavior func() error) btcrpc.IBtcRpc {
    // Implementation for dynamic behavior mock
}

func createWeb3BigInt(value string) *model.Web3BigInt {
    // Implementation to create Web3BigInt from string
}

func checkBitcoinAPIWithCircuitBreaker(cb *CircuitBreakerBtcRPC) HealthCheck {
    // Implementation for health check integration
}

func validateCircuitBreakerConfig(config CircuitBreakerConfig) error {
    // Implementation for config validation
}

func setupTestLoggerWithCapture(t *testing.T) *CaptureLogger {
    // Implementation for logger that captures log entries
}
```

## Coverage Requirements

- **Function Coverage**: 100% of all circuit breaker methods
- **Branch Coverage**: >95% including all state transitions
- **Concurrency Coverage**: Thread safety validation
- **Performance Coverage**: Benchmark tests for overhead measurement
- **Integration Coverage**: Health check and metrics integration