# Background Job Monitoring Unit Test Cases

**File**: `internal/monitoring/background_job_monitoring_test.go`  
**Package**: `monitoring`  
**Target Coverage**: >90%  

## Test Suite Overview

Comprehensive unit tests for background job monitoring including job status tracking, instrumented job execution, stalled job detection, metrics collection, and thread safety. Focus on cryptocurrency system job reliability and monitoring accuracy.

## Test Cases

### 1. Job Status Manager Tests

#### TestJobStatusManager_RegisterJob
```go
func TestJobStatusManager_RegisterJob(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    jsm := NewJobStatusManager(logger, metrics)
    jobName := "test_job"
    
    // Act
    jsm.RegisterJob(jobName)
    
    // Assert
    status, exists := jsm.GetJobStatus(jobName)
    assert.True(t, exists, "Job should be registered")
    assert.Equal(t, jobName, status.JobName)
    assert.Equal(t, JobStatusPending, status.Status)
    assert.Equal(t, int64(0), status.SuccessCount)
    assert.Equal(t, int64(0), status.FailureCount)
    assert.Equal(t, int64(0), status.ConsecutiveFailures)
    assert.NotNil(t, status.Metadata)
    assert.True(t, status.CreatedAt.After(time.Time{}))
    assert.True(t, status.UpdatedAt.After(time.Time{}))
}
```

#### TestJobStatusManager_RegisterJobTwice
```go
func TestJobStatusManager_RegisterJobTwice(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    jobName := "duplicate_job"
    
    // Act - Register same job twice
    jsm.RegisterJob(jobName)
    firstRegistration, _ := jsm.GetJobStatus(jobName)
    
    jsm.RegisterJob(jobName)
    secondRegistration, _ := jsm.GetJobStatus(jobName)
    
    // Assert - Should not overwrite existing registration
    assert.Equal(t, firstRegistration.CreatedAt, secondRegistration.CreatedAt)
    assert.Equal(t, firstRegistration.JobName, secondRegistration.JobName)
}
```

### 2. Job Execution Lifecycle Tests

#### TestJobStatusManager_StartJob
```go
func TestJobStatusManager_StartJob(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    jsm := NewJobStatusManager(logger, metrics)
    jobName := "btc_transaction_indexing"
    
    // Act
    startTime := time.Now()
    jsm.StartJob(jobName)
    
    // Assert
    status, exists := jsm.GetJobStatus(jobName)
    assert.True(t, exists)
    assert.Equal(t, JobStatusRunning, status.Status)
    assert.True(t, status.LastRunTime.After(startTime.Add(-1*time.Second)))
    assert.True(t, status.LastRunTime.Before(time.Now().Add(1*time.Second)))
    
    // Verify metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_background_jobs_active" {
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetGauge().GetValue())
        }
    }
}
```

