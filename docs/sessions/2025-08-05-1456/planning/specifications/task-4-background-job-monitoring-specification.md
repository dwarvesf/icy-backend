# Task 4: Background Job Monitoring Implementation Specification

**Date**: 2025-08-05  
**Task**: Background Job Monitoring (Instrumentation Monitoring)  
**Priority**: High  
**Estimated Effort**: 4-5 days  

## Overview

Implement comprehensive monitoring for background jobs executed via robfig/cron/v3, including job status tracking, performance metrics, stalled job detection, and job health reporting. This provides instrumentation monitoring for critical background processes like transaction indexing and swap processing.

## Functional Requirements

### 1. Job Status Tracking

**Purpose**: Track execution status, duration, and success/failure rates for all background jobs  
**Implementation**: Thread-safe in-memory status manager with optional persistence  
**Data Retention**: Keep job status for last 24 hours  

**Job Status Data**:
- Job execution status (pending, running, success, failed, stalled)
- Execution timing (start time, duration, next run)
- Success/failure counters
- Error messages and metadata

### 2. Job Health Endpoint

**Purpose**: Provide HTTP endpoint for job status monitoring  
**Endpoint**: `GET /api/v1/health/jobs`  
**Authentication**: None required (for monitoring systems)  
**Response**: Comprehensive job health and status information  

### 3. Background Job Metrics

**Purpose**: Collect Prometheus metrics for job performance analysis  
**Integration**: Works with Task 3 application metrics  
**Focus Areas**: Job duration, success rates, pending transaction counts  

### 4. Stalled Job Detection

**Purpose**: Automatically detect and report jobs that hang or fail to complete  
**Implementation**: Periodic checking with configurable thresholds  
**Action**: Update status to "stalled" and alert via logs/metrics  

## Technical Specification

### 1. Job Status Data Structures

```go
package monitoring

import (
    "sync"
    "time"
)

type JobExecutionStatus string

const (
    JobStatusPending JobExecutionStatus = "pending"
    JobStatusRunning JobExecutionStatus = "running"
    JobStatusSuccess JobExecutionStatus = "success"
    JobStatusFailed  JobExecutionStatus = "failed"
    JobStatusStalled JobExecutionStatus = "stalled"
)

type JobStatus struct {
    JobName          string                 `json:"job_name"`
    Status           JobExecutionStatus     `json:"status"`
    LastRunTime      time.Time              `json:"last_run_time"`
    LastDuration     time.Duration          `json:"last_duration_ms"`
    NextRunTime      time.Time              `json:"next_run_time,omitempty"`
    SuccessCount     int64                  `json:"success_count"`
    FailureCount     int64                  `json:"failure_count"`
    ConsecutiveFailures int64              `json:"consecutive_failures"`
    LastError        string                 `json:"last_error,omitempty"`
    AverageExecution time.Duration          `json:"average_execution_ms"`
    MaxExecutionTime time.Duration          `json:"max_execution_ms"`
    MinExecutionTime time.Duration          `json:"min_execution_ms"`
    Metadata         map[string]interface{} `json:"metadata,omitempty"`
    CreatedAt        time.Time              `json:"created_at"`
    UpdatedAt        time.Time              `json:"updated_at"`
}

type JobsSummary struct {
    TotalJobs       int `json:"total_jobs"`
    RunningJobs     int `json:"running_jobs"`
    HealthyJobs     int `json:"healthy_jobs"`
    UnhealthyJobs   int `json:"unhealthy_jobs"`
    StalledJobs     int `json:"stalled_jobs"`
    LastUpdateTime  time.Time `json:"last_update_time"`
}
```

### 2. Job Status Manager

