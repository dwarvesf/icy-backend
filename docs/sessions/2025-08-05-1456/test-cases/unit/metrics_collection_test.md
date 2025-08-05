# Metrics Collection Unit Test Cases

**File**: `internal/monitoring/metrics_collection_test.go`  
**Package**: `monitoring`  
**Target Coverage**: >90%  

## Test Suite Overview

Comprehensive unit tests for Prometheus metrics collection including HTTP metrics middleware, business logic metrics, external API metrics, and data sanitization. Focus on accuracy, performance, cardinality control, and security.

## Test Cases

### 1. HTTP Metrics Middleware Tests

#### TestHTTPMetrics_Middleware_SuccessfulRequest
```go
func TestHTTPMetrics_Middleware_SuccessfulRequest(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.Use(metrics.Middleware())
    router.GET("/test", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "success"})
    })
    
    // Act
    w := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/test", nil)
    router.ServeHTTP(w, req)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    
    // Verify metrics were recorded
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    // Check request count metric
    requestCountFound := false
    durationFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_http_requests_total":
            requestCountFound = true
            assert.Equal(t, 1, len(mf.GetMetric()))
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetCounter().GetValue())
            
            // Verify labels
            labels := metric.GetLabel()
            assert.Equal(t, "GET", getLabelValue(labels, "method"))
            assert.Equal(t, "/test", getLabelValue(labels, "endpoint"))
            assert.Equal(t, "200", getLabelValue(labels, "status"))
            
        case "icy_backend_http_request_duration_seconds":
            durationFound = true
            assert.Equal(t, 1, len(mf.GetMetric()))
            metric := mf.GetMetric()[0]
            assert.True(t, metric.GetHistogram().GetSampleCount() == 1)
            assert.True(t, metric.GetHistogram().GetSampleSum() > 0)
        }
    }
    
    assert.True(t, requestCountFound, "Request count metric not found")
    assert.True(t, durationFound, "Duration metric not found")
}
```

#### TestHTTPMetrics_Middleware_ErrorRequest
```go
func TestHTTPMetrics_Middleware_ErrorRequest(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.Use(metrics.Middleware())
    router.GET("/error", func(c *gin.Context) {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
    })
    
    // Act
    w := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/error", nil)
    router.ServeHTTP(w, req)
    
    // Assert
    assert.Equal(t, http.StatusInternalServerError, w.Code)
    
    // Verify error status recorded
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_http_requests_total" {
            metric := mf.GetMetric()[0]
            labels := metric.GetLabel()
            assert.Equal(t, "500", getLabelValue(labels, "status"))
        }
    }
}
```

#### TestHTTPMetrics_Middleware_ActiveRequestsGauge
```go
func TestHTTPMetrics_Middleware_ActiveRequestsGauge(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.Use(metrics.Middleware())
    
    requestStarted := make(chan bool)
    requestCanFinish := make(chan bool)
    
    router.GET("/slow", func(c *gin.Context) {
        requestStarted <- true
        <-requestCanFinish
        c.JSON(http.StatusOK, gin.H{"message": "completed"})
    })
    
    // Act - Start request in goroutine
    go func() {
        w := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/slow", nil)
        router.ServeHTTP(w, req)
    }()
    
    // Wait for request to start
    <-requestStarted
    
    // Assert - Active requests should be 1
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_http_active_requests" {
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetGauge().GetValue())
        }
    }
    
    // Finish request
    requestCanFinish <- true
    
    // Wait a bit for request to complete
    time.Sleep(10 * time.Millisecond)
    
    // Assert - Active requests should be 0
    metricFamilies, err = registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_http_active_requests" {
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(0), metric.GetGauge().GetValue())
        }
    }
}
```