#### TestJobStatusManager_CompleteJobSuccess
```go
func TestJobStatusManager_CompleteJobSuccess(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    jsm := NewJobStatusManager(logger, metrics)
    jobName := "icy_transaction_indexing"
    
    jsm.StartJob(jobName)
    time.Sleep(10 * time.Millisecond) // Simulate some execution time
    
    metadata := map[string]interface{}{
        "transactions_processed": 15,
        "blocks_scanned":        5,
    }
    
    // Act
    jsm.CompleteJob(jobName, nil, metadata)
    
    // Assert
    status, exists := jsm.GetJobStatus(jobName)
    assert.True(t, exists)
    assert.Equal(t, JobStatusSuccess, status.Status)
    assert.Equal(t, int64(1), status.SuccessCount)
    assert.Equal(t, int64(0), status.FailureCount)
    assert.Equal(t, int64(0), status.ConsecutiveFailures)
    assert.Empty(t, status.LastError)
    assert.True(t, status.LastDuration > 0)
    assert.Equal(t, status.LastDuration, status.AverageExecution)
    assert.Equal(t, status.LastDuration, status.MaxExecutionTime)
    assert.Equal(t, status.LastDuration, status.MinExecutionTime)
    
    // Check metadata
    assert.Equal(t, 15, status.Metadata["transactions_processed"])
    assert.Equal(t, 5, status.Metadata["blocks_scanned"])
    
    // Verify metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    successCountFound := false
    durationFound := false
    activeJobsFound := false
    
    for _, mf := range metricFamilies {
        switch mf.GetName() {
        case "icy_backend_background_job_runs_total":
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "job_name") == jobName &&
                   getLabelValue(labels, "status") == "success" {
                    successCountFound = true
                    assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                }
            }
        case "icy_backend_background_job_duration_seconds":
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "job_name") == jobName &&
                   getLabelValue(labels, "status") == "success" {
                    durationFound = true
                    assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
                    assert.True(t, metric.GetHistogram().GetSampleSum() > 0)
                }
            }
        case "icy_backend_background_jobs_active":
            activeJobsFound = true
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(0), metric.GetGauge().GetValue()) // Should be decremented
        }
    }
    
    assert.True(t, successCountFound, "Success count metric not found")
    assert.True(t, durationFound, "Duration metric not found")
    assert.True(t, activeJobsFound, "Active jobs metric not found")
}
```

#### TestJobStatusManager_CompleteJobFailure
```go
func TestJobStatusManager_CompleteJobFailure(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    jsm := NewJobStatusManager(logger, metrics)
    jobName := "swap_request_processing"
    
    jsm.StartJob(jobName)
    time.Sleep(5 * time.Millisecond)
    
    jobError := errors.New("database connection timeout")
    metadata := map[string]interface{}{
        "error_type": "database",
        "retry_count": 3,
    }
    
    // Act
    jsm.CompleteJob(jobName, jobError, metadata)
    
    // Assert
    status, exists := jsm.GetJobStatus(jobName)
    assert.True(t, exists)
    assert.Equal(t, JobStatusFailed, status.Status)
    assert.Equal(t, int64(0), status.SuccessCount)
    assert.Equal(t, int64(1), status.FailureCount)
    assert.Equal(t, int64(1), status.ConsecutiveFailures)
    assert.Equal(t, jobError.Error(), status.LastError)
    assert.True(t, status.LastDuration > 0)
    
    // Check metadata
    assert.Equal(t, "database", status.Metadata["error_type"])
    assert.Equal(t, 3, status.Metadata["retry_count"])
    
    // Verify error metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    errorCountFound := false
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_background_job_runs_total" {
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "job_name") == jobName &&
                   getLabelValue(labels, "status") == "error" {
                    errorCountFound = true
                    assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                }
            }
        }
    }
    
    assert.True(t, errorCountFound, "Error count metric not found")
}
```

#### TestJobStatusManager_MultipleJobExecutions
```go
func TestJobStatusManager_MultipleJobExecutions(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    jobName := "btc_transaction_indexing"
    
    // Act - Execute job multiple times with varying durations
    durations := []time.Duration{
        10 * time.Millisecond,
        20 * time.Millisecond,
        15 * time.Millisecond,
        25 * time.Millisecond,
        5 * time.Millisecond,
    }
    
    for i, duration := range durations {
        jsm.StartJob(jobName)
        time.Sleep(duration)
        
        var err error
        if i == 2 { // Third execution fails
            err = errors.New("temporary failure")
        }
        
        jsm.CompleteJob(jobName, err, map[string]interface{}{
            "execution_number": i + 1,
        })
    }
    
    // Assert
    status, exists := jsm.GetJobStatus(jobName)
    assert.True(t, exists)
    assert.Equal(t, int64(4), status.SuccessCount) // 4 successes, 1 failure
    assert.Equal(t, int64(1), status.FailureCount)
    assert.Equal(t, int64(0), status.ConsecutiveFailures) // Last execution succeeded
    
    // Check execution time statistics
    assert.Equal(t, 5*time.Millisecond, status.MinExecutionTime)
    assert.Equal(t, 25*time.Millisecond, status.MaxExecutionTime)
    
    // Average should be around 15ms (75ms total / 5 executions)
    expectedAvg := (10 + 20 + 15 + 25 + 5) * time.Millisecond / 5
    assert.True(t, 
        status.AverageExecution >= expectedAvg-2*time.Millisecond &&
        status.AverageExecution <= expectedAvg+2*time.Millisecond,
        "Average execution time not calculated correctly: %v (expected ~%v)", 
        status.AverageExecution, expectedAvg)
}
```