```go
package monitoring

import (
    "sync"
    "time"
    
    "github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type JobStatusManager struct {
    mu                sync.RWMutex
    statuses          map[string]*JobStatus
    logger            *logger.Logger
    metrics           *BackgroundJobMetrics
    stalledThreshold  time.Duration
    cleanupInterval   time.Duration
    retentionPeriod   time.Duration
}

func NewJobStatusManager(logger *logger.Logger, metrics *BackgroundJobMetrics) *JobStatusManager {
    jsm := &JobStatusManager{
        statuses:         make(map[string]*JobStatus),
        logger:           logger,
        metrics:          metrics,
        stalledThreshold: 5 * time.Minute,
        cleanupInterval:  1 * time.Hour,
        retentionPeriod:  24 * time.Hour,
    }
    
    // Start background processes
    go jsm.startStalledJobDetection()
    go jsm.startPeriodicCleanup()
    
    return jsm
}

func (jsm *JobStatusManager) RegisterJob(jobName string) {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    if _, exists := jsm.statuses[jobName]; !exists {
        jsm.statuses[jobName] = &JobStatus{
            JobName:         jobName,
            Status:          JobStatusPending,
            Metadata:        make(map[string]interface{}),
            CreatedAt:       time.Now(),
            UpdatedAt:       time.Now(),
            MinExecutionTime: time.Duration(math.MaxInt64),
        }
        
        jsm.logger.Info("Job registered for monitoring", map[string]string{
            "job_name": jobName,
        })
    }
}

func (jsm *JobStatusManager) StartJob(jobName string) {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    status, exists := jsm.statuses[jobName]
    if !exists {
        jsm.RegisterJob(jobName)
        status = jsm.statuses[jobName]
    }
    
    status.Status = JobStatusRunning
    status.LastRunTime = time.Now()
    status.UpdatedAt = time.Now()
    
    // Update metrics
    jsm.metrics.activeJobs.Inc()
    
    jsm.logger.Info("Job started", map[string]string{
        "job_name":   jobName,
        "start_time": status.LastRunTime.Format(time.RFC3339),
    })
}

func (jsm *JobStatusManager) CompleteJob(jobName string, err error, metadata map[string]interface{}) {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    status, exists := jsm.statuses[jobName]
    if !exists {
        jsm.logger.Error("Attempted to complete unregistered job", map[string]string{
            "job_name": jobName,
        })
        return
    }
    
    // Calculate execution duration
    duration := time.Since(status.LastRunTime)
    status.LastDuration = duration
    status.UpdatedAt = time.Now()
    
    // Update execution time statistics
    if duration < status.MinExecutionTime || status.MinExecutionTime == time.Duration(math.MaxInt64) {
        status.MinExecutionTime = duration
    }
    if duration > status.MaxExecutionTime {
        status.MaxExecutionTime = duration
    }
    
    // Calculate average execution time
    totalRuns := status.SuccessCount + status.FailureCount
    if totalRuns > 0 {
        totalTime := status.AverageExecution * time.Duration(totalRuns)
        totalTime += duration
        status.AverageExecution = totalTime / time.Duration(totalRuns+1)
    } else {
        status.AverageExecution = duration
    }
    
    // Update metadata
    if metadata != nil {
        for key, value := range metadata {
            status.Metadata[key] = value
        }
    }
    
    // Update status and counters
    if err != nil {
        status.Status = JobStatusFailed
        status.FailureCount++
        status.ConsecutiveFailures++
        status.LastError = err.Error()
        
        // Record metrics
        jsm.metrics.jobRuns.WithLabelValues(jobName, "error").Inc()
        jsm.metrics.jobDuration.WithLabelValues(jobName, "failed").Observe(duration.Seconds())
        
        jsm.logger.Error("Job failed", map[string]string{
            "job_name":            jobName,
            "duration":            duration.String(),
            "error":               err.Error(),
            "consecutive_failures": fmt.Sprintf("%d", status.ConsecutiveFailures),
        })
    } else {
        status.Status = JobStatusSuccess
        status.SuccessCount++
        status.ConsecutiveFailures = 0 // Reset consecutive failures
        status.LastError = ""
        
        // Record metrics
        jsm.metrics.jobRuns.WithLabelValues(jobName, "success").Inc()
        jsm.metrics.jobDuration.WithLabelValues(jobName, "success").Observe(duration.Seconds())
        
        jsm.logger.Info("Job completed successfully", map[string]string{
            "job_name": jobName,
            "duration": duration.String(),
        })
    }
    
    // Update metrics
    jsm.metrics.activeJobs.Dec()
}

func (jsm *JobStatusManager) GetJobStatus(jobName string) (*JobStatus, bool) {
    jsm.mu.RLock()
    defer jsm.mu.RUnlock()
    
    if status, exists := jsm.statuses[jobName]; exists {
        // Create a copy to avoid race conditions
        statusCopy := *status
        statusCopy.Metadata = make(map[string]interface{})
        for k, v := range status.Metadata {
            statusCopy.Metadata[k] = v
        }
        return &statusCopy, true
    }
    
    return nil, false
}

func (jsm *JobStatusManager) GetAllJobStatuses() map[string]JobStatus {
    jsm.mu.RLock()
    defer jsm.mu.RUnlock()
    
    result := make(map[string]JobStatus)
    currentTime := time.Now()
    
    for name, status := range jsm.statuses {
        statusCopy := *status
        statusCopy.Metadata = make(map[string]interface{})
        for k, v := range status.Metadata {
            statusCopy.Metadata[k] = v
        }
        
        // Check for stalled jobs
        if status.Status == JobStatusRunning && 
           currentTime.Sub(status.LastRunTime) > jsm.stalledThreshold {
            statusCopy.Status = JobStatusStalled
        }
        
        result[name] = statusCopy
    }
    
    return result
}

func (jsm *JobStatusManager) GetJobsSummary() JobsSummary {
    statuses := jsm.GetAllJobStatuses()
    
    summary := JobsSummary{
        TotalJobs:      len(statuses),
        LastUpdateTime: time.Now(),
    }
    
    for _, status := range statuses {
        switch status.Status {
        case JobStatusRunning:
            summary.RunningJobs++
        case JobStatusSuccess:
            if status.ConsecutiveFailures == 0 {
                summary.HealthyJobs++
            } else {
                summary.UnhealthyJobs++
            }
        case JobStatusFailed:
            summary.UnhealthyJobs++
        case JobStatusStalled:
            summary.StalledJobs++
        }
    }
    
    return summary
}

func (jsm *JobStatusManager) startStalledJobDetection() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        jsm.detectStalledJobs()
    }
}

func (jsm *JobStatusManager) detectStalledJobs() {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    currentTime := time.Now()
    stalledCount := 0
    
    for jobName, status := range jsm.statuses {
        if status.Status == JobStatusRunning &&
           currentTime.Sub(status.LastRunTime) > jsm.stalledThreshold {
            
            status.Status = JobStatusStalled
            status.UpdatedAt = currentTime
            stalledCount++
            
            jsm.logger.Error("Job detected as stalled", map[string]string{
                "job_name":      jobName,
                "last_run_time": status.LastRunTime.Format(time.RFC3339),
                "duration":      currentTime.Sub(status.LastRunTime).String(),
            })
        }
    }
    
    jsm.metrics.stalledJobs.Set(float64(stalledCount))
}

func (jsm *JobStatusManager) startPeriodicCleanup() {
    ticker := time.NewTicker(jsm.cleanupInterval)
    defer ticker.Stop()
    
    for range ticker.C {
        jsm.cleanupOldStatuses()
    }
}

func (jsm *JobStatusManager) cleanupOldStatuses() {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    cutoff := time.Now().Add(-jsm.retentionPeriod)
    cleaned := 0
    
    for jobName, status := range jsm.statuses {
        if status.UpdatedAt.Before(cutoff) && 
           status.Status != JobStatusRunning {
            
            delete(jsm.statuses, jobName)
            cleaned++
        }
    }
    
    if cleaned > 0 {
        jsm.logger.Info("Cleaned up old job statuses", map[string]string{
            "cleaned_count": fmt.Sprintf("%d", cleaned),
        })
    }
}
```

