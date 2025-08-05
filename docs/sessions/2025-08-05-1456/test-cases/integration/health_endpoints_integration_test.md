# Health Endpoints Integration Test Cases

**File**: `internal/handler/health/health_integration_test.go`  
**Package**: `health_integration`  
**Target Coverage**: End-to-end health endpoint functionality  

## Test Suite Overview

Comprehensive integration tests for health endpoints with real database connections, external API interactions, and complete HTTP request flows. Focus on validating SLA compliance, error handling, and monitoring integration in a cryptocurrency system environment.

## Test Cases

### 1. Basic Health Endpoint Integration

#### TestHealthEndpoint_BasicHealth_E2E
```go
func TestHealthEndpoint_BasicHealth_E2E(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServer(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 5 * time.Second}
    
    // Act
    start := time.Now()
    resp, err := client.Get(testServer.URL + "/healthz")
    responseTime := time.Since(start)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    // Verify SLA compliance
    assert.True(t, responseTime < 200*time.Millisecond,
        "Basic health check exceeded SLA: %v", responseTime)
    
    // Verify response format
    var response map[string]string
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    assert.Equal(t, "ok", response["message"])
    
    // Verify headers
    assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
    
    resp.Body.Close()
}
```

#### TestHealthEndpoint_BasicHealth_HighLoad
```go
func TestHealthEndpoint_BasicHealth_HighLoad(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServer(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 5 * time.Second}
    const numRequests = 100
    const concurrency = 10
    
    var wg sync.WaitGroup
    results := make(chan time.Duration, numRequests)
    errors := make(chan error, numRequests)
    
    // Act - Concurrent requests
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for j := 0; j < numRequests/concurrency; j++ {
                start := time.Now()
                resp, err := client.Get(testServer.URL + "/healthz")
                duration := time.Since(start)
                
                if err != nil {
                    errors <- err
                    continue
                }
                
                if resp.StatusCode != http.StatusOK {
                    errors <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
                    resp.Body.Close()
                    continue
                }
                
                resp.Body.Close()
                results <- duration
            }
        }()
    }
    
    wg.Wait()
    close(results)
    close(errors)
    
    // Assert
    errorCount := len(errors)
    assert.Equal(t, 0, errorCount, "No errors expected under load")
    
    resultCount := len(results)
    assert.Equal(t, numRequests, resultCount, "All requests should complete")
    
    // Verify SLA compliance under load
    slaViolations := 0
    totalDuration := time.Duration(0)
    
    for duration := range results {
        totalDuration += duration
        if duration >= 200*time.Millisecond {
            slaViolations++
        }
    }
    
    averageDuration := totalDuration / time.Duration(numRequests)
    assert.True(t, slaViolations < numRequests/10, 
        "Too many SLA violations: %d/%d", slaViolations, numRequests)
    assert.True(t, averageDuration < 200*time.Millisecond,
        "Average response time exceeded SLA: %v", averageDuration)
}
```

### 2. Database Health Integration Tests

#### TestHealthEndpoint_DatabaseHealth_RealDB
```go
func TestHealthEndpoint_DatabaseHealth_RealDB(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithRealDB(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 10 * time.Second}
    
    // Act
    start := time.Now()
    resp, err := client.Get(testServer.URL + "/api/v1/health/db")
    responseTime := time.Since(start)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    // Verify SLA compliance
    assert.True(t, responseTime < 500*time.Millisecond,
        "Database health check exceeded SLA: %v", responseTime)
    
    // Verify response structure
    var response HealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    assert.Equal(t, "healthy", response.Status)
    assert.Contains(t, response.Checks, "database")
    assert.True(t, response.Duration > 0)
    
    dbCheck := response.Checks["database"]
    assert.Equal(t, "healthy", dbCheck.Status)
    assert.True(t, dbCheck.Latency > 0)
    assert.Empty(t, dbCheck.Error)
    assert.Contains(t, dbCheck.Metadata, "driver")
    assert.Contains(t, dbCheck.Metadata, "connection_pool")
    
    resp.Body.Close()
}
```