### 3. Stalled Job Detection Tests

#### TestJobStatusManager_StalledJobDetection
```go
func TestJobStatusManager_StalledJobDetection(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    // Create job status manager with short stalled threshold for testing
    jsm := &JobStatusManager{
        statuses:         make(map[string]*JobStatus),
        logger:           logger,
        metrics:          metrics,
        stalledThreshold: 100 * time.Millisecond, // Very short for testing
        cleanupInterval:  1 * time.Hour,
        retentionPeriod:  24 * time.Hour,
    }
    
    jobName := "stalled_job"
    jsm.StartJob(jobName)
    
    // Act - Wait longer than stalled threshold
    time.Sleep(150 * time.Millisecond)
    
    // Trigger stalled job detection
    jsm.detectStalledJobs()
    
    // Assert
    status, exists := jsm.GetJobStatus(jobName)
    assert.True(t, exists)
    assert.Equal(t, JobStatusStalled, status.Status)
    
    // Verify stalled metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_background_jobs_stalled" {
            metric := mf.GetMetric()[0]
            assert.Equal(t, float64(1), metric.GetGauge().GetValue())
        }
    }
}
```

#### TestJobStatusManager_StalledJobInGetAllStatuses
```go
func TestJobStatusManager_StalledJobInGetAllStatuses(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    
    jsm := &JobStatusManager{
        statuses:         make(map[string]*JobStatus),
        logger:           logger,
        metrics:          metrics,
        stalledThreshold: 50 * time.Millisecond,
        cleanupInterval:  1 * time.Hour,
        retentionPeriod:  24 * time.Hour,
    }
    
    jobName := "running_job"
    jsm.StartJob(jobName)
    
    // Act - Wait for job to become stalled
    time.Sleep(75 * time.Millisecond)
    
    // Get all statuses should detect stalled job
    allStatuses := jsm.GetAllJobStatuses()
    
    // Assert
    assert.Contains(t, allStatuses, jobName)
    assert.Equal(t, JobStatusStalled, allStatuses[jobName].Status)
}
```

### 4. Job Summary Tests

#### TestJobStatusManager_GetJobsSummary
```go
func TestJobStatusManager_GetJobsSummary(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    // Create various job statuses
    jobs := []struct {
        name              string
        status            JobExecutionStatus
        consecutiveFailures int64
    }{
        {"healthy_job_1", JobStatusSuccess, 0},
        {"healthy_job_2", JobStatusSuccess, 0},
        {"unhealthy_job_1", JobStatusFailed, 3},
        {"unhealthy_job_2", JobStatusSuccess, 2}, // Success but has recent failures
        {"running_job", JobStatusRunning, 0},
        {"stalled_job", JobStatusStalled, 1},
    }
    
    for _, job := range jobs {
        jsm.RegisterJob(job.name)
        status := jsm.statuses[job.name]
        status.Status = job.status
        status.ConsecutiveFailures = job.consecutiveFailures
    }
    
    // Act
    summary := jsm.GetJobsSummary()
    
    // Assert
    assert.Equal(t, 6, summary.TotalJobs)
    assert.Equal(t, 1, summary.RunningJobs)
    assert.Equal(t, 2, summary.HealthyJobs)    // 2 jobs with success and no consecutive failures
    assert.Equal(t, 2, summary.UnhealthyJobs) // 1 failed + 1 with consecutive failures
    assert.Equal(t, 1, summary.StalledJobs)
    assert.True(t, summary.LastUpdateTime.After(time.Time{}))
}
```