#### TestHTTPMetrics_Middleware_RequestResponseSize
```go
func TestHTTPMetrics_Middleware_RequestResponseSize(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.Use(metrics.Middleware())
    router.POST("/data", func(c *gin.Context) {
        response := make([]byte, 1024) // 1KB response
        c.Data(http.StatusOK, "application/octet-stream", response)
    })
    
    // Act - Send request with 512 bytes
    requestData := make([]byte, 512)
    w := httptest.NewRecorder()
    req := httptest.NewRequest("POST", "/data", bytes.NewReader(requestData))
    req.Header.Set("Content-Length", "512")
    router.ServeHTTP(w, req)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    requestSizeFound := false
    responseSizeFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_http_request_size_bytes":
            requestSizeFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
            assert.Equal(t, float64(512), metric.GetHistogram().GetSampleSum())
            
        case "icy_backend_http_response_size_bytes":
            responseSizeFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
            assert.Equal(t, float64(1024), metric.GetHistogram().GetSampleSum())
        }
    }
    
    assert.True(t, requestSizeFound, "Request size metric not found")
    assert.True(t, responseSizeFound, "Response size metric not found")
}
```

#### TestHTTPMetrics_EndpointNormalization
```go
func TestHTTPMetrics_EndpointNormalization(t *testing.T) {
    tests := []struct {
        path     string
        expected string
    }{
        {"/healthz", "/healthz"},
        {"/api/v1/health/db", "/api/v1/health/*"},
        {"/api/v1/health/external", "/api/v1/health/*"},
        {"/api/v1/oracle/ratio", "/api/v1/oracle/*"},
        {"/api/v1/oracle/treasury", "/api/v1/oracle/*"},
        {"/api/v1/swap/create", "/api/v1/swap/*"},
        {"/api/v1/transactions", "/api/v1/transactions"},
        {"/swagger/index.html", "/swagger/*"},
        {"/metrics", "/metrics"},
        {"/unknown/path", "other"},
        {"", "unknown"},
    }
    
    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            result := normalizeEndpoint(tt.path)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### TestHTTPMetrics_Middleware_Performance
```go
func TestHTTPMetrics_Middleware_Performance(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.Use(metrics.Middleware())
    router.GET("/test", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "ok"})
    })
    
    // Act - Measure overhead
    const numRequests = 1000
    start := time.Now()
    
    for i := 0; i < numRequests; i++ {
        w := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/test", nil)
        router.ServeHTTP(w, req)
    }
    
    totalTime := time.Since(start)
    avgOverhead := totalTime / time.Duration(numRequests)
    
    // Assert - Should be less than 1ms overhead per request
    assert.True(t, avgOverhead < 1*time.Millisecond,
        "Metrics middleware overhead too high: %v", avgOverhead)
}
```

### 2. Business Logic Metrics Tests

#### TestBusinessMetrics_OracleInstrumentation
```go
func TestBusinessMetrics_OracleInstrumentation(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewBusinessMetrics()
    metrics.MustRegister(registry)
    
    mockOracle := setupMockOracleHandler(t)
    instrumentedOracle := NewInstrumentedOracleHandler(mockOracle, metrics, setupTestLogger())
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    instrumentedOracle.GetCirculatedICY(c)
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    operationCountFound := false
    durationFound := false
    dataAgeFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_oracle_operations_total":
            operationCountFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetCounter().GetValue())
            
            labels := metric.GetLabel()
            assert.Equal(t, "circulated_icy", getLabelValue(labels, "operation"))
            assert.Equal(t, "success", getLabelValue(labels, "status"))
            
        case "icy_backend_oracle_calculation_duration_seconds":
            durationFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
            assert.True(t, metric.GetHistogram().GetSampleSum() > 0)
            
        case "icy_backend_oracle_data_age_seconds":
            dataAgeFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(0), metric.GetGauge().GetValue()) // Fresh data
        }
    }
    
    assert.True(t, operationCountFound, "Oracle operation count not found")
    assert.True(t, durationFound, "Oracle duration not found")
    assert.True(t, dataAgeFound, "Oracle data age not found")
}
```

#### TestBusinessMetrics_SwapInstrumentation
```go
func TestBusinessMetrics_SwapInstrumentation(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewBusinessMetrics()
    metrics.MustRegister(registry)
    
    mockSwap := setupMockSwapHandler(t)
    instrumentedSwap := NewInstrumentedSwapHandler(mockSwap, metrics, setupTestLogger())
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Mock request with amount data
    requestBody := `{"btc_amount": "0.1", "icy_amount": "1000"}`
    c.Request = httptest.NewRequest("POST", "/swap", strings.NewReader(requestBody))
    c.Request.Header.Set("Content-Type", "application/json")
    
    // Act
    instrumentedSwap.CreateSwapRequest(c)
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    swapCountFound := false
    durationFound := false
    amountDistributionFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_swap_operations_total":
            swapCountFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetCounter().GetValue())
            
        case "icy_backend_swap_processing_duration_seconds":
            durationFound = true
            
        case "icy_backend_swap_amount_distribution":
            amountDistributionFound = true
            // Should have both BTC and ICY amount recordings
            assert.True(t, len(mf.GetMetric()) >= 1)
        }
    }
    
    assert.True(t, swapCountFound, "Swap operation count not found")
    assert.True(t, durationFound, "Swap duration not found")
    assert.True(t, amountDistributionFound, "Amount distribution not found")
}
```

#### TestBusinessMetrics_ErrorHandling
```go
func TestBusinessMetrics_ErrorHandling(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewBusinessMetrics()
    metrics.MustRegister(registry)
    
    mockOracle := setupMockFailingOracleHandler(t, errors.New("external API error"))
    instrumentedOracle := NewInstrumentedOracleHandler(mockOracle, metrics, setupTestLogger())
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    instrumentedOracle.GetCirculatedICY(c)
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_oracle_operations_total" {
            metric := mf.GetMetric()[0]
            labels := metric.GetLabel()
            assert.Equal(t, "error", getLabelValue(labels, "status"))
        }
    }
}
```

### 3. External API Metrics Tests

#### TestExternalAPIMetrics_SuccessfulCall
```go
func TestExternalAPIMetrics_SuccessfulCall(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    // Act - Simulate successful API call
    start := time.Now()
    time.Sleep(50 * time.Millisecond) // Simulate API call duration
    duration := time.Since(start)
    
    metrics.apiDuration.WithLabelValues("btc_rpc", "send").Observe(duration.Seconds())
    metrics.apiCalls.WithLabelValues("btc_rpc", "success", "").Inc()
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    durationFound := false
    callsFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_external_api_duration_seconds":
            durationFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
            assert.True(t, metric.GetHistogram().GetSampleSum() >= 0.05) // ~50ms
            
        case "icy_backend_external_api_calls_total":
            callsFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetCounter().GetValue())
            
            labels := metric.GetLabel()
            assert.Equal(t, "btc_rpc", getLabelValue(labels, "api_name"))
            assert.Equal(t, "success", getLabelValue(labels, "status"))
        }
    }
    
    assert.True(t, durationFound, "API duration metric not found")
    assert.True(t, callsFound, "API calls metric not found")
}
```

#### TestExternalAPIMetrics_CircuitBreakerStates
```go
func TestExternalAPIMetrics_CircuitBreakerStates(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewExternalAPIMetrics()
    metrics.MustRegister(registry)
    
    // Act - Test different circuit breaker states
    testCases := []struct {
        state float64
        name  string
    }{
        {0, "closed"},
        {1, "open"},
        {2, "half-open"},
    }
    
    for _, tc := range testCases {
        metrics.circuitBreakerState.WithLabelValues("btc_rpc").Set(tc.state)
        
        // Assert
        metricFamilies, err := registry.Gather()
        assert.NoError(t, err)
        
        for _, mf := range metricFamilies {
            if mf.GetName() == "icy_backend_circuit_breaker_state" {
                metric := mf.GetMetric()[0]
                assert.Equal(t, tc.state, metric.GetGauge().GetValue(),
                    "Circuit breaker state not set correctly for %s", tc.name)
            }
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
    
    // Act - Record different timeout types
    timeoutTypes := []string{"connection", "request", "health_check"}
    
    for _, timeoutType := range timeoutTypes {
        metrics.timeouts.WithLabelValues("btc_rpc", timeoutType).Inc()
    }
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_external_api_timeouts_total" {
            assert.Equal(t, len(timeoutTypes), len(mf.GetMetric()))
            
            for _, metric := range mf.GetMetric() {
                assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                
                labels := metric.GetLabel()
                timeoutType := getLabelValue(labels, "timeout_type")
                assert.Contains(t, timeoutTypes, timeoutType)
            }
        }
    }
}
```

### 4. Data Sanitization Tests

#### TestDataSanitizer_AddressSanitization
```go
func TestDataSanitizer_AddressSanitization(t *testing.T) {
    sanitizer := NewDataSanitizer(setupTestLogger())
    
    tests := []struct {
        name     string
        address  string
        expected string
    }{
        {
            name:     "Bitcoin address",
            address:  "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
            expected: "1A1zP1eP..DivfNa",
        },
        {
            name:     "Ethereum address",
            address:  "0x742d35Cc6323C56db4DB5E8A7Cc7D25C1e3C82F6",
            expected: "0x742d35..3C82F6",
        },
        {
            name:     "Short address",
            address:  "short",
            expected: "[REDACTED]",
        },
        {
            name:     "Empty address",
            address:  "",
            expected: "[REDACTED]",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := sanitizer.SanitizeAddress(tt.address)
            assert.Equal(t, tt.expected, result)
            
            // Ensure no full address is exposed
            if len(tt.address) > 16 {
                assert.NotEqual(t, tt.address, result)
                assert.True(t, len(result) < len(tt.address))
            }
        })
    }
}
```

#### TestDataSanitizer_AmountSanitization
```go
func TestDataSanitizer_AmountSanitization(t *testing.T) {
    sanitizer := NewDataSanitizer(setupTestLogger())
    
    tests := []struct {
        name     string
        amount   *model.Web3BigInt
        expected string
    }{
        {
            name:     "Micro amount",
            amount:   createWeb3BigInt("50000"), // 0.0005 BTC
            expected: "micro",
        },
        {
            name:     "Small amount",
            amount:   createWeb3BigInt("500000"), // 0.005 BTC
            expected: "small",
        },
        {
            name:     "Medium amount",
            amount:   createWeb3BigInt("5000000"), // 0.05 BTC
            expected: "medium",
        },
        {
            name:     "Large amount",
            amount:   createWeb3BigInt("50000000"), // 0.5 BTC
            expected: "large",
        },
        {
            name:     "Extra large amount",
            amount:   createWeb3BigInt("150000000"), // 1.5 BTC
            expected: "xlarge",
        },
        {
            name:     "Nil amount",
            amount:   nil,
            expected: "unknown",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := sanitizer.SanitizeAmount(tt.amount)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### TestDataSanitizer_ErrorMessageSanitization
```go
func TestDataSanitizer_ErrorMessageSanitization(t *testing.T) {
    sanitizer := NewDataSanitizer(setupTestLogger())
    
    tests := []struct {
        name     string
        error    error
        expected []string // Patterns that should NOT be in result
        allowed  []string // Patterns that should be in result
    }{
        {
            name:     "Bitcoin address in error",
            error:    errors.New("invalid address 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
            expected: []string{"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"},
            allowed:  []string{"[BTC_ADDRESS]"},
        },
        {
            name:     "Ethereum address in error",
            error:    errors.New("failed to send to 0x742d35Cc6323C56db4DB5E8A7Cc7D25C1e3C82F6"),
            expected: []string{"0x742d35Cc6323C56db4DB5E8A7Cc7D25C1e3C82F6"},
            allowed:  []string{"[ETH_ADDRESS]"},
        },
        {
            name:     "Transaction hash in error",
            error:    errors.New("tx hash 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef failed"),
            expected: []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
            allowed:  []string{"[HASH]"},
        },
        {
            name:     "Amount in error",
            error:    errors.New(`invalid request: {"amount":"1.5"}`),
            expected: []string{`"amount":"1.5"`},
            allowed:  []string{`"amount":"[REDACTED]"`},
        },
        {
            name:     "Private key mention",
            error:    errors.New("private key validation failed"),
            expected: []string{"private key"},
            allowed:  []string{"[PRIVATE_KEY]"},
        },
        {
            name:     "Secret mention", 
            error:    errors.New("secret token expired"),
            expected: []string{"secret"},
            allowed:  []string{"[SECRET]"},
        },
        {
            name:     "Nil error",
            error:    nil,
            expected: []string{},
            allowed:  []string{},
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := sanitizer.SanitizeErrorMessage(tt.error)
            
            if tt.error == nil {
                assert.Empty(t, result)
                return
            }
            
            // Check that sensitive patterns are removed
            for _, pattern := range tt.expected {
                assert.NotContains(t, result, pattern,
                    "Sensitive pattern '%s' found in sanitized error", pattern)
            }
            
            // Check that replacement patterns are present
            for _, pattern := range tt.allowed {
                assert.Contains(t, result, pattern,
                    "Expected replacement pattern '%s' not found", pattern)
            }
        })
    }
}
```

#### TestDataSanitizer_HashSensitiveData
```go
func TestDataSanitizer_HashSensitiveData(t *testing.T) {
    sanitizer := NewDataSanitizer(setupTestLogger())
    
    tests := []struct {
        name string
        data string
    }{
        {"private_key", "5KJvsngHeMpm884wtkJNzQGaCErckhHJBGFsvd3VyK5qMZXj3hS"},
        {"secret_token", "sk_live_1234567890abcdef"},
        {"user_id", "user123"},
        {"transaction_id", "tx_abc123def456"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := sanitizer.HashSensitiveData(tt.data)
            
            // Should start with "hash_"
            assert.True(t, strings.HasPrefix(result, "hash_"),
                "Hash result should start with 'hash_'")
            
            // Should not contain original data
            assert.NotContains(t, result, tt.data,
                "Original data should not be in hash result")
            
            // Should be consistent (same input = same output)
            result2 := sanitizer.HashSensitiveData(tt.data)
            assert.Equal(t, result, result2,
                "Hash should be consistent for same input")
            
            // Different inputs should produce different hashes
            if tt.data != "" {
                differentResult := sanitizer.HashSensitiveData(tt.data + "_different")
                assert.NotEqual(t, result, differentResult,
                    "Different inputs should produce different hashes")
            }
        })
    }
}
```

### 5. Metrics Registry Tests

#### TestMetricsRegistry_Registration
```go
func TestMetricsRegistry_Registration(t *testing.T) {
    // Arrange
    registry := NewMetricsRegistry()
    
    // Act - Registry should be created with metrics registered
    metricFamilies, err := registry.registry.Gather()
    assert.NoError(t, err)
    
    // Assert - Check that expected metrics families are registered
    expectedMetrics := []string{
        "icy_backend_http_requests_total",
        "icy_backend_http_request_duration_seconds",
        "icy_backend_http_active_requests",
        "icy_backend_oracle_data_age_seconds",
        "icy_backend_oracle_operations_total",
        "icy_backend_swap_operations_total",
        "icy_backend_external_api_duration_seconds",
        "icy_backend_external_api_calls_total",
        "icy_backend_circuit_breaker_state",
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

#### TestMetricsRegistry_HTTPHandler
```go
func TestMetricsRegistry_HTTPHandler(t *testing.T) {
    // Arrange
    registry := NewMetricsRegistry()
    handler := registry.Handler()
    
    // Act
    w := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/metrics", nil)
    handler.ServeHTTP(w, req)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
    
    body := w.Body.String()
    assert.Contains(t, body, "icy_backend_http_requests_total")
    assert.Contains(t, body, "TYPE counter")
    assert.Contains(t, body, "HELP")
}
```

#### TestMetricsRegistry_MetricsCount
```go
func TestMetricsRegistry_MetricsCount(t *testing.T) {
    // Arrange
    registry := NewMetricsRegistry()
    
    // Record some metrics
    registry.HTTPMetrics().requestsTotal.WithLabelValues("GET", "/test", "200").Inc()
    registry.BusinessMetrics().oracleOperations.WithLabelValues("ratio", "success").Inc()
    
    // Act
    count := registry.GetMetricsCount()
    
    // Assert - Should have at least the metrics we just recorded
    assert.True(t, count >= 2, "Expected at least 2 metrics, got %d", count)
}
```

### 6. Cardinality Control Tests

#### TestMetrics_CardinalityLimits
```go
func TestMetrics_CardinalityLimits(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    // Act - Try to create high cardinality by using many different endpoints
    endpoints := make([]string, 2000) // More than recommended cardinality
    for i := 0; i < 2000; i++ {
        endpoints[i] = fmt.Sprintf("/dynamic/endpoint/%d", i)
    }
    
    for _, endpoint := range endpoints {
        normalizedEndpoint := normalizeEndpoint(endpoint)
        metrics.requestsTotal.WithLabelValues("GET", normalizedEndpoint, "200").Inc()
    }
    
    // Assert - Should be normalized to control cardinality
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_http_requests_total" {
            // Should be normalized to "other" to prevent cardinality explosion
            assert.True(t, len(mf.GetMetric()) < 100,
                "Too many metric series created: %d", len(mf.GetMetric()))
            
            // Check that endpoints were normalized
            foundOther := false
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                endpoint := getLabelValue(labels, "endpoint")
                if endpoint == "other" {
                    foundOther = true
                    break
                }
            }
            assert.True(t, foundOther, "Dynamic endpoints should be normalized to 'other'")
        }
    }
}
```

#### TestMetrics_MemoryUsage
```go
func TestMetrics_MemoryUsage(t *testing.T) {
    // Arrange
    registry := NewMetricsRegistry()
    
    var m1, m2 runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&m1)
    
    // Act - Create many metrics within cardinality limits
    for i := 0; i < 1000; i++ {
        registry.HTTPMetrics().requestsTotal.WithLabelValues("GET", "/api/v1/oracle/*", "200").Inc()
        registry.BusinessMetrics().oracleOperations.WithLabelValues("ratio", "success").Inc()
    }
    
    runtime.GC()
    runtime.ReadMemStats(&m2)
    
    // Assert - Memory usage should be reasonable
    memoryIncrease := m2.Alloc - m1.Alloc
    assert.True(t, memoryIncrease < 10*1024*1024, // Less than 10MB
        "Memory usage too high: %d bytes", memoryIncrease)
}
```

## Performance Benchmarks

```go
func BenchmarkHTTPMetrics_Middleware(b *testing.B) {
    registry := prometheus.NewRegistry()
    metrics := NewHTTPMetrics()
    metrics.MustRegister(registry)
    
    gin.SetMode(gin.TestMode)
    router := gin.New() 
    router.Use(metrics.Middleware())
    router.GET("/test", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "ok"})
    })
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        w := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/test", nil)
        router.ServeHTTP(w, req)
    }
}