#### TestHealthEndpoint_DatabaseHealth_ConnectionPool
```go
func TestHealthEndpoint_DatabaseHealth_ConnectionPool(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithRealDB(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 10 * time.Second}
    
    // Create multiple concurrent database health checks to test connection pool
    const numRequests = 20
    var wg sync.WaitGroup
    results := make(chan HealthResponse, numRequests)
    
    // Act - Concurrent database health checks
    for i := 0; i < numRequests; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            resp, err := client.Get(testServer.URL + "/api/v1/health/db")
            if err != nil {
                return
            }
            defer resp.Body.Close()
            
            if resp.StatusCode == http.StatusOK {
                var response HealthResponse
                if json.NewDecoder(resp.Body).Decode(&response) == nil {
                    results <- response
                }
            }
        }()
    }
    
    wg.Wait()
    close(results)
    
    // Assert
    healthyResponses := 0
    for response := range results {
        if response.Status == "healthy" {
            healthyResponses++
            
            dbCheck := response.Checks["database"]
            assert.Equal(t, "healthy", dbCheck.Status)
            
            // Verify connection pool information is present
            poolInfo, exists := dbCheck.Metadata["connection_pool"]
            assert.True(t, exists, "Connection pool info should be present")
            
            poolMap, ok := poolInfo.(map[string]interface{})
            assert.True(t, ok, "Connection pool should be a map")
            
            assert.Contains(t, poolMap, "open_connections")
            assert.Contains(t, poolMap, "in_use")
            assert.Contains(t, poolMap, "idle")
        }
    }
    
    assert.True(t, healthyResponses >= numRequests-2, 
        "Most requests should succeed: %d/%d", healthyResponses, numRequests)
}
```

#### TestHealthEndpoint_DatabaseHealth_SlowQuery
```go
func TestHealthEndpoint_DatabaseHealth_SlowQuery(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithSlowDB(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 10 * time.Second}
    
    // Act
    start := time.Now()
    resp, err := client.Get(testServer.URL + "/api/v1/health/db")
    responseTime := time.Since(start)
    
    // Assert
    assert.NoError(t, err)
    
    var response HealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    // Should still be healthy but with higher latency
    if response.Status == "healthy" {
        dbCheck := response.Checks["database"]
        assert.True(t, dbCheck.Latency > 100, "Should show realistic latency for slow query")
        assert.True(t, responseTime < 500*time.Millisecond, "Should still meet SLA")
    } else {
        // If unhealthy due to timeout, verify proper error handling
        assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
        assert.Equal(t, "unhealthy", response.Status)
    }
    
    resp.Body.Close()
}
```

### 3. External API Health Integration Tests

#### TestHealthEndpoint_ExternalAPIs_RealEndpoints
```go
func TestHealthEndpoint_ExternalAPIs_RealEndpoints(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping external API integration test in short mode")
    }
    
    // Arrange
    testServer := setupIntegrationTestServerWithRealAPIs(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 15 * time.Second}
    
    // Act
    start := time.Now()
    resp, err := client.Get(testServer.URL + "/api/v1/health/external")
    responseTime := time.Since(start)
    
    // Assert
    assert.NoError(t, err)
    
    // Should respond within SLA even if some external APIs are slow
    assert.True(t, responseTime < 2000*time.Millisecond,
        "External API health check exceeded SLA: %v", responseTime)
    
    var response HealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    // Response should contain both API checks
    assert.Contains(t, response.Checks, "blockstream_api")
    assert.Contains(t, response.Checks, "base_rpc")
    
    // Verify each API check has required fields
    for apiName, check := range response.Checks {
        assert.True(t, check.Status == "healthy" || check.Status == "unhealthy",
            "API %s should have valid status", apiName)
        assert.True(t, check.Latency >= 0, "Latency should be non-negative")
        assert.Contains(t, check.Metadata, "endpoint")
        
        if check.Status == "unhealthy" {
            assert.NotEmpty(t, check.Error, "Unhealthy API should have error message")
        }
    }
    
    resp.Body.Close()
}
```