### 5. Concurrent Access Tests

#### TestJobStatusManager_ConcurrentAccess
```go
func TestJobStatusManager_ConcurrentAccess(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    const numGoroutines = 20
    const jobsPerGoroutine = 10
    
    var wg sync.WaitGroup
    
    // Act - Concurrent job operations
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(goroutineID int) {
            defer wg.Done()
            
            for j := 0; j < jobsPerGoroutine; j++ {
                jobName := fmt.Sprintf("job_%d_%d", goroutineID, j)
                
                // Start job
                jsm.StartJob(jobName)
                
                // Simulate work
                time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
                
                // Complete job (random success/failure)
                var err error
                if rand.Float32() < 0.1 { // 10% failure rate
                    err = errors.New("random failure")
                }
                
                jsm.CompleteJob(jobName, err, map[string]interface{}{
                    "goroutine_id": goroutineID,
                    "job_id":      j,
                })
            }
        }(i)
    }
    
    // Also read job statuses concurrently
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for k := 0; k < 20; k++ {
                allStatuses := jsm.GetAllJobStatuses()
                summary := jsm.GetJobsSummary()
                
                // Basic consistency check
                assert.True(t, len(allStatuses) >= 0)
                assert.True(t, summary.TotalJobs >= 0)
                
                time.Sleep(1 * time.Millisecond)
            }
        }()
    }
    
    wg.Wait()
    
    // Assert - Final state should be consistent
    allStatuses := jsm.GetAllJobStatuses()
    summary := jsm.GetJobsSummary()
    
    totalExpectedJobs := numGoroutines * jobsPerGoroutine
    assert.Equal(t, totalExpectedJobs, len(allStatuses))
    assert.Equal(t, totalExpectedJobs, summary.TotalJobs)
    
    // All jobs should be completed (success or failed)
    for _, status := range allStatuses {
        assert.True(t, 
            status.Status == JobStatusSuccess || status.Status == JobStatusFailed,
            "Job %s has unexpected status: %s", status.JobName, status.Status)
        assert.True(t, status.SuccessCount+status.FailureCount == 1)
    }
}
```

### 6. Cleanup Tests

#### TestJobStatusManager_PeriodicCleanup
```go
func TestJobStatusManager_PeriodicCleanup(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    
    jsm := &JobStatusManager{
        statuses:         make(map[string]*JobStatus),
        logger:           logger,
        metrics:          metrics,
        stalledThreshold: 5 * time.Minute,
        cleanupInterval:  1 * time.Hour,
        retentionPeriod:  100 * time.Millisecond, // Very short for testing
    }
    
    // Create old completed jobs
    oldJobs := []string{"old_job_1", "old_job_2", "old_job_3"}
    for _, jobName := range oldJobs {
        jsm.RegisterJob(jobName)
        status := jsm.statuses[jobName]
        status.Status = JobStatusSuccess
        status.UpdatedAt = time.Now().Add(-200 * time.Millisecond) // Older than retention period
    }
    
    // Create recent job
    jsm.RegisterJob("recent_job")
    recentStatus := jsm.statuses["recent_job"]
    recentStatus.Status = JobStatusSuccess
    recentStatus.UpdatedAt = time.Now() // Recent
    
    // Create still running job (should not be cleaned)
    jsm.RegisterJob("running_job")
    runningStatus := jsm.statuses["running_job"]
    runningStatus.Status = JobStatusRunning
    runningStatus.UpdatedAt = time.Now().Add(-200 * time.Millisecond) // Old but running
    
    // Verify initial state
    assert.Equal(t, 5, len(jsm.statuses))
    
    // Act
    jsm.cleanupOldStatuses()
    
    // Assert
    assert.Equal(t, 2, len(jsm.statuses)) // Only recent_job and running_job should remain
    
    _, recentExists := jsm.GetJobStatus("recent_job")
    assert.True(t, recentExists, "Recent job should not be cleaned up")
    
    _, runningExists := jsm.GetJobStatus("running_job")
    assert.True(t, runningExists, "Running job should not be cleaned up")
    
    for _, oldJob := range oldJobs {
        _, exists := jsm.GetJobStatus(oldJob)
        assert.False(t, exists, "Old job %s should be cleaned up", oldJob)
    }
}
```