### 3. Instrumented Job Wrapper

```go
package monitoring

import (
    "context"
    "fmt"
    "runtime/debug"
    "time"
    
    "github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type InstrumentedJob struct {
    jobName       string
    jobFunc       func() error
    statusManager *JobStatusManager
    logger        *logger.Logger
    timeout       time.Duration
}

func NewInstrumentedJob(
    jobName string,
    jobFunc func() error,
    statusManager *JobStatusManager,
    logger *logger.Logger,
    timeout time.Duration,
) *InstrumentedJob {
    
    // Register job for monitoring
    statusManager.RegisterJob(jobName)
    
    return &InstrumentedJob{
        jobName:       jobName,
        jobFunc:       jobFunc,
        statusManager: statusManager,
        logger:        logger,
        timeout:       timeout,
    }
}

func (ij *InstrumentedJob) Execute() {
    // Start job tracking
    ij.statusManager.StartJob(ij.jobName)
    
    // Setup timeout context
    ctx, cancel := context.WithTimeout(context.Background(), ij.timeout)
    defer cancel()
    
    // Execute with panic recovery
    var err error
    var metadata map[string]interface{}
    
    func() {
        defer func() {
            if r := recover(); r != nil {
                err = fmt.Errorf("job panicked: %v", r)
                metadata = map[string]interface{}{
                    "panic":      fmt.Sprintf("%v", r),
                    "stack_trace": string(debug.Stack()),
                }
                
                ij.logger.Error("Job panicked", map[string]string{
                    "job_name": ij.jobName,
                    "panic":     fmt.Sprintf("%v", r),
                })
            }
        }()
        
        // Execute job with timeout
        done := make(chan error, 1)
        go func() {
            done <- ij.jobFunc()
        }()
        
        select {
        case err = <-done:
            // Job completed
            if err != nil {
                metadata = map[string]interface{}{
                    "error_type": classifyJobError(err),
                }
            }
        case <-ctx.Done():
            err = fmt.Errorf("job timeout after %v", ij.timeout)
            metadata = map[string]interface{}{
                "error_type": "timeout",
                "timeout":    ij.timeout.String(),
            }
        }
    }()
    
    // Complete job tracking
    ij.statusManager.CompleteJob(ij.jobName, err, metadata)
}

func classifyJobError(err error) string {
    if err == nil {
        return ""
    }
    
    errStr := strings.ToLower(err.Error())
    switch {
    case strings.Contains(errStr, "timeout"), strings.Contains(errStr, "deadline"):
        return "timeout"
    case strings.Contains(errStr, "connection"), strings.Contains(errStr, "network"):
        return "network"
    case strings.Contains(errStr, "database"), strings.Contains(errStr, "sql"):
        return "database"
    case strings.Contains(errStr, "external"), strings.Contains(errStr, "api"):
        return "external_api"
    case strings.Contains(errStr, "panic"):
        return "panic"
    default:
        return "unknown"
    }
}
```