#### TestHealthEndpoint_ExternalAPIs_WithCircuitBreaker
```go
func TestHealthEndpoint_ExternalAPIs_WithCircuitBreaker(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithMockAPIs(t, map[string]error{
        "btc_rpc":  errors.New("API temporarily unavailable"),
        "base_rpc": nil, // Healthy
    })
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 10 * time.Second}
    
    // First, make multiple requests to trigger circuit breaker
    for i := 0; i < 3; i++ {
        resp, _ := client.Get(testServer.URL + "/api/v1/health/external")
        if resp != nil {
            resp.Body.Close()
        }
        time.Sleep(100 * time.Millisecond)
    }
    
    // Act - Health check should now reflect circuit breaker state
    resp, err := client.Get(testServer.URL + "/api/v1/health/external")
    assert.NoError(t, err)
    defer resp.Body.Close()
    
    var response HealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    // Assert
    assert.Equal(t, "unhealthy", response.Status) // Overall unhealthy due to BTC API
    
    btcCheck := response.Checks["blockstream_api"]
    assert.Equal(t, "unhealthy", btcCheck.Status)
    
    baseCheck := response.Checks["base_rpc"]
    assert.Equal(t, "healthy", baseCheck.Status)
    
    // Verify circuit breaker state is reported
    if metadata, exists := btcCheck.Metadata["circuit_breaker_state"]; exists {
        state, ok := metadata.(string)
        assert.True(t, ok)
        assert.True(t, state == "open" || state == "half-open")
    }
}
```

#### TestHealthEndpoint_ExternalAPIs_Timeout
```go
func TestHealthEndpoint_ExternalAPIs_Timeout(t *testing.T) {
    // Arrange - Setup server with very slow external APIs
    testServer := setupIntegrationTestServerWithSlowAPIs(t, 5*time.Second)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 15 * time.Second}
    
    // Act
    start := time.Now()
    resp, err := client.Get(testServer.URL + "/api/v1/health/external")
    responseTime := time.Since(start)
    
    // Assert
    assert.NoError(t, err)
    
    // Should timeout the external API calls but still respond within overall SLA
    assert.True(t, responseTime < 2500*time.Millisecond,
        "Should respect overall timeout: %v", responseTime)
    
    var response HealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    // Should be unhealthy due to timeouts
    assert.Equal(t, "unhealthy", response.Status)
    
    // At least one API should have timed out
    timeoutFound := false
    for _, check := range response.Checks {
        if strings.Contains(check.Error, "timeout") {
            timeoutFound = true
            break
        }
    }
    assert.True(t, timeoutFound, "Expected at least one timeout error")
    
    resp.Body.Close()
}
```

### 4. Background Job Health Integration Tests

#### TestHealthEndpoint_JobsHealth_RealJobs
```go
func TestHealthEndpoint_JobsHealth_RealJobs(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithBackgroundJobs(t)
    defer testServer.Cleanup()
    
    // Start some background jobs
    testServer.StartBackgroundJob("btc_transaction_indexing", func() error {
        time.Sleep(50 * time.Millisecond)
        return nil
    })
    
    testServer.StartBackgroundJob("icy_transaction_indexing", func() error {
        time.Sleep(30 * time.Millisecond)
        return nil
    })
    
    // Wait for jobs to complete
    time.Sleep(200 * time.Millisecond)
    
    client := &http.Client{Timeout: 5 * time.Second}
    
    // Act
    resp, err := client.Get(testServer.URL + "/api/v1/health/jobs")
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    var response JobsHealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    assert.Equal(t, "healthy", response.Status)
    assert.True(t, response.Summary.TotalJobs >= 2)
    assert.True(t, response.Summary.HealthyJobs >= 2)
    assert.Equal(t, 0, response.Summary.StalledJobs)
    
    // Verify individual job statuses
    assert.Contains(t, response.Jobs, "btc_transaction_indexing")
    assert.Contains(t, response.Jobs, "icy_transaction_indexing")
    
    for jobName, job := range response.Jobs {
        assert.Equal(t, "success", string(job.Status), "Job %s should be successful", jobName)
        assert.True(t, job.LastDuration > 0, "Job %s should have recorded duration", jobName)
        assert.Equal(t, int64(1), job.SuccessCount, "Job %s should have success count", jobName)
    }
    
    resp.Body.Close()
}
```

