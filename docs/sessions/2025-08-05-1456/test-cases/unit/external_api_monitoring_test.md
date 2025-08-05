# External API Monitoring Unit Test Cases

**File**: `internal/monitoring/external_api_monitoring_test.go`  
**Package**: `monitoring`  
**Target Coverage**: >90%  

## Test Suite Overview

Comprehensive unit tests for external API monitoring including circuit breaker wrappers, timeout handling, error classification, metrics collection, and health check integration. Focus on cryptocurrency system resilience and external dependency monitoring.

## Test Cases

### 1. External API Metrics Tests

#### TestExternalAPIMetrics_Registration
```go
func TestExternalAPIMetrics_Registration(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    
    // Act
    metrics.MustRegister(registry)
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    expectedMetrics := []string{
        "icy_backend_external_api_duration_seconds",
        "icy_backend_external_api_calls_total",
        "icy_backend_circuit_breaker_state",
        "icy_backend_external_api_timeouts_total",
    }
    
    registeredMetrics := make(map[string]bool)
    for _, mf := range metricFamilies {
        registeredMetrics[mf.GetName()] = true
    }
    
    for _, expected := range expectedMetrics {
        assert.True(t, registeredMetrics[expected],
            "Expected metric '%s' not registered", expected)
    }
}
```

#### TestExternalAPIMetrics_APICallRecording
```go
func TestExternalAPIMetrics_APICallRecording(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    // Act - Record various API calls
    testCases := []struct {
        apiName   string
        operation string
        status    string
        errorType string
        duration  time.Duration
    }{
        {"btc_rpc", "send", "success", "", 100 * time.Millisecond},
        {"btc_rpc", "fees", "error", "timeout", 500 * time.Millisecond},
        {"base_rpc", "balance", "success", "", 200 * time.Millisecond},
        {"base_rpc", "send", "error", "network", 1000 * time.Millisecond},
    }
    
    for _, tc := range testCases {
        metrics.apiDuration.WithLabelValues(tc.apiName, tc.operation).Observe(tc.duration.Seconds())
        metrics.apiCalls.WithLabelValues(tc.apiName, tc.status, tc.errorType).Inc()
    }
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    durationFound := false
    callsFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_external_api_duration_seconds":
            durationFound = true
            assert.Equal(t, 4, len(mf.GetMetric())) // 4 different operation recordings
            
            for _, metric := range mf.GetMetric() {
                assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
                assert.True(t, metric.GetHistogram().GetSampleSum() > 0)
            }
            
        case "icy_backend_external_api_calls_total":
            callsFound = true
            assert.Equal(t, 4, len(mf.GetMetric()))
            
            for _, metric := range mf.GetMetric() {
                assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                
                labels := metric.GetLabel()
                apiName := getLabelValue(labels, "api_name")
                assert.True(t, apiName == "btc_rpc" || apiName == "base_rpc")
            }
        }
    }
    
    assert.True(t, durationFound, "Duration metrics not found")
    assert.True(t, callsFound, "Call count metrics not found")
}
```

#### TestExternalAPIMetrics_CircuitBreakerStateTracking
```go
func TestExternalAPIMetrics_CircuitBreakerStateTracking(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    // Act - Record circuit breaker state changes
    stateChanges := []struct {
        apiName string
        state   float64
    }{
        {"btc_rpc", 0},  // Closed
        {"base_rpc", 0}, // Closed
        {"btc_rpc", 1},  // Open
        {"base_rpc", 2}, // Half-open
        {"btc_rpc", 0},  // Back to closed
    }
    
    for _, sc := range stateChanges {
        metrics.circuitBreakerState.WithLabelValues(sc.apiName).Set(sc.state)
    }
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_circuit_breaker_state" {
            assert.Equal(t, 2, len(mf.GetMetric())) // btc_rpc and base_rpc
            
            stateValues := make(map[string]float64)
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                apiName := getLabelValue(labels, "api_name")
                stateValues[apiName] = metric.GetGauge().GetValue()
            }
            
            assert.Equal(t, float64(0), stateValues["btc_rpc"])  // Final state: closed
            assert.Equal(t, float64(2), stateValues["base_rpc"]) // Final state: half-open
        }
    }
}
```