### 4. Background Job Metrics

```go
package monitoring

type BackgroundJobMetrics struct {
    jobDuration            *prometheus.HistogramVec
    jobRuns                *prometheus.CounterVec
    activeJobs             prometheus.Gauge
    stalledJobs            prometheus.Gauge
    pendingTransactions    *prometheus.GaugeVec
    jobExecutionHistory    *prometheus.CounterVec
    jobTimeouts            *prometheus.CounterVec
}

func NewBackgroundJobMetrics() *BackgroundJobMetrics {
    return &BackgroundJobMetrics{
        jobDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_background_job_duration_seconds",
                Help: "Background job execution duration in seconds",
                Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800}, // 1s to 30min
            },
            []string{"job_name", "status"},
        ),
        jobRuns: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_background_job_runs_total",
                Help: "Total number of background job runs",
            },
            []string{"job_name", "status"},
        ),
        activeJobs: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_background_jobs_active",
                Help: "Number of currently running background jobs",
            },
        ),
        stalledJobs: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_background_jobs_stalled",
                Help: "Number of stalled background jobs",
            },
        ),
        pendingTransactions: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_pending_transactions_total",
                Help: "Number of pending transactions by type",
            },
            []string{"transaction_type"}, // "btc", "icy", "swap"
        ),
        jobExecutionHistory: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_job_execution_history_total",
                Help: "Historical job execution counts",
            },
            []string{"job_name", "date"}, // date in YYYY-MM-DD format
        ),
        jobTimeouts: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_job_timeouts_total",
                Help: "Total job timeouts",
            },
            []string{"job_name"},
        ),
    }
}

func (m *BackgroundJobMetrics) MustRegister(registry *prometheus.Registry) {
    registry.MustRegister(
        m.jobDuration,
        m.jobRuns,
        m.activeJobs,
        m.stalledJobs,
        m.pendingTransactions,
        m.jobExecutionHistory,
        m.jobTimeouts,
    )
}
```