#### TestHealthEndpoint_JobsHealth_FailedJobs
```go
func TestHealthEndpoint_JobsHealth_FailedJobs(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithBackgroundJobs(t)
    defer testServer.Cleanup()
    
    // Start jobs with some failures
    testServer.StartBackgroundJob("successful_job", func() error {
        return nil
    })
    
    testServer.StartBackgroundJob("failing_job", func() error {
        return errors.New("job processing failed")
    })
    
    testServer.StartBackgroundJob("btc_transaction_indexing", func() error {
        return errors.New("critical job failed") // Critical job failure
    })
    
    // Wait for jobs to complete
    time.Sleep(100 * time.Millisecond)
    
    client := &http.Client{Timeout: 5 * time.Second}
    
    // Act
    resp, err := client.Get(testServer.URL + "/api/v1/health/jobs")
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode) // Critical job failed
    
    var response JobsHealthResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    assert.True(t, response.Summary.UnhealthyJobs > 0)
    
    // Verify job details
    btcJob := response.Jobs["btc_transaction_indexing"]
    assert.Equal(t, "failed", string(btcJob.Status))
    assert.Contains(t, btcJob.LastError, "critical job failed")
    
    failingJob := response.Jobs["failing_job"]
    assert.Equal(t, "failed", string(failingJob.Status))
    assert.Contains(t, failingJob.LastError, "job processing failed")
    
    successfulJob := response.Jobs["successful_job"]
    assert.Equal(t, "success", string(successfulJob.Status))
    
    resp.Body.Close()
}
```

### 5. Complete Health Check Integration

#### TestHealthEndpoints_CompleteFlow
```go
func TestHealthEndpoints_CompleteFlow(t *testing.T) {
    // Arrange
    testServer := setupCompleteIntegrationTestServer(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 15 * time.Second}
    
    // Test all health endpoints in sequence
    endpoints := []struct {
        path           string
        expectedStatus int
        maxResponseTime time.Duration
    }{
        {"/healthz", http.StatusOK, 200 * time.Millisecond},
        {"/api/v1/health/db", http.StatusOK, 500 * time.Millisecond},
        {"/api/v1/health/external", http.StatusOK, 2000 * time.Millisecond},
        {"/api/v1/health/jobs", http.StatusOK, 100 * time.Millisecond},
    }
    
    results := make(map[string]struct {
        statusCode   int
        responseTime time.Duration
        body         []byte
    })
    
    // Act - Test each endpoint
    for _, endpoint := range endpoints {
        start := time.Now()
        resp, err := client.Get(testServer.URL + endpoint.path)
        responseTime := time.Since(start)
        
        assert.NoError(t, err, "Request to %s should not error", endpoint.path)
        
        body, err := io.ReadAll(resp.Body)
        assert.NoError(t, err)
        resp.Body.Close()
        
        results[endpoint.path] = struct {
            statusCode   int
            responseTime time.Duration
            body         []byte
        }{
            statusCode:   resp.StatusCode,
            responseTime: responseTime,
            body:         body,
        }
    }
    
    // Assert all endpoints
    for _, endpoint := range endpoints {
        result := results[endpoint.path]
        
        assert.Equal(t, endpoint.expectedStatus, result.statusCode,
            "Endpoint %s should return correct status", endpoint.path)
        
        assert.True(t, result.responseTime < endpoint.maxResponseTime,
            "Endpoint %s exceeded SLA: %v > %v", endpoint.path, result.responseTime, endpoint.maxResponseTime)
        
        // Verify response is valid JSON
        var jsonResponse interface{}
        err := json.Unmarshal(result.body, &jsonResponse)
        assert.NoError(t, err, "Endpoint %s should return valid JSON", endpoint.path)
    }
}
```

### 6. Authentication and Security Integration

#### TestHealthEndpoints_NoAuthentication
```go
func TestHealthEndpoints_NoAuthentication(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithAuth(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 5 * time.Second}
    
    // Health endpoints that should bypass authentication
    healthEndpoints := []string{
        "/healthz",
        "/api/v1/health/db",
        "/api/v1/health/external",
        "/api/v1/health/jobs",
    }
    
    // Protected endpoint for comparison
    protectedEndpoint := "/api/v1/oracle/ratio"
    
    // Act & Assert - Health endpoints should work without auth
    for _, endpoint := range healthEndpoints {
        resp, err := client.Get(testServer.URL + endpoint)
        assert.NoError(t, err, "Health endpoint %s should not require auth", endpoint)
        assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
            "Health endpoint %s should not return 401", endpoint)
        resp.Body.Close()
    }
    
    // Verify protected endpoint still requires auth
    resp, err := client.Get(testServer.URL + protectedEndpoint)
    assert.NoError(t, err)
    assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
        "Protected endpoint should require auth")
    resp.Body.Close()
}
```