func BenchmarkDataSanitizer_SanitizeAddress(b *testing.B) {
    sanitizer := NewDataSanitizer(setupTestLogger())
    address := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        sanitizer.SanitizeAddress(address)
    }
}

func BenchmarkBusinessMetrics_OracleInstrumentation(b *testing.B) {
    registry := prometheus.NewRegistry()
    metrics := NewBusinessMetrics()
    metrics.MustRegister(registry)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        metrics.oracleCalculationDuration.WithLabelValues("ratio").Observe(time.Since(start).Seconds())
        metrics.oracleOperations.WithLabelValues("ratio", "success").Inc()
    }
}
```

## Test Helper Functions

```go
func getLabelValue(labels []*dto.LabelPair, name string) string {
    for _, label := range labels {
        if label.GetName() == name {
            return label.GetValue()
        }
    }
    return ""
}

func createWeb3BigInt(value string) *model.Web3BigInt {
    // Implementation to create Web3BigInt from string
}

func setupMockOracleHandler(t *testing.T) handler.IOracleHandler {
    // Implementation for mock oracle handler
}

func setupMockSwapHandler(t *testing.T) handler.ISwapHandler {
    // Implementation for mock swap handler
}

func setupTestLogger() *logger.Logger {
    // Implementation for test logger
}
```

## Coverage Requirements

- **Function Coverage**: 100% of all metrics collection functions
- **Branch Coverage**: >90% including error paths and edge cases
- **Performance Coverage**: Benchmark tests for all critical paths
- **Security Coverage**: Data sanitization validation
- **Cardinality Coverage**: High cardinality protection testing