### 7. Instrumented Job Tests

#### TestInstrumentedJob_SuccessfulExecution
```go
func TestInstrumentedJob_SuccessfulExecution(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    jsm := NewJobStatusManager(logger, metrics)
    
    executed := false
    jobFunc := func() error {
        time.Sleep(10 * time.Millisecond) // Simulate work
        executed = true
        return nil
    }
    
    instrumentedJob := NewInstrumentedJob(
        "test_successful_job",
        jobFunc,
        jsm,
        logger,
        5*time.Second, // Timeout
    )
    
    // Act
    start := time.Now()
    instrumentedJob.Execute()
    duration := time.Since(start)
    
    // Assert
    assert.True(t, executed, "Job function should have been executed")
    assert.True(t, duration >= 10*time.Millisecond, "Job should have taken at least 10ms")
    assert.True(t, duration < 1*time.Second, "Job should complete quickly")
    
    // Verify job status
    status, exists := jsm.GetJobStatus("test_successful_job")
    assert.True(t, exists)
    assert.Equal(t, JobStatusSuccess, status.Status)
    assert.Equal(t, int64(1), status.SuccessCount)
    assert.Equal(t, int64(0), status.FailureCount)
    assert.Empty(t, status.LastError)
}
```

#### TestInstrumentedJob_FailureExecution
```go
func TestInstrumentedJob_FailureExecution(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    expectedError := errors.New("job processing failed")
    jobFunc := func() error {
        time.Sleep(5 * time.Millisecond)
        return expectedError
    }
    
    instrumentedJob := NewInstrumentedJob(
        "test_failing_job",
        jobFunc,
        jsm,
        logger,
        5*time.Second,
    )
    
    // Act
    instrumentedJob.Execute()
    
    // Assert
    status, exists := jsm.GetJobStatus("test_failing_job")
    assert.True(t, exists)
    assert.Equal(t, JobStatusFailed, status.Status)
    assert.Equal(t, int64(0), status.SuccessCount)
    assert.Equal(t, int64(1), status.FailureCount)
    assert.Equal(t, expectedError.Error(), status.LastError)
    assert.Contains(t, status.Metadata, "error_type")
}
```

#### TestInstrumentedJob_TimeoutExecution
```go
func TestInstrumentedJob_TimeoutExecution(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    jobFunc := func() error {
        time.Sleep(200 * time.Millisecond) // Longer than timeout
        return nil
    }
    
    instrumentedJob := NewInstrumentedJob(
        "test_timeout_job",
        jobFunc,
        jsm,
        logger,
        100*time.Millisecond, // Short timeout
    )
    
    // Act
    start := time.Now()
    instrumentedJob.Execute()
    duration := time.Since(start)
    
    // Assert
    assert.True(t, duration >= 100*time.Millisecond, "Should wait for timeout")
    assert.True(t, duration < 150*time.Millisecond, "Should not wait much longer than timeout")
    
    status, exists := jsm.GetJobStatus("test_timeout_job")
    assert.True(t, exists)
    assert.Equal(t, JobStatusFailed, status.Status)
    assert.Contains(t, status.LastError, "timeout")
    assert.Equal(t, "timeout", status.Metadata["error_type"])
    assert.Equal(t, "100ms", status.Metadata["timeout"])
}
```