#### TestHealthEndpoints_InformationDisclosure
```go
func TestHealthEndpoints_InformationDisclosure(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServer(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 10 * time.Second}
    
    // Act - Get all health endpoint responses
    endpoints := []string{
        "/healthz",
        "/api/v1/health/db",
        "/api/v1/health/external",
        "/api/v1/health/jobs",
    }
    
    responses := make(map[string]string)
    for _, endpoint := range endpoints {
        resp, err := client.Get(testServer.URL + endpoint)
        if err != nil {
            continue
        }
        
        body, err := io.ReadAll(resp.Body)
        resp.Body.Close()
        if err != nil {
            continue
        }
        
        responses[endpoint] = string(body)
    }
    
    // Assert - No sensitive information should be disclosed
    sensitivePatterns := []string{
        "password",
        "secret",
        "key",
        "token",
        "private",
        "localhost", // Internal hostnames
        "127.0.0.1",
        "192.168.",
        "10.0.",
        "172.16.",
        "database=", // Connection strings
        "postgres://",
        "mysql://",
    }
    
    for endpoint, responseBody := range responses {
        lowerBody := strings.ToLower(responseBody)
        
        for _, pattern := range sensitivePatterns {
            assert.NotContains(t, lowerBody, pattern,
                "Health endpoint %s should not expose sensitive info: %s", endpoint, pattern)
        }
        
        // Verify no stack traces or internal paths
        assert.NotContains(t, responseBody, "/usr/local/",
            "Health endpoint %s should not expose internal paths", endpoint)
        assert.NotContains(t, responseBody, "panic:",
            "Health endpoint %s should not expose panic information", endpoint)
    }
}
```

### 7. Metrics Integration Tests

#### TestHealthEndpoints_MetricsIntegration
```go
func TestHealthEndpoints_MetricsIntegration(t *testing.T) {
    // Arrange
    testServer := setupIntegrationTestServerWithMetrics(t)
    defer testServer.Cleanup()
    
    client := &http.Client{Timeout: 5 * time.Second}
    
    // Act - Make requests to health endpoints
    healthEndpoints := []string{
        "/healthz",
        "/api/v1/health/db",
        "/api/v1/health/external",
        "/api/v1/health/jobs",
    }
    
    for _, endpoint := range healthEndpoints {
        for i := 0; i < 3; i++ { // Multiple requests to generate metrics
            resp, err := client.Get(testServer.URL + endpoint)
            if err == nil {
                resp.Body.Close()
            }
        }
    }
    
    // Get metrics
    metricsResp, err := client.Get(testServer.URL + "/metrics")
    assert.NoError(t, err)
    defer metricsResp.Body.Close()
    
    metricsBody, err := io.ReadAll(metricsResp.Body)
    assert.NoError(t, err)
    
    metricsText := string(metricsBody)
    
    // Assert - Verify health endpoint metrics are recorded
    expectedMetrics := []string{
        "icy_backend_http_requests_total",
        "icy_backend_http_request_duration_seconds",
    }
    
    for _, metric := range expectedMetrics {
        assert.Contains(t, metricsText, metric,
            "Metrics should contain %s", metric)
    }
    
    // Verify health endpoint specific metrics
    for _, endpoint := range healthEndpoints {
        normalizedEndpoint := normalizeEndpointForTest(endpoint)
        metricLine := fmt.Sprintf(`method="GET",endpoint="%s"`, normalizedEndpoint)
        assert.Contains(t, metricsText, metricLine,
            "Metrics should contain entries for %s", endpoint)
    }
}
```

### 8. Load Testing Integration