#### TestExternalAPIMetrics_TimeoutTracking
```go
func TestExternalAPIMetrics_TimeoutTracking(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    // Act - Record different types of timeouts
    timeoutCases := []struct {
        apiName     string
        timeoutType string
        count       int
    }{
        {"btc_rpc", "connection", 2},
        {"btc_rpc", "request", 1},
        {"btc_rpc", "health_check", 1},
        {"base_rpc", "connection", 1},
        {"base_rpc", "request", 3},
    }
    
    for _, tc := range timeoutCases {
        for i := 0; i < tc.count; i++ {
            metrics.timeouts.WithLabelValues(tc.apiName, tc.timeoutType).Inc()
        }
    }
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_external_api_timeouts_total" {
            assert.Equal(t, 5, len(mf.GetMetric())) // 5 different timeout type recordings
            
            timeoutCounts := make(map[string]map[string]float64)
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                apiName := getLabelValue(labels, "api_name")
                timeoutType := getLabelValue(labels, "timeout_type")
                
                if timeoutCounts[apiName] == nil {
                    timeoutCounts[apiName] = make(map[string]float64)
                }
                timeoutCounts[apiName][timeoutType] = metric.GetCounter().GetValue()
            }
            
            assert.Equal(t, float64(2), timeoutCounts["btc_rpc"]["connection"])
            assert.Equal(t, float64(1), timeoutCounts["btc_rpc"]["request"])
            assert.Equal(t, float64(1), timeoutCounts["btc_rpc"]["health_check"])
            assert.Equal(t, float64(1), timeoutCounts["base_rpc"]["connection"])
            assert.Equal(t, float64(3), timeoutCounts["base_rpc"]["request"])
        }
    }
}
```

### 2. Timeout Configuration Tests

#### TestTimeoutConfig_Defaults
```go
func TestTimeoutConfig_Defaults(t *testing.T) {
    // Test default timeout configurations for different APIs
    configs := map[string]TimeoutConfig{
        "blockstream_api": TimeoutConfigs["blockstream_api"],
        "base_rpc":       TimeoutConfigs["base_rpc"],
    }
    
    for apiName, config := range configs {
        t.Run(apiName, func(t *testing.T) {
            assert.True(t, config.ConnectionTimeout > 0, "Connection timeout should be positive")
            assert.True(t, config.RequestTimeout > 0, "Request timeout should be positive")
            assert.True(t, config.HealthCheckTimeout > 0, "Health check timeout should be positive")
            
            // Connection timeout should be shorter than request timeout
            assert.True(t, config.ConnectionTimeout <= config.RequestTimeout,
                "Connection timeout should be <= request timeout")
            
            // Health check timeout should be reasonable
            assert.True(t, config.HealthCheckTimeout <= config.RequestTimeout,
                "Health check timeout should be <= request timeout")
            
            // Verify specific values
            switch apiName {
            case "blockstream_api":
                assert.Equal(t, 2*time.Second, config.ConnectionTimeout)
                assert.Equal(t, 5*time.Second, config.RequestTimeout)
                assert.Equal(t, 3*time.Second, config.HealthCheckTimeout)
                
            case "base_rpc":
                assert.Equal(t, 3*time.Second, config.ConnectionTimeout)
                assert.Equal(t, 10*time.Second, config.RequestTimeout)
                assert.Equal(t, 5*time.Second, config.HealthCheckTimeout)
            }
        })
    }
}
```