### 5. Job Health Handler

```go
package health

import (
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    
    "github.com/dwarvesf/icy-backend/internal/monitoring"
)

type JobsHealthResponse struct {
    Status      string                        `json:"status"`
    Timestamp   time.Time                     `json:"timestamp"`
    Jobs        map[string]monitoring.JobStatus `json:"jobs"`
    Summary     monitoring.JobsSummary        `json:"summary"`
    Duration    int64                         `json:"duration_ms"`
}

func (h *HealthHandler) Jobs(c *gin.Context) {
    start := time.Now()
    
    // Get job statuses
    jobs := h.jobStatusManager.GetAllJobStatuses()
    summary := h.jobStatusManager.GetJobsSummary()
    
    // Determine overall status
    overallStatus := "healthy"
    if summary.StalledJobs > 0 {
        overallStatus = "unhealthy"
    } else if summary.UnhealthyJobs > 0 {
        // Check if unhealthy jobs are critical
        criticalJobsUnhealthy := false
        criticalJobs := []string{
            "btc_transaction_indexing",
            "icy_transaction_indexing", 
            "swap_request_processing",
        }
        
        for _, criticalJob := range criticalJobs {
            if jobStatus, exists := jobs[criticalJob]; exists {
                if jobStatus.Status == monitoring.JobStatusFailed &&
                   jobStatus.ConsecutiveFailures > 2 {
                    criticalJobsUnhealthy = true
                    break
                }
            }
        }
        
        if criticalJobsUnhealthy {
            overallStatus = "unhealthy"
        } else {
            overallStatus = "degraded"
        }
    }
    
    response := JobsHealthResponse{
        Status:    overallStatus,
        Timestamp: time.Now(),
        Jobs:      jobs,
        Summary:   summary,
        Duration:  time.Since(start).Milliseconds(),
    }
    
    statusCode := http.StatusOK
    if overallStatus == "unhealthy" {
        statusCode = http.StatusServiceUnavailable
    } else if overallStatus == "degraded" {
        statusCode = http.StatusPartialContent // 206
    }
    
    // Log health check
    h.logger.Info("Jobs health check completed", map[string]string{
        "overall_status":  overallStatus,
        "duration":        fmt.Sprintf("%dms", response.Duration),
        "total_jobs":      fmt.Sprintf("%d", summary.TotalJobs),
        "unhealthy_jobs":  fmt.Sprintf("%d", summary.UnhealthyJobs),
        "stalled_jobs":    fmt.Sprintf("%d", summary.StalledJobs),
        "running_jobs":    fmt.Sprintf("%d", summary.RunningJobs),
    })
    
    c.JSON(statusCode, response)
}
```

### 6. Enhanced Telemetry Integration