#### TestHealthEndpoints_LoadTesting
```go
func TestHealthEndpoints_LoadTesting(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    // Arrange
    testServer := setupIntegrationTestServer(t)
    defer testServer.Cleanup()
    
    const (
        duration    = 30 * time.Second
        concurrency = 50
    )
    
    client := &http.Client{Timeout: 5 * time.Second}
    
    // Metrics collection
    var (
        totalRequests int64
        successCount  int64
        errorCount    int64
        slaViolations int64
    )
    
    ctx, cancel := context.WithTimeout(context.Background(), duration)
    defer cancel()
    
    var wg sync.WaitGroup
    
    // Act - Generate load
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for {
                select {
                case <-ctx.Done():
                    return
                default:
                    atomic.AddInt64(&totalRequests, 1)
                    
                    start := time.Now()
                    resp, err := client.Get(testServer.URL + "/healthz")
                    duration := time.Since(start)
                    
                    if err != nil {
                        atomic.AddInt64(&errorCount, 1)
                        continue
                    }
                    
                    if resp.StatusCode != http.StatusOK {
                        atomic.AddInt64(&errorCount, 1)
                        resp.Body.Close()
                        continue
                    }
                    
                    resp.Body.Close()
                    atomic.AddInt64(&successCount, 1)
                    
                    if duration >= 200*time.Millisecond {
                        atomic.AddInt64(&slaViolations, 1)
                    }
                }
            }
        }()
    }
    
    wg.Wait()
    
    // Assert
    t.Logf("Load test results: Total=%d, Success=%d, Errors=%d, SLA Violations=%d",
        totalRequests, successCount, errorCount, slaViolations)
    
    assert.True(t, totalRequests > 0, "Should have made requests")
    assert.True(t, successCount > 0, "Should have successful requests")
    
    // Error rate should be minimal
    errorRate := float64(errorCount) / float64(totalRequests)
    assert.True(t, errorRate < 0.01, "Error rate should be < 1%: %f", errorRate)
    
    // SLA violation rate should be acceptable
    slaViolationRate := float64(slaViolations) / float64(successCount)
    assert.True(t, slaViolationRate < 0.05, "SLA violation rate should be < 5%: %f", slaViolationRate)
    
    // Request rate should be reasonable
    requestRate := float64(totalRequests) / duration.Seconds()
    assert.True(t, requestRate > 100, "Should handle > 100 req/sec: %f", requestRate)
}
```

## Test Helper Functions

```go
type IntegrationTestServer struct {
    URL      string
    server   *httptest.Server
    cleanup  func()
    jobManager *monitoring.JobStatusManager
}

func setupIntegrationTestServer(t *testing.T) *IntegrationTestServer {
    // Setup complete test server with mocked dependencies
}

func setupIntegrationTestServerWithRealDB(t *testing.T) *IntegrationTestServer {
    // Setup test server with real database connection
}

func setupIntegrationTestServerWithRealAPIs(t *testing.T) *IntegrationTestServer {
    // Setup test server with real external API connections
}

func setupIntegrationTestServerWithMockAPIs(t *testing.T, apiErrors map[string]error) *IntegrationTestServer {
    // Setup test server with mock external APIs that can simulate errors
}

func setupIntegrationTestServerWithSlowDB(t *testing.T) *IntegrationTestServer {
    // Setup test server with database that has slow responses
}

func setupIntegrationTestServerWithSlowAPIs(t *testing.T, delay time.Duration) *IntegrationTestServer {
    // Setup test server with slow external APIs
}

func setupIntegrationTestServerWithBackgroundJobs(t *testing.T) *IntegrationTestServer {
    // Setup test server with background job monitoring
}

func setupCompleteIntegrationTestServer(t *testing.T) *IntegrationTestServer {
    // Setup complete test server with all components
}

func setupIntegrationTestServerWithAuth(t *testing.T) *IntegrationTestServer {
    // Setup test server with authentication middleware
}

func setupIntegrationTestServerWithMetrics(t *testing.T) *IntegrationTestServer {
    // Setup test server with metrics collection enabled
}

func (s *IntegrationTestServer) StartBackgroundJob(jobName string, jobFunc func() error) {
    // Start a background job for testing
}

func (s *IntegrationTestServer) Cleanup() {
    // Cleanup test server resources
}

func normalizeEndpointForTest(endpoint string) string {
    // Normalize endpoint for metrics comparison
}
```

## Test Configuration

```go
// Integration test configuration
const (
    TestTimeout = 30 * time.Second
    DBTestTimeout = 10 * time.Second
    ExternalAPITestTimeout = 15 * time.Second
    
    // SLA thresholds for validation
    BasicHealthSLA = 200 * time.Millisecond
    DatabaseHealthSLA = 500 * time.Millisecond
    ExternalHealthSLA = 2000 * time.Millisecond
    JobsHealthSLA = 100 * time.Millisecond
)

// Test environment variables
var (
    TestDatabaseURL = os.Getenv("TEST_DATABASE_URL")
    TestBitcoinAPI = os.Getenv("TEST_BITCOIN_API")
    TestBaseRPCURL = os.Getenv("TEST_BASE_RPC_URL")
)
```

## Coverage Requirements

- **End-to-End Coverage**: Complete request/response cycles
- **SLA Validation**: All endpoints meet performance requirements
- **Error Scenario Coverage**: Database failures, API timeouts, job failures
- **Security Coverage**: Authentication bypass, information disclosure prevention
- **Load Testing**: Performance under concurrent load
- **Metrics Integration**: Verify monitoring data collection