#### TestTimeoutConfig_Validation
```go
func TestTimeoutConfig_Validation(t *testing.T) {
    tests := []struct {
        name      string
        config    TimeoutConfig
        shouldErr bool
    }{
        {
            name: "Valid configuration",
            config: TimeoutConfig{
                ConnectionTimeout:  2 * time.Second,
                RequestTimeout:     5 * time.Second,
                HealthCheckTimeout: 3 * time.Second,
            },
            shouldErr: false,
        },
        {
            name: "Zero connection timeout",
            config: TimeoutConfig{
                ConnectionTimeout:  0,
                RequestTimeout:     5 * time.Second,
                HealthCheckTimeout: 3 * time.Second,
            },
            shouldErr: true,
        },
        {
            name: "Negative request timeout",
            config: TimeoutConfig{
                ConnectionTimeout:  2 * time.Second,
                RequestTimeout:     -1 * time.Second,
                HealthCheckTimeout: 3 * time.Second,
            },
            shouldErr: true,
        },
        {
            name: "Health check timeout longer than request timeout",
            config: TimeoutConfig{
                ConnectionTimeout:  2 * time.Second,
                RequestTimeout:     3 * time.Second,
                HealthCheckTimeout: 5 * time.Second,
            },
            shouldErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateTimeoutConfig(tt.config)
            
            if tt.shouldErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 3. Wrapper Integration Tests

#### TestCircuitBreakerWrapper_Initialization
```go
func TestCircuitBreakerWrapper_Initialization(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockBtcRPC(t)
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    logger := setupTestLogger()
    
    // Act
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, logger)
    
    // Assert
    assert.NotNil(t, wrapper)
    assert.NotNil(t, wrapper.circuitBreaker)
    assert.Equal(t, gobreaker.StateClosed, wrapper.circuitBreaker.State())
    
    // Verify initial circuit breaker state is recorded in metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_circuit_breaker_state" {
            assert.Equal(t, 1, len(mf.GetMetric()))
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(gobreaker.StateClosed), metric.GetGauge().GetValue())
        }
    }
}
```

#### TestCircuitBreakerWrapper_MethodDelegation
```go
func TestCircuitBreakerWrapper_MethodDelegation(t *testing.T) {
    // Arrange
    expectedTxHash := "test_tx_hash_123"
    expectedFee := int64(2500)
    expectedFees := &model.BtcFees{
        Fast:   15000,
        Medium: 10000,
        Slow:   5000,
    }
    
    mockBtcRPC := setupMockBtcRPCWithExpectations(t, map[string]interface{}{
        "Send":         []interface{}{expectedTxHash, expectedFee, nil},
        "EstimateFees": []interface{}{expectedFees, nil},
        "GetBalance":   []interface{}{createWeb3BigInt("500000000"), nil}, // 5 BTC
    })
    
    config := CircuitBreakerConfig{
        MaxRequests:                    10,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   5,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Act & Assert - Test Send method
    txHash, fee, err := wrapper.Send("test_address", createWeb3BigInt("100000"))
    assert.NoError(t, err)
    assert.Equal(t, expectedTxHash, txHash)
    assert.Equal(t, expectedFee, fee)
    
    // Act & Assert - Test EstimateFees method
    fees, err := wrapper.EstimateFees()
    assert.NoError(t, err)
    assert.Equal(t, expectedFees, fees)
    
    // Act & Assert - Test GetBalance method
    balance, err := wrapper.GetBalance("test_address")
    assert.NoError(t, err)
    assert.Equal(t, "500000000", balance.String())
    
    // Verify circuit breaker remains closed
    assert.Equal(t, gobreaker.StateClosed, wrapper.circuitBreaker.State())
}
```

### 4. Health Check Integration Tests

#### TestExternalAPIHealthCheck_WithCircuitBreaker
```go
func TestExternalAPIHealthCheck_WithCircuitBreaker(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockHealthyBtcRPC(t)
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Act
    healthCheck := performBitcoinAPIHealthCheck(wrapper)
    
    // Assert
    assert.Equal(t, "healthy", healthCheck.Status)
    assert.Empty(t, healthCheck.Error)
    assert.True(t, healthCheck.Latency > 0)
    assert.Contains(t, healthCheck.Metadata, "endpoint")
    assert.Contains(t, healthCheck.Metadata, "circuit_breaker_state")
    assert.Equal(t, "closed", healthCheck.Metadata["circuit_breaker_state"])
}
```

#### TestExternalAPIHealthCheck_CircuitBreakerOpen
```go
func TestExternalAPIHealthCheck_CircuitBreakerOpen(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("service unavailable"))
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   2, // Low threshold for testing
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    // Force circuit breaker to open
    for i := 0; i < 2; i++ {
        wrapper.EstimateFees()
    }
    assert.Equal(t, gobreaker.StateOpen, wrapper.circuitBreaker.State())
    
    // Act
    healthCheck := performBitcoinAPIHealthCheck(wrapper)
    
    // Assert
    assert.Equal(t, "unhealthy", healthCheck.Status)
    assert.Contains(t, healthCheck.Error, "circuit breaker open")
    assert.Contains(t, healthCheck.Metadata, "circuit_breaker_state")
    assert.Equal(t, "open", healthCheck.Metadata["circuit_breaker_state"])
}
```

### 5. Error Handling and Logging Tests

#### TestExternalAPILogging_SuccessfulCall
```go
func TestExternalAPILogging_SuccessfulCall(t *testing.T) {
    // Arrange
    testLogger := setupTestLoggerWithCapture(t)
    mockBtcRPC := setupMockSuccessfulBtcRPC(t, "success_tx", 1000)
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, testLogger)
    
    // Act
    txHash, fee, err := wrapper.Send("test_address", createWeb3BigInt("100000"))
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "success_tx", txHash)
    assert.Equal(t, int64(1000), fee)
    
    // Verify logging
    logEntries := testLogger.GetCapturedLogs()
    assert.True(t, len(logEntries) > 0)
    
    successLogFound := false
    for _, entry := range logEntries {
        if entry.Level == "INFO" && strings.Contains(entry.Message, "External API call successful") {
            successLogFound = true
            assert.Contains(t, entry.Fields, "service")
            assert.Contains(t, entry.Fields, "operation")
            assert.Contains(t, entry.Fields, "duration")
            assert.Contains(t, entry.Fields, "cb_state")
            
            assert.Equal(t, "btc_rpc", entry.Fields["service"])
            assert.Equal(t, "send", entry.Fields["operation"])
            assert.Equal(t, "closed", entry.Fields["cb_state"])
            break
        }
    }
    
    assert.True(t, successLogFound, "Success log entry not found")
}
```

#### TestExternalAPILogging_FailedCall
```go
func TestExternalAPILogging_FailedCall(t *testing.T) {
    // Arrange
    testLogger := setupTestLoggerWithCapture(t)
    expectedError := errors.New("insufficient funds for transaction")
    mockBtcRPC := setupMockBtcRPCWithSpecificError(t, expectedError)
    
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, testLogger)
    
    // Act
    _, _, err := wrapper.Send("test_address", createWeb3BigInt("100000000000")) // Very large amount
    
    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "insufficient funds")
    
    // Verify error logging
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
            assert.Equal(t, "send", entry.Fields["operation"])
            assert.Contains(t, entry.Fields["error"], "insufficient funds")
            assert.Equal(t, "unknown", entry.Fields["error_type"]) // Custom classification
            break
        }
    }
    
    assert.True(t, errorLogFound, "Error log entry not found")
}
```

### 6. Timeout Execution Tests

#### TestTimeoutExecution_RequestTimeout
```go
func TestTimeoutExecution_RequestTimeout(t *testing.T) {
    // Arrange
    timeoutConfig := TimeoutConfig{
        ConnectionTimeout:  2 * time.Second,
        RequestTimeout:     200 * time.Millisecond, // Short timeout for testing
        HealthCheckTimeout: 3 * time.Second,
    }
    
    // Mock that takes longer than timeout
    mockBtcRPC := setupMockSlowBtcRPC(t, 500*time.Millisecond)
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    wrapper := NewCircuitBreakerBtcRPCWithTimeout(mockBtcRPC, config, timeoutConfig, metrics, setupTestLogger())
    
    // Act
    start := time.Now()
    result, err := wrapper.executeWithTimeout("request", func() (interface{}, error) {
        return wrapper.EstimateFees()
    })
    duration := time.Since(start)
    
    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
    assert.Nil(t, result)
    assert.True(t, duration >= 200*time.Millisecond, "Should wait for timeout")
    assert.True(t, duration < 400*time.Millisecond, "Should not wait much longer than timeout")
    
    // Verify timeout metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_external_api_timeouts_total" {
            found := false
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "api_name") == "btc_rpc" &&
                   getLabelValue(labels, "timeout_type") == "request" {
                    found = true
                    assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                }
            }
            assert.True(t, found, "Request timeout metric not found")
        }
    }
}
```

#### TestTimeoutExecution_HealthCheckTimeout
```go
func TestTimeoutExecution_HealthCheckTimeout(t *testing.T) {
    // Arrange
    timeoutConfig := TimeoutConfig{
        ConnectionTimeout:  2 * time.Second,
        RequestTimeout:     5 * time.Second,
        HealthCheckTimeout: 150 * time.Millisecond, // Short health check timeout
    }
    
    mockBtcRPC := setupMockSlowBtcRPC(t, 300*time.Millisecond) // Slower than health check timeout
    config := CircuitBreakerConfig{
        MaxRequests:                    5,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   3,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPCWithTimeout(mockBtcRPC, config, timeoutConfig, metrics, setupTestLogger())
    
    // Act
    start := time.Now()
    result, err := wrapper.executeWithTimeout("health_check", func() (interface{}, error) {
        return wrapper.EstimateFees()
    })
    duration := time.Since(start)
    
    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
    assert.Nil(t, result)
    assert.True(t, duration >= 150*time.Millisecond, "Should wait for health check timeout")
    assert.True(t, duration < 250*time.Millisecond, "Should not wait much longer than timeout")
}
```

### 7. Configuration Integration Tests

#### TestExternalAPIConfig_LoadFromEnvironment
```go
func TestExternalAPIConfig_LoadFromEnvironment(t *testing.T) {
    // Arrange - Set environment variables
    os.Setenv("EXTERNAL_API_BLOCKSTREAM_CB_MAX_REQUESTS", "10")
    os.Setenv("EXTERNAL_API_BLOCKSTREAM_CB_TIMEOUT", "120s")
    os.Setenv("EXTERNAL_API_BLOCKSTREAM_TIMEOUT_REQUEST", "8s")
    defer func() {
        os.Unsetenv("EXTERNAL_API_BLOCKSTREAM_CB_MAX_REQUESTS")
        os.Unsetenv("EXTERNAL_API_BLOCKSTREAM_CB_TIMEOUT")
        os.Unsetenv("EXTERNAL_API_BLOCKSTREAM_TIMEOUT_REQUEST")
    }()
    
    // Act
    config, err := LoadExternalAPIConfig("blockstream_api")
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, uint32(10), config.CircuitBreaker.MaxRequests)
    assert.Equal(t, 120*time.Second, config.CircuitBreaker.Timeout)
    assert.Equal(t, 8*time.Second, config.Timeouts.RequestTimeout)
}
```

#### TestExternalAPIConfig_DefaultFallback
```go
func TestExternalAPIConfig_DefaultFallback(t *testing.T) {
    // Arrange - Ensure no environment variables are set
    envVars := []string{
        "EXTERNAL_API_BASE_RPC_CB_MAX_REQUESTS",
        "EXTERNAL_API_BASE_RPC_CB_TIMEOUT",
        "EXTERNAL_API_BASE_RPC_TIMEOUT_REQUEST",
    }
    
    for _, envVar := range envVars {
        os.Unsetenv(envVar)
    }
    
    // Act
    config, err := LoadExternalAPIConfig("base_rpc")
    
    // Assert
    assert.NoError(t, err)
    
    // Should use default values
    defaultConfig := CircuitBreakerConfigs["base_rpc"]
    assert.Equal(t, defaultConfig.MaxRequests, config.CircuitBreaker.MaxRequests)
    assert.Equal(t, defaultConfig.Timeout, config.CircuitBreaker.Timeout)
    assert.Equal(t, defaultConfig.ConsecutiveFailureThreshold, config.CircuitBreaker.ConsecutiveFailureThreshold)
    
    defaultTimeouts := TimeoutConfigs["base_rpc"]
    assert.Equal(t, defaultTimeouts.RequestTimeout, config.Timeouts.RequestTimeout)
    assert.Equal(t, defaultTimeouts.ConnectionTimeout, config.Timeouts.ConnectionTimeout)
    assert.Equal(t, defaultTimeouts.HealthCheckTimeout, config.Timeouts.HealthCheckTimeout)
}
```

### 8. Error Classification Tests

#### TestAPIErrorClassification_Comprehensive
```go
func TestAPIErrorClassification_Comprehensive(t *testing.T) {
    tests := []struct {
        name         string
        error        error
        expectedType APIErrorType
    }{
        {
            name:         "Connection timeout",
            error:        errors.New("dial tcp: i/o timeout"),
            expectedType: ErrorTypeTimeout,
        },
        {
            name:         "Request timeout",
            error:        errors.New("request timeout after 30s"),
            expectedType: ErrorTypeTimeout,
        },
        {
            name:         "Context deadline exceeded",
            error:        errors.New("context deadline exceeded"),
            expectedType: ErrorTypeTimeout,
        },
        {
            name:         "Network unreachable",
            error:        errors.New("network is unreachable"),
            expectedType: ErrorTypeNetworkError,
        },
        {
            name:         "Connection refused",
            error:        errors.New("connection refused"),
            expectedType: ErrorTypeNetworkError,
        },
        {
            name:         "HTTP 500 error",
            error:        errors.New("HTTP 500 Internal Server Error"),
            expectedType: ErrorTypeServerError,
        },
        {
            name:         "HTTP 502 error",
            error:        errors.New("502 Bad Gateway"),
            expectedType: ErrorTypeServerError,
        },
        {
            name:         "HTTP 503 error",
            error:        errors.New("503 Service Unavailable"),
            expectedType: ErrorTypeServerError,
        },
        {
            name:         "HTTP 400 error",
            error:        errors.New("400 Bad Request"),
            expectedType: ErrorTypeClientError,
        },
        {
            name:         "HTTP 401 error",
            error:        errors.New("401 Unauthorized"),
            expectedType: ErrorTypeClientError,
        },
        {
            name:         "HTTP 429 rate limit",
            error:        errors.New("429 Too Many Requests"),
            expectedType: ErrorTypeClientError,
        },
        {
            name:         "Bitcoin RPC error",
            error:        errors.New("insufficient funds for transaction"),
            expectedType: ErrorTypeUnknown,
        },
        {
            name:         "Generic API error",
            error:        errors.New("unexpected API response"),
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
            result := classifyAPIError(tt.error)
            assert.Equal(t, tt.expectedType, result)
        })
    }
}
```

### 9. Concurrent Access Tests

#### TestExternalAPIMonitoring_ConcurrentCalls
```go
func TestExternalAPIMonitoring_ConcurrentCalls(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockBtcRPCWithRandomBehavior(t, 0.1) // 10% failure rate
    config := CircuitBreakerConfig{
        MaxRequests:                    20,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   10,
    }
    
    metrics := NewExternalAPIMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    const numGoroutines = 50
    const callsPerGoroutine = 20
    
    var wg sync.WaitGroup
    successCount := int64(0)
    errorCount := int64(0)
    
    // Act - Concurrent API calls
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(goroutineID int) {
            defer wg.Done()
            
            for j := 0; j < callsPerGoroutine; j++ {
                // Alternate between different API calls
                var err error
                switch j % 3 {
                case 0:
                    _, err = wrapper.EstimateFees()
                case 1:
                    _, err = wrapper.GetBalance("test_address")
                case 2:
                    _, _, err = wrapper.Send("test_address", createWeb3BigInt("1000"))
                }
                
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
    assert.True(t, successCount > 0, "Some calls should succeed")
    
    // Circuit breaker should handle the load appropriately
    finalState := wrapper.circuitBreaker.State()
    assert.True(t, 
        finalState == gobreaker.StateClosed || 
        finalState == gobreaker.StateHalfOpen || 
        finalState == gobreaker.StateOpen,
        "Circuit breaker should be in a valid state: %v", finalState)
    
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
                "Metrics count should match actual calls")
        }
    }
}
```

### 10. Integration with Base RPC Tests

#### TestCircuitBreakerBaseRPC_Integration
```go
func TestCircuitBreakerBaseRPC_Integration(t *testing.T) {
    // Arrange
    expectedSupply := createWeb3BigInt("21000000000000000000000000") // 21M ICY
    expectedBalance := createWeb3BigInt("5000000000000000000000")    // 5000 ICY
    
    mockBaseRPC := setupMockSuccessfulBaseRPC(t, map[string]interface{}{
        "ICYTotalSupply": expectedSupply,
        "GetBalance":     expectedBalance,
        "Transfer":       "0xabcdef123456789",
    })
    
    config := CircuitBreakerConfig{
        MaxRequests:                    3,
        Interval:                      45 * time.Second,
        Timeout:                       120 * time.Second,
        ConsecutiveFailureThreshold:   5,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBaseRPC(mockBaseRPC, config, metrics, setupTestLogger())
    
    // Act & Assert - Test various Base RPC methods
    supply, err := wrapper.ICYTotalSupply()
    assert.NoError(t, err)
    assert.Equal(t, expectedSupply.String(), supply.String())
    
    balance, err := wrapper.GetBalance("0x123...")
    assert.NoError(t, err)
    assert.Equal(t, expectedBalance.String(), balance.String())
    
    txHash, err := wrapper.Transfer("0x456...", createWeb3BigInt("1000000000000000000"))
    assert.NoError(t, err)
    assert.Equal(t, "0xabcdef123456789", txHash)
    
    // Verify circuit breaker remains closed
    assert.Equal(t, gobreaker.StateClosed, wrapper.circuitBreaker.State())
}
```

## Performance Benchmarks

```go
func BenchmarkCircuitBreakerWrapper_SuccessfulCalls(b *testing.B) {
    mockBtcRPC := setupMockSuccessfulBtcRPC(nil, "bench_tx", 1000)
    config := CircuitBreakerConfig{
        MaxRequests:                    100,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   10,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        wrapper.EstimateFees()
    }
}

func BenchmarkExternalAPIMetrics_Recording(b *testing.B) {
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        metrics.apiDuration.WithLabelValues("btc_rpc", "send").Observe(0.1)
        metrics.apiCalls.WithLabelValues("btc_rpc", "success", "").Inc()
    }
}