```go
package telemetry

type InstrumentedTelemetry struct {
    telemetry.ITelemetry
    statusManager *monitoring.JobStatusManager
    metrics       *monitoring.BackgroundJobMetrics
    logger        *logger.Logger
}

func NewInstrumentedTelemetry(
    baseTelemetry telemetry.ITelemetry,
    statusManager *monitoring.JobStatusManager,
    metrics *monitoring.BackgroundJobMetrics,
    logger *logger.Logger,
) *InstrumentedTelemetry {
    return &InstrumentedTelemetry{
        ITelemetry:    baseTelemetry,
        statusManager: statusManager,
        metrics:       metrics,
        logger:        logger,
    }
}

func (it *InstrumentedTelemetry) IndexBtcTransaction() error {
    job := monitoring.NewInstrumentedJob(
        "btc_transaction_indexing",
        it.ITelemetry.IndexBtcTransaction,
        it.statusManager,
        it.logger,
        10*time.Minute, // 10 minute timeout
    )
    
    job.Execute()
    
    // Update pending transaction count
    if pendingCount := it.getBtcPendingCount(); pendingCount >= 0 {
        it.metrics.pendingTransactions.WithLabelValues("btc").Set(float64(pendingCount))
    }
    
    return nil // Always return nil since error handling is done in the job
}

func (it *InstrumentedTelemetry) IndexIcyTransaction() error {
    job := monitoring.NewInstrumentedJob(
        "icy_transaction_indexing",
        it.ITelemetry.IndexIcyTransaction,
        it.statusManager,
        it.logger,
        10*time.Minute,
    )
    
    job.Execute()
    
    if pendingCount := it.getIcyPendingCount(); pendingCount >= 0 {
        it.metrics.pendingTransactions.WithLabelValues("icy").Set(float64(pendingCount))
    }
    
    return nil
}

func (it *InstrumentedTelemetry) ProcessSwapRequests() error {
    job := monitoring.NewInstrumentedJob(
        "swap_request_processing",
        it.ITelemetry.ProcessSwapRequests,
        it.statusManager,
        it.logger,
        15*time.Minute,
    )
    
    job.Execute()
    
    if pendingCount := it.getSwapPendingCount(); pendingCount >= 0 {
        it.metrics.pendingTransactions.WithLabelValues("swap").Set(float64(pendingCount))
    }
    
    return nil
}

// Helper methods to get pending transaction counts
func (it *InstrumentedTelemetry) getBtcPendingCount() int64 {
    // Implementation would depend on store interface
    // This is a placeholder showing the pattern
    return -1 // Return -1 if count unavailable
}

func (it *InstrumentedTelemetry) getIcyPendingCount() int64 {
    return -1
}

func (it *InstrumentedTelemetry) getSwapPendingCount() int64 {
    return -1
}
```

## Integration Requirements

### 1. Server Initialization

```go
// In internal/server/server.go
func Init() {
    // ... existing initialization
    
    // Create job monitoring components
    backgroundJobMetrics := monitoring.NewBackgroundJobMetrics()
    jobStatusManager := monitoring.NewJobStatusManager(logger, backgroundJobMetrics)
    
    // Create instrumented telemetry
    baseTelemetry := telemetry.New(
        db,
        s,
        appConfig,
        logger,
        btcRpcWithCB,
        baseRpcWithCB,
        oracle,
    )
    
    instrumentedTelemetry := telemetry.NewInstrumentedTelemetry(
        baseTelemetry,
        jobStatusManager,
        backgroundJobMetrics,
        logger,
    )
    
    // Setup cron with job monitoring
    c := cron.New(cron.WithLogger(cron.VerbosePrintfLogger(logger)))
    
    // Add cron jobs with instrumentation
    indexInterval := "2m"
    if appConfig.IndexInterval != "" {
        indexInterval = appConfig.IndexInterval
    }
    
    c.AddFunc("@every "+indexInterval, func() {
        go instrumentedTelemetry.IndexBtcTransaction()
        go instrumentedTelemetry.IndexIcyTransaction()
        go instrumentedTelemetry.IndexIcySwapTransaction()
        instrumentedTelemetry.ProcessSwapRequests()
        instrumentedTelemetry.ProcessPendingBtcTransactions()
    })
    
    c.Start()
    
    // Pass job status manager to HTTP server
    httpServer := http.NewHttpServer(
        appConfig,
        logger,
        oracle,
        baseRpcWithCB,
        btcRpcWithCB,
        db,
        metricsRegistry,
        jobStatusManager, // Add job status manager
    )
    httpServer.Run()
}
```

