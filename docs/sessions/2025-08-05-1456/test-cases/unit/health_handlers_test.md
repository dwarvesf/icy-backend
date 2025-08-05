# Health Handlers Unit Test Cases

**File**: `internal/handler/health/health_test.go`  
**Package**: `health`  
**Target Coverage**: >90%  

## Test Suite Overview

Comprehensive unit tests for all health handler methods including basic health, database health, external API health, and background job health endpoints. Focus on various scenarios, error handling, timeout behavior, and response format validation.

## Test Cases

### 1. Basic Health Endpoint Tests

#### TestHealthHandler_Basic_Success
```go
func TestHealthHandler_Basic_Success(t *testing.T) {
    // Arrange
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    handler := &HealthHandler{}
    
    // Act
    handler.Basic(c)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    
    var response map[string]string
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    assert.Equal(t, "ok", response["message"])
    
    // Performance assertion
    assert.True(t, w.Header().Get("Content-Length") != "")
}
```

#### TestHealthHandler_Basic_ResponseTime
```go
func TestHealthHandler_Basic_ResponseTime(t *testing.T) {
    // Test SLA requirement: < 200ms response time
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    handler := &HealthHandler{}
    
    start := time.Now()
    handler.Basic(c)
    duration := time.Since(start)
    
    // Assert response time SLA
    assert.True(t, duration < 200*time.Millisecond, 
        "Basic health check exceeded SLA: %v", duration)
    assert.Equal(t, http.StatusOK, w.Code)
}
```

### 2. Database Health Tests

#### TestHealthHandler_Database_Healthy
```go
func TestHealthHandler_Database_Healthy(t *testing.T) {
    // Arrange
    mockDB := setupMockHealthyDatabase(t)
    defer cleanupMockDatabase(mockDB)
    
    handler := &HealthHandler{
        db:     mockDB,
        logger: setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.Database(c)
    duration := time.Since(start)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    assert.True(t, duration < 500*time.Millisecond, 
        "Database health check exceeded SLA: %v", duration)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "healthy", response.Status)
    assert.Contains(t, response.Checks, "database")
    
    dbCheck := response.Checks["database"]
    assert.Equal(t, "healthy", dbCheck.Status)
    assert.True(t, dbCheck.Latency > 0)
    assert.Empty(t, dbCheck.Error)
    
    // Validate metadata
    assert.Contains(t, dbCheck.Metadata, "driver")
    assert.Equal(t, "postgres", dbCheck.Metadata["driver"])
    assert.Contains(t, dbCheck.Metadata, "connection_pool")
}
```

#### TestHealthHandler_Database_Unhealthy_ConnectionFailure
```go
func TestHealthHandler_Database_Unhealthy_ConnectionFailure(t *testing.T) {
    // Arrange
    mockDB := setupMockFailingDatabase(t, errors.New("connection refused"))
    handler := &HealthHandler{
        db:     mockDB,
        logger: setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    handler.Database(c)
    
    // Assert
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    assert.Contains(t, response.Checks, "database")
    
    dbCheck := response.Checks["database"]
    assert.Equal(t, "unhealthy", dbCheck.Status)
    assert.Contains(t, dbCheck.Error, "connection refused")
    assert.True(t, dbCheck.Latency > 0)
}
```

#### TestHealthHandler_Database_Timeout
```go
func TestHealthHandler_Database_Timeout(t *testing.T) {
    // Arrange
    mockDB := setupMockSlowDatabase(t, 6*time.Second) // Exceeds 5s timeout
    handler := &HealthHandler{
        db:     mockDB,
        logger: setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.Database(c)
    duration := time.Since(start)
    
    // Assert
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
    assert.True(t, duration >= 5*time.Second && duration < 6*time.Second,
        "Database timeout not enforced correctly: %v", duration)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    dbCheck := response.Checks["database"]
    assert.Contains(t, dbCheck.Error, "timeout")
}
```