func BenchmarkTimeoutExecution(b *testing.B) {
    mockBtcRPC := setupMockFastBtcRPC(nil)
    config := CircuitBreakerConfig{
        MaxRequests:                    100,
        Interval:                      30 * time.Second,
        Timeout:                       60 * time.Second,
        ConsecutiveFailureThreshold:   10,
    }
    timeoutConfig := TimeoutConfig{
        ConnectionTimeout:  2 * time.Second,
        RequestTimeout:     5 * time.Second,
        HealthCheckTimeout: 3 * time.Second,
    }
    
    metrics := NewExternalAPIMetrics()
    wrapper := NewCircuitBreakerBtcRPCWithTimeout(mockBtcRPC, config, timeoutConfig, metrics, setupTestLogger())
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        wrapper.executeWithTimeout("request", func() (interface{}, error) {
            return wrapper.EstimateFees()
        })
    }
}
```

## Test Helper Functions

```go
func setupMockBtcRPC(t *testing.T) btcrpc.IBtcRpc {
    // Implementation for basic mock
}

func setupMockSuccessfulBtcRPC(t *testing.T, txHash string, fee int64) btcrpc.IBtcRpc {
    // Implementation for successful mock with specific return values
}

func setupMockFailingBtcRPC(t *testing.T, err error) btcrpc.IBtcRpc {
    // Implementation for failing mock
}