### 2. Health Handler Extension

```go
// In internal/handler/health/health.go
type HealthHandler struct {
    config           *config.AppConfig
    logger           *logger.Logger
    db               *gorm.DB
    btcRPC           btcrpc.IBtcRpc
    baseRPC          baserpc.IBaseRPC
    jobStatusManager *monitoring.JobStatusManager // Add job status manager
}

func New(
    config *config.AppConfig,
    logger *logger.Logger,
    db *gorm.DB,
    btcRPC btcrpc.IBtcRpc,
    baseRPC baserpc.IBaseRPC,
    jobStatusManager *monitoring.JobStatusManager,
) IHealthHandler {
    return &HealthHandler{
        config:           config,
        logger:           logger,
        db:               db,
        btcRPC:           btcRPC,
        baseRPC:          baseRPC,
        jobStatusManager: jobStatusManager,
    }
}
```

### 3. Route Registration

```go
// In internal/transport/http/v1.go
func loadV1Routes(r *gin.Engine, h *handler.Handler) {
    // ... existing routes
    
    // Health endpoints
    health := v1.Group("/health")
    {
        health.GET("/db", h.HealthHandler.Database)
        health.GET("/external", h.HealthHandler.External)
        health.GET("/jobs", h.HealthHandler.Jobs) // Add jobs health endpoint
    }
    
    // ... rest of routes
}
```

## Testing Requirements

### 1. Unit Tests

**File**: `internal/monitoring/job_status_manager_test.go`
```go
func TestJobStatusManager_RegisterJob(t *testing.T) {
    // Test job registration
}

func TestJobStatusManager_JobExecution(t *testing.T) {
    // Test complete job execution cycle
}

func TestJobStatusManager_StalledJobDetection(t *testing.T) {
    // Test stalled job detection
}

func TestJobStatusManager_ThreadSafety(t *testing.T) {
    // Test concurrent access safety
}

func TestInstrumentedJob_ExecutionWithTimeout(t *testing.T) {
    // Test job execution with timeout
}

func TestInstrumentedJob_PanicRecovery(t *testing.T) {
    // Test panic recovery
}
```

### 2. Integration Tests

**File**: `internal/monitoring/job_monitoring_integration_test.go`

Test job monitoring with actual cron jobs and telemetry operations.

### 3. Performance Tests

- Memory usage of job status tracking
- Overhead of job instrumentation
- Concurrent job execution handling

## Performance Requirements

- Job instrumentation overhead: < 10ms per job execution
- Memory usage for job status: < 5MB for 100 jobs over 24 hours
- Stalled job detection latency: < 1 minute
- Jobs health endpoint response time: < 100ms

## Documentation Requirements

### 1. Operational Documentation

- Job monitoring dashboard setup
- Alert configuration for stalled jobs
- Troubleshooting guide for job failures
- Performance tuning recommendations

### 2. API Documentation

- Jobs health endpoint documentation
- Job status interpretation guide
- Metrics usage examples

## Acceptance Criteria

- [ ] Job status manager tracks all background job executions
- [ ] Thread-safe job status updates with no race conditions
- [ ] Stalled job detection works within 1 minute of threshold
- [ ] Jobs health endpoint returns comprehensive status information
- [ ] Job metrics properly integrated with Prometheus
- [ ] Job instrumentation has minimal performance overhead
- [ ] Panic recovery prevents job crashes from affecting system
- [ ] Job timeout handling works correctly
- [ ] Historical job data cleanup prevents memory leaks
- [ ] Unit test coverage > 90%
- [ ] Integration tests validate job monitoring accuracy
- [ ] Performance requirements met under concurrent job execution