#### TestHealthHandler_Database_ConnectionPoolMetrics
```go
func TestHealthHandler_Database_ConnectionPoolMetrics(t *testing.T) {
    // Arrange
    mockDB := setupMockDatabaseWithPoolStats(t, sql.DBStats{
        OpenConnections: 5,
        InUse:          2,
        Idle:           3,
        MaxOpenConnections: 10,
    })
    
    handler := &HealthHandler{
        db:     mockDB,
        logger: setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    handler.Database(c)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    dbCheck := response.Checks["database"]
    poolInfo := dbCheck.Metadata["connection_pool"].(map[string]interface{})
    
    assert.Equal(t, 5, int(poolInfo["open_connections"].(float64)))
    assert.Equal(t, 2, int(poolInfo["in_use"].(float64)))
    assert.Equal(t, 3, int(poolInfo["idle"].(float64)))
    assert.Equal(t, 10, int(poolInfo["max_open"].(float64)))
}
```

### 3. External API Health Tests

#### TestHealthHandler_External_AllHealthy
```go
func TestHealthHandler_External_AllHealthy(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockHealthyBtcRPC(t)
    mockBaseRPC := setupMockHealthyBaseRPC(t)
    
    handler := &HealthHandler{
        btcRPC:  mockBtcRPC,
        baseRPC: mockBaseRPC,
        logger:  setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.External(c)
    duration := time.Since(start)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    assert.True(t, duration < 2000*time.Millisecond,
        "External API health check exceeded SLA: %v", duration)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "healthy", response.Status)
    assert.Contains(t, response.Checks, "blockstream_api")
    assert.Contains(t, response.Checks, "base_rpc")
    
    // Validate Bitcoin API check
    btcCheck := response.Checks["blockstream_api"]
    assert.Equal(t, "healthy", btcCheck.Status)
    assert.True(t, btcCheck.Latency > 0 && btcCheck.Latency < 3000) // < 3s timeout
    assert.Contains(t, btcCheck.Metadata, "endpoint")
    assert.Equal(t, "blockstream.info", btcCheck.Metadata["endpoint"])
    
    // Validate Base RPC check
    baseCheck := response.Checks["base_rpc"]
    assert.Equal(t, "healthy", baseCheck.Status)
    assert.True(t, baseCheck.Latency > 0 && baseCheck.Latency < 3000)
    assert.Contains(t, baseCheck.Metadata, "endpoint")
}
```

#### TestHealthHandler_External_PartiallyUnhealthy
```go
func TestHealthHandler_External_PartiallyUnhealthy(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockFailingBtcRPC(t, errors.New("API rate limit exceeded"))
    mockBaseRPC := setupMockHealthyBaseRPC(t)
    
    handler := &HealthHandler{
        btcRPC:  mockBtcRPC,
        baseRPC: mockBaseRPC,
        logger:  setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    handler.External(c)
    
    // Assert
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    
    // Bitcoin API should be unhealthy
    btcCheck := response.Checks["blockstream_api"]
    assert.Equal(t, "unhealthy", btcCheck.Status)
    assert.Contains(t, btcCheck.Error, "API rate limit exceeded")
    
    // Base RPC should be healthy
    baseCheck := response.Checks["base_rpc"]
    assert.Equal(t, "healthy", baseCheck.Status)
    assert.Empty(t, baseCheck.Error)
}
```

#### TestHealthHandler_External_Timeout
```go
func TestHealthHandler_External_Timeout(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockSlowBtcRPC(t, 4*time.Second) // Exceeds 3s timeout
    mockBaseRPC := setupMockSlowBaseRPC(t, 4*time.Second)
    
    handler := &HealthHandler{
        btcRPC:  mockBtcRPC,
        baseRPC: mockBaseRPC,
        logger:  setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.External(c)
    duration := time.Since(start)
    
    // Assert
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
    assert.True(t, duration >= 3*time.Second && duration < 11*time.Second,
        "External API timeout not enforced correctly: %v", duration)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    
    // Check that at least one API timed out
    timeoutFound := false
    for _, check := range response.Checks {
        if strings.Contains(check.Error, "timeout") {
            timeoutFound = true
            break
        }
    }
    assert.True(t, timeoutFound, "Expected timeout error in response")
}
```