func setupMockSlowBtcRPC(t *testing.T, delay time.Duration) btcrpc.IBtcRpc {
    // Implementation for slow mock
}

func setupMockBtcRPCWithRandomBehavior(t *testing.T, failureRate float32) btcrpc.IBtcRpc {
    // Implementation for random behavior mock
}

func setupMockSuccessfulBaseRPC(t *testing.T, returnValues map[string]interface{}) baserpc.IBaseRPC {
    // Implementation for successful Base RPC mock
}

func performBitcoinAPIHealthCheck(wrapper *CircuitBreakerBtcRPC) HealthCheck {
    // Implementation for health check integration
}

func createWeb3BigInt(value string) *model.Web3BigInt {
    // Implementation to create Web3BigInt from string
}

func validateTimeoutConfig(config TimeoutConfig) error {
    // Implementation for timeout config validation
}

func LoadExternalAPIConfig(apiName string) (*ExternalAPIConfig, error) {
    // Implementation for loading config from environment
}

func classifyAPIError(err error) APIErrorType {
    // Implementation for API error classification
}

func setupTestLoggerWithCapture(t *testing.T) *CaptureLogger {
    // Implementation for logger that captures log entries
}

func getLabelValue(labels []*dto.LabelPair, name string) string {
    for _, label := range labels {
        if label.GetName() == name {
            return label.GetValue()
        }
    }
    return ""
}
```

## Coverage Requirements

- **Function Coverage**: 100% of external API monitoring functions
- **Branch Coverage**: >90% including all error paths and timeout scenarios
- **Integration Coverage**: Circuit breaker and health check integration
- **Performance Coverage**: Benchmark tests for wrapper overhead
- **Concurrency Coverage**: Thread safety under concurrent load