#### TestInstrumentedJob_PanicRecovery
```go
func TestInstrumentedJob_PanicRecovery(t *testing.T) {
    // Arrange
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    jobFunc := func() error {
        panic("unexpected panic in job")
    }
    
    instrumentedJob := NewInstrumentedJob(
        "test_panic_job",
        jobFunc,
        jsm,
        logger,
        5*time.Second,
    )
    
    // Act - Should not panic the test
    assert.NotPanics(t, func() {
        instrumentedJob.Execute()
    })
    
    // Assert
    status, exists := jsm.GetJobStatus("test_panic_job")
    assert.True(t, exists)
    assert.Equal(t, JobStatusFailed, status.Status)
    assert.Contains(t, status.LastError, "panicked")
    assert.Contains(t, status.Metadata, "panic")
    assert.Contains(t, status.Metadata, "stack_trace")
}
```

### 8. Error Classification Tests

#### TestClassifyJobError
```go
func TestClassifyJobError(t *testing.T) {
    tests := []struct {
        name         string
        error        error
        expectedType string
    }{
        {
            name:         "Timeout error",
            error:        errors.New("operation timeout after 30s"),
            expectedType: "timeout",
        },
        {
            name:         "Deadline exceeded",
            error:        errors.New("context deadline exceeded"),
            expectedType: "timeout",
        },
        {
            name:         "Network error",
            error:        errors.New("network connection failed"),
            expectedType: "network",
        },
        {
            name:         "Database error",
            error:        errors.New("database query failed"),
            expectedType: "database",
        },
        {
            name:         "SQL error",
            error:        errors.New("sql: connection refused"),
            expectedType: "database",
        },
        {
            name:         "External API error",
            error:        errors.New("external API call failed"),
            expectedType: "external_api",
        },
        {
            name:         "Panic error",
            error:        errors.New("job panicked: runtime error"),
            expectedType: "panic",
        },
        {
            name:         "Unknown error",
            error:        errors.New("some unknown error"),
            expectedType: "unknown",
        },
        {
            name:         "Nil error",
            error:        nil,
            expectedType: "",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := classifyJobError(tt.error)
            assert.Equal(t, tt.expectedType, result)
        })
    }
}
```

### 9. Instrumented Telemetry Tests

#### TestInstrumentedTelemetry_IndexBtcTransaction
```go
func TestInstrumentedTelemetry_IndexBtcTransaction(t *testing.T) {
    // Arrange
    mockTelemetry := setupMockTelemetry(t, nil) // Success
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    registry := prometheus.NewRegistry()
    metrics.MustRegister(registry)
    
    jsm := NewJobStatusManager(logger, metrics)
    
    instrumentedTelemetry := NewInstrumentedTelemetry(
        mockTelemetry,
        jsm,
        metrics,
        logger,
    )
    
    // Act
    err := instrumentedTelemetry.IndexBtcTransaction()
    
    // Assert
    assert.NoError(t, err, "Instrumented telemetry should always return nil")
    
    // Verify job was tracked
    status, exists := jsm.GetJobStatus("btc_transaction_indexing")
    assert.True(t, exists)
    assert.Equal(t, JobStatusSuccess, status.Status)
    
    // Verify metrics
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_background_job_runs_total" {
            found := false
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                if getLabelValue(labels, "job_name") == "btc_transaction_indexing" &&
                   getLabelValue(labels, "status") == "success" {
                    found = true
                    assert.Equal(t, float64(1), metric.GetCounter().GetValue())
                }
            }
            assert.True(t, found, "BTC indexing success metric not found")
        }
    }
}
```

#### TestInstrumentedTelemetry_ProcessSwapRequestsFailure
```go
func TestInstrumentedTelemetry_ProcessSwapRequestsFailure(t *testing.T) {
    // Arrange
    expectedError := errors.New("swap processing database error")
    mockTelemetry := setupMockTelemetry(t, expectedError)
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    instrumentedTelemetry := NewInstrumentedTelemetry(
        mockTelemetry,
        jsm,
        metrics,
        logger,
    )
    
    // Act
    err := instrumentedTelemetry.ProcessSwapRequests()
    
    // Assert
    assert.NoError(t, err, "Instrumented telemetry should always return nil")
    
    // Verify job failure was tracked
    status, exists := jsm.GetJobStatus("swap_request_processing")
    assert.True(t, exists)
    assert.Equal(t, JobStatusFailed, status.Status)
    assert.Contains(t, status.LastError, "swap processing database error")
}
```