#### TestHealthHandler_External_ParallelExecution
```go
func TestHealthHandler_External_ParallelExecution(t *testing.T) {
    // Arrange
    mockBtcRPC := setupMockSlowBtcRPC(t, 1*time.Second)
    mockBaseRPC := setupMockSlowBaseRPC(t, 1*time.Second)
    
    handler := &HealthHandler{
        btcRPC:  mockBtcRPC,
        baseRPC: mockBaseRPC,
        logger:  setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.External(c)
    duration := time.Since(start)
    
    // Assert parallel execution - should be ~1s, not ~2s
    assert.True(t, duration < 1500*time.Millisecond,
        "APIs not executed in parallel: %v", duration)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Contains(t, response.Checks, "blockstream_api")
    assert.Contains(t, response.Checks, "base_rpc")
}
```

#### TestHealthHandler_External_OverallTimeout
```go
func TestHealthHandler_External_OverallTimeout(t *testing.T) {
    // Arrange - Both APIs take 6 seconds, but overall timeout is 10 seconds
    mockBtcRPC := setupMockSlowBtcRPC(t, 6*time.Second)
    mockBaseRPC := setupMockSlowBaseRPC(t, 6*time.Second)
    
    handler := &HealthHandler{
        btcRPC:  mockBtcRPC,
        baseRPC: mockBaseRPC,
        logger:  setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.External(c)
    duration := time.Since(start)
    
    // Assert overall timeout (10s) is enforced
    assert.True(t, duration >= 6*time.Second && duration <= 11*time.Second,
        "Overall timeout not enforced correctly: %v", duration)
    
    var response HealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
}
```

### 4. Background Job Health Tests

#### TestHealthHandler_Jobs_AllHealthy
```go
func TestHealthHandler_Jobs_AllHealthy(t *testing.T) {
    // Arrange
    mockJobManager := setupMockHealthyJobManager(t)
    handler := &HealthHandler{
        jobStatusManager: mockJobManager,
        logger:          setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    start := time.Now()
    handler.Jobs(c)
    duration := time.Since(start)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    assert.True(t, duration < 100*time.Millisecond,
        "Jobs health check exceeded expected time: %v", duration)
    
    var response JobsHealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "healthy", response.Status)
    assert.True(t, len(response.Jobs) > 0)
    assert.Equal(t, 0, response.Summary.StalledJobs)
    assert.Equal(t, 0, response.Summary.UnhealthyJobs)
}
```

#### TestHealthHandler_Jobs_StalledJobs
```go
func TestHealthHandler_Jobs_StalledJobs(t *testing.T) {
    // Arrange
    mockJobManager := setupMockJobManagerWithStalledJobs(t)
    handler := &HealthHandler{
        jobStatusManager: mockJobManager,
        logger:          setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    handler.Jobs(c)
    
    // Assert
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
    
    var response JobsHealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    assert.True(t, response.Summary.StalledJobs > 0)
}
```

#### TestHealthHandler_Jobs_CriticalJobFailures
```go
func TestHealthHandler_Jobs_CriticalJobFailures(t *testing.T) {
    // Arrange
    mockJobManager := setupMockJobManagerWithCriticalFailures(t)
    handler := &HealthHandler{
        jobStatusManager: mockJobManager,
        logger:          setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    handler.Jobs(c)
    
    // Assert
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
    
    var response JobsHealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "unhealthy", response.Status)
    
    // Check that critical jobs are failing
    criticalJobs := []string{
        "btc_transaction_indexing",
        "icy_transaction_indexing", 
        "swap_request_processing",
    }
    
    failedCriticalJobs := 0
    for _, jobName := range criticalJobs {
        if job, exists := response.Jobs[jobName]; exists {
            if job.Status == "failed" && job.ConsecutiveFailures > 2 {
                failedCriticalJobs++
            }
        }
    }
    
    assert.True(t, failedCriticalJobs > 0)
}
```

#### TestHealthHandler_Jobs_DegradedState
```go
func TestHealthHandler_Jobs_DegradedState(t *testing.T) {
    // Arrange - Non-critical jobs failing
    mockJobManager := setupMockJobManagerWithNonCriticalFailures(t)
    handler := &HealthHandler{
        jobStatusManager: mockJobManager,
        logger:          setupTestLogger(),
    }
    
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    
    // Act
    handler.Jobs(c)
    
    // Assert - Should return 206 Partial Content for degraded state
    assert.Equal(t, http.StatusPartialContent, w.Code)
    
    var response JobsHealthResponse
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    
    assert.Equal(t, "degraded", response.Status)
    assert.True(t, response.Summary.UnhealthyJobs > 0)
    assert.Equal(t, 0, response.Summary.StalledJobs) // No stalled jobs
}
```

### 5. Error Handling and Edge Cases

#### TestHealthHandler_NilDependencies
```go
func TestHealthHandler_NilDependencies(t *testing.T) {
    tests := []struct {
        name    string
        handler *HealthHandler
        method  string
    }{
        {
            name: "Database health with nil DB",
            handler: &HealthHandler{
                db:     nil,
                logger: setupTestLogger(),
            },
            method: "Database",
        },
        {
            name: "External health with nil RPC",
            handler: &HealthHandler{
                btcRPC: nil,
                baseRPC: nil,
                logger: setupTestLogger(),
            },
            method: "External",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            gin.SetMode(gin.TestMode)
            w := httptest.NewRecorder()
            c, _ := gin.CreateTestContext(w)
            
            // Act
            switch tt.method {
            case "Database":
                tt.handler.Database(c)
            case "External":
                tt.handler.External(c)
            }
            
            // Assert - Should handle gracefully
            assert.Equal(t, http.StatusServiceUnavailable, w.Code)
            
            var response HealthResponse
            err := json.Unmarshal(w.Body.Bytes(), &response)
            assert.NoError(t, err)
            assert.Equal(t, "unhealthy", response.Status)
        })
    }
}
```

#### TestHealthHandler_ResponseFormat
```go
func TestHealthHandler_ResponseFormat(t *testing.T) {
    // Test response format compliance for all endpoints
    tests := []struct {
        name       string
        setupFunc  func(t *testing.T) *HealthHandler
        method     string
        expectedFields []string
    }{
        {
            name:      "Basic health response format",
            setupFunc: setupBasicHealthHandler,
            method:    "Basic",
            expectedFields: []string{"message"},
        },
        {
            name:      "Database health response format", 
            setupFunc: setupDatabaseHealthHandler,
            method:    "Database",
            expectedFields: []string{"status", "timestamp", "checks", "duration_ms"},
        },
        {
            name:      "External health response format",
            setupFunc: setupExternalHealthHandler, 
            method:    "External",
            expectedFields: []string{"status", "timestamp", "checks", "duration_ms"},
        },
        {
            name:      "Jobs health response format",
            setupFunc: setupJobsHealthHandler,
            method:    "Jobs", 
            expectedFields: []string{"status", "timestamp", "jobs", "summary", "duration_ms"},
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := tt.setupFunc(t)
            
            gin.SetMode(gin.TestMode)
            w := httptest.NewRecorder()
            c, _ := gin.CreateTestContext(w)
            
            // Act
            switch tt.method {
            case "Basic":
                handler.Basic(c)
            case "Database":
                handler.Database(c)
            case "External":
                handler.External(c)
            case "Jobs":
                handler.Jobs(c)
            }
            
            // Assert response format
            var response map[string]interface{}
            err := json.Unmarshal(w.Body.Bytes(), &response)
            assert.NoError(t, err)
            
            for _, field := range tt.expectedFields {
                assert.Contains(t, response, field, 
                    "Missing required field: %s", field)
            }
        })
    }
}
```