### 10. Background Job Metrics Tests

#### TestBackgroundJobMetrics_Registration
```go
func TestBackgroundJobMetrics_Registration(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewBackgroundJobMetrics()
    
    // Act
    metrics.MustRegister(registry)
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    expectedMetrics := []string{
        "icy_backend_background_job_duration_seconds",
        "icy_backend_background_job_runs_total",
        "icy_backend_background_jobs_active",
        "icy_backend_background_jobs_stalled",
        "icy_backend_pending_transactions_total",
        "icy_backend_job_execution_history_total",
        "icy_backend_job_timeouts_total",
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

#### TestBackgroundJobMetrics_PendingTransactions
```go
func TestBackgroundJobMetrics_PendingTransactions(t *testing.T) {
    // Arrange
    registry := prometheus.NewRegistry()
    metrics := NewBackgroundJobMetrics()
    metrics.MustRegister(registry)
    
    // Act - Record pending transactions
    metrics.pendingTransactions.WithLabelValues("btc").Set(15)
    metrics.pendingTransactions.WithLabelValues("icy").Set(8)
    metrics.pendingTransactions.WithLabelValues("swap").Set(3)
    
    // Assert
    metricFamilies, err := registry.Gather()
    assert.NoError(t, err)
    
    for _, mf := range metricFamilies {
        if mf.GetName() == "icy_backend_pending_transactions_total" {
            assert.Equal(t, 3, len(mf.GetMetric())) // Three transaction types
            
            transactionCounts := make(map[string]float64)
            for _, metric := range mf.GetMetric() {
                labels := metric.GetLabel()
                txType := getLabelValue(labels, "transaction_type")
                transactionCounts[txType] = metric.GetGauge().GetValue()
            }
            
            assert.Equal(t, float64(15), transactionCounts["btc"])
            assert.Equal(t, float64(8), transactionCounts["icy"])
            assert.Equal(t, float64(3), transactionCounts["swap"])
        }
    }
}
```

## Performance Benchmarks

```go
func BenchmarkJobStatusManager_StartCompleteJob(b *testing.B) {
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        jobName := fmt.Sprintf("bench_job_%d", i)
        jsm.StartJob(jobName)
        jsm.CompleteJob(jobName, nil, nil)
    }
}

func BenchmarkInstrumentedJob_Execute(b *testing.B) {
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    jobFunc := func() error {
        // Simulate minimal work
        time.Sleep(1 * time.Microsecond)
        return nil
    }
    
    instrumentedJob := NewInstrumentedJob(
        "bench_job",
        jobFunc,
        jsm,
        logger,
        5*time.Second,
    )
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        instrumentedJob.Execute()
    }
}

func BenchmarkJobStatusManager_GetAllJobStatuses(b *testing.B) {
    logger := setupTestLogger()
    metrics := NewBackgroundJobMetrics()
    jsm := NewJobStatusManager(logger, metrics)
    
    // Setup many jobs
    for i := 0; i < 100; i++ {
        jobName := fmt.Sprintf("job_%d", i)
        jsm.RegisterJob(jobName)
        status := jsm.statuses[jobName]
        status.Status = JobStatusSuccess
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        jsm.GetAllJobStatuses()
    }
}
```

## Test Helper Functions

```go
func setupTestLogger() *logger.Logger {
    // Implementation for test logger
}

func setupMockTelemetry(t *testing.T, returnError error) telemetry.ITelemetry {
    // Implementation for mock telemetry
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

- **Function Coverage**: 100% of all job monitoring functions
- **Branch Coverage**: >90% including error paths and edge cases
- **Concurrency Coverage**: Thread safety validation under load
- **Performance Coverage**: Overhead measurement benchmarks
- **Integration Coverage**: Telemetry and metrics integration validation