#### TestHealthHandler_ConcurrentRequests
```go
func TestHealthHandler_ConcurrentRequests(t *testing.T) {
    // Test thread safety of health handlers
    handler := &HealthHandler{
        db:      setupMockHealthyDatabase(t),
        btcRPC:  setupMockHealthyBtcRPC(t),
        baseRPC: setupMockHealthyBaseRPC(t),
        logger:  setupTestLogger(),
    }
    
    const numRequests = 50
    var wg sync.WaitGroup
    results := make(chan int, numRequests)
    
    // Act - Concurrent requests
    for i := 0; i < numRequests; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            gin.SetMode(gin.TestMode)
            w := httptest.NewRecorder()
            c, _ := gin.CreateTestContext(w)
            
            handler.Database(c)
            results <- w.Code
        }()
    }
    
    wg.Wait()
    close(results)
    
    // Assert - All requests should succeed
    successCount := 0
    for code := range results {
        if code == http.StatusOK {
            successCount++
        }
    }
    
    assert.Equal(t, numRequests, successCount, 
        "Not all concurrent requests succeeded")
}
```

## Test Helper Functions

### Mock Setup Functions

```go
func setupMockHealthyDatabase(t *testing.T) *gorm.DB {
    // Implementation details for healthy database mock
}

func setupMockFailingDatabase(t *testing.T, err error) *gorm.DB {
    // Implementation details for failing database mock
}

func setupMockSlowDatabase(t *testing.T, delay time.Duration) *gorm.DB {
    // Implementation details for slow database mock
}

func setupMockHealthyBtcRPC(t *testing.T) btcrpc.IBtcRpc {
    // Implementation details for healthy Bitcoin RPC mock
}

func setupMockFailingBtcRPC(t *testing.T, err error) btcrpc.IBtcRpc {
    // Implementation details for failing Bitcoin RPC mock
}

func setupMockHealthyBaseRPC(t *testing.T) baserpc.IBaseRPC {
    // Implementation details for healthy Base RPC mock
}

func setupMockHealthyJobManager(t *testing.T) *monitoring.JobStatusManager {
    // Implementation details for healthy job manager mock
}

func setupTestLogger() *logger.Logger {
    // Implementation details for test logger
}
```

## Performance Benchmarks

```go
func BenchmarkHealthHandler_Basic(b *testing.B) {
    handler := &HealthHandler{}
    gin.SetMode(gin.TestMode)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        w := httptest.NewRecorder()
        c, _ := gin.CreateTestContext(w)
        handler.Basic(c)
    }
}

func BenchmarkHealthHandler_Database(b *testing.B) {
    handler := &HealthHandler{
        db:     setupMockHealthyDatabase(nil),
        logger: setupTestLogger(),
    }
    gin.SetMode(gin.TestMode)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        w := httptest.NewRecorder()
        c, _ := gin.CreateTestContext(w)
        handler.Database(c)
    }
}
```

## Test Configuration

```go
// Test configuration constants
const (
    TestTimeout = 30 * time.Second
    HealthCheckSLABasic = 200 * time.Millisecond
    HealthCheckSLADatabase = 500 * time.Millisecond
    HealthCheckSLAExternal = 2000 * time.Millisecond
    HealthCheckSLAJobs = 100 * time.Millisecond
)
```

## Coverage Requirements

- **Function Coverage**: 100% of all handler methods
- **Branch Coverage**: >90% including all error paths
- **Edge Case Coverage**: Nil checks, timeout scenarios, concurrent access
- **Performance Coverage**: SLA compliance validation
- **Security Coverage**: No sensitive data in responses