# ADR-004: Background Job Monitoring and Status Tracking

**Date**: 2025-08-05  
**Status**: Proposed  
**Deciders**: Project Team  
**Context**: Phase 1 Monitoring Implementation  

## Context

The ICY Backend uses robfig/cron/v3 for background job scheduling, running critical tasks like indexing Bitcoin transactions, ICY transactions, and processing swap requests every 2 minutes. These background jobs are essential for system functionality and require comprehensive monitoring, status tracking, and performance metrics to ensure reliable operation.

## Decision

### 1. Job Status Tracking Architecture

**Decision**: Implement in-memory thread-safe job status tracking with optional persistence

**Status Data Structure**:
```go
type JobStatus struct {
    JobName       string                 `json:"job_name"`
    LastRunTime   time.Time             `json:"last_run_time"`
    LastDuration  time.Duration         `json:"last_duration_ms"`
    Status        JobExecutionStatus    `json:"status"`
    SuccessCount  int64                 `json:"success_count"`
    FailureCount  int64                 `json:"failure_count"`
    LastError     string                `json:"last_error,omitempty"`
    NextRunTime   time.Time             `json:"next_run_time,omitempty"`
    Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type JobExecutionStatus string

const (
    JobStatusPending   JobExecutionStatus = "pending"
    JobStatusRunning   JobExecutionStatus = "running"
    JobStatusSuccess   JobExecutionStatus = "success"
    JobStatusFailed    JobExecutionStatus = "failed"
    JobStatusStalled   JobExecutionStatus = "stalled"
)
```

**Thread-Safe Status Manager**:
```go
type JobStatusManager struct {
    mu       sync.RWMutex
    statuses map[string]*JobStatus
    logger   *logger.Logger
    metrics  *BackgroundJobMetrics
}

func NewJobStatusManager(logger *logger.Logger) *JobStatusManager {
    return &JobStatusManager{
        statuses: make(map[string]*JobStatus),
        logger:   logger,
        metrics:  NewBackgroundJobMetrics(),
    }
}

func (jsm *JobStatusManager) UpdateJobStatus(jobName string, status JobExecutionStatus, duration time.Duration, err error) {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    jobStatus, exists := jsm.statuses[jobName]
    if !exists {
        jobStatus = &JobStatus{
            JobName:  jobName,
            Metadata: make(map[string]interface{}),
        }
        jsm.statuses[jobName] = jobStatus
    }
    
    jobStatus.LastRunTime = time.Now()
    jobStatus.LastDuration = duration
    jobStatus.Status = status
    
    if err != nil {
        jobStatus.FailureCount++
        jobStatus.LastError = err.Error()
        jsm.metrics.jobRuns.WithLabelValues(jobName, "error").Inc()
    } else {
        jobStatus.SuccessCount++
        jobStatus.LastError = ""
        jsm.metrics.jobRuns.WithLabelValues(jobName, "success").Inc()
    }
    
    jsm.metrics.jobDuration.WithLabelValues(jobName, string(status)).Observe(duration.Seconds())
}
```

### 2. Cron Job Instrumentation Strategy

**Job Wrapper Implementation**:
```go
type InstrumentedJob struct {
    jobName       string
    jobFunc       func() error
    statusManager *JobStatusManager
    logger        *logger.Logger
}

func NewInstrumentedJob(jobName string, jobFunc func() error, statusManager *JobStatusManager, logger *logger.Logger) *InstrumentedJob {
    return &InstrumentedJob{
        jobName:       jobName,
        jobFunc:       jobFunc,
        statusManager: statusManager,
        logger:        logger,
    }
}

func (ij *InstrumentedJob) Execute() {
    start := time.Now()
    
    // Update status to running
    ij.statusManager.UpdateJobStatus(ij.jobName, JobStatusRunning, 0, nil)
    
    // Log job start
    ij.logger.Info("Background job started", map[string]string{
        "job_name": ij.jobName,
        "start_time": start.Format(time.RFC3339),
    })
    
    // Execute with recovery
    var err error
    func() {
        defer func() {
            if r := recover(); r != nil {
                err = fmt.Errorf("job panicked: %v", r)
                ij.logger.Error("Background job panicked", map[string]string{
                    "job_name": ij.jobName,
                    "panic": fmt.Sprintf("%v", r),
                })
            }
        }()
        
        err = ij.jobFunc()
    }()
    
    duration := time.Since(start)
    status := JobStatusSuccess
    if err != nil {
        status = JobStatusFailed
    }
    
    // Update final status
    ij.statusManager.UpdateJobStatus(ij.jobName, status, duration, err)
    
    // Log completion
    logFields := map[string]string{
        "job_name": ij.jobName,
        "duration": duration.String(),
        "status":   string(status),
    }
    
    if err != nil {
        logFields["error"] = err.Error()
        ij.logger.Error("Background job failed", logFields)
    } else {
        ij.logger.Info("Background job completed", logFields)
    }
}
```

### 3. Integration with Existing Telemetry System

**Modified Server Initialization**:
```go
func Init() {
    // ... existing initialization
    
    // Create job status manager
    jobStatusManager := NewJobStatusManager(logger)
    
    // Initialize telemetry with status manager
    telemetryInstance := telemetry.New(
        db,
        s,
        appConfig,
        logger,
        btcRpc,
        baseRpc,
        oracle,
        jobStatusManager, // Add status manager
    )
    
    c := cron.New(cron.WithLogger(cron.VerbosePrintfLogger(logger)))
    
    // Create instrumented jobs
    btcIndexingJob := NewInstrumentedJob(
        "btc_transaction_indexing",
        telemetryInstance.IndexBtcTransaction,
        jobStatusManager,
        logger,
    )
    
    icyIndexingJob := NewInstrumentedJob(
        "icy_transaction_indexing", 
        telemetryInstance.IndexIcyTransaction,
        jobStatusManager,
        logger,
    )
    
    swapProcessingJob := NewInstrumentedJob(
        "swap_request_processing",
        telemetryInstance.ProcessSwapRequests,
        jobStatusManager,
        logger,
    )
    
    // Schedule instrumented jobs
    indexInterval := "2m"
    if appConfig.IndexInterval != "" {
        indexInterval = appConfig.IndexInterval
    }
    
    c.AddFunc("@every "+indexInterval, func() {
        go btcIndexingJob.Execute()
        go icyIndexingJob.Execute()
        go swapProcessingJob.Execute()
        // Add other jobs...
    })
    
    c.Start()
    
    // Initialize HTTP server with job status manager
    httpServer := http.NewHttpServer(appConfig, logger, oracle, baseRpc, btcRpc, db, jobStatusManager)
    httpServer.Run()
}
```

### 4. Job Status HTTP Endpoint

**Health Handler Extension**:
```go
type JobsHealthResponse struct {
    Status    string                 `json:"status"`
    Timestamp time.Time              `json:"timestamp"`  
    Jobs      map[string]JobStatus   `json:"jobs"`
    Summary   JobsSummary            `json:"summary"`
}

type JobsSummary struct {
    TotalJobs     int   `json:"total_jobs"`
    RunningJobs   int   `json:"running_jobs"`
    HealthyJobs   int   `json:"healthy_jobs"`
    UnhealthyJobs int   `json:"unhealthy_jobs"`
    StalledJobs   int   `json:"stalled_jobs"`
}

func (h *HealthHandler) Jobs(c *gin.Context) {
    start := time.Now()
    
    jobs := h.jobStatusManager.GetAllJobStatuses()
    summary := calculateJobsSummary(jobs)
    
    overallStatus := "healthy"
    if summary.UnhealthyJobs > 0 || summary.StalledJobs > 0 {
        overallStatus = "unhealthy"
    }
    
    response := JobsHealthResponse{
        Status:    overallStatus,
        Timestamp: time.Now(),
        Jobs:      jobs,
        Summary:   summary,
    }
    
    statusCode := http.StatusOK
    if overallStatus == "unhealthy" {
        statusCode = http.StatusServiceUnavailable
    }
    
    h.logger.Info("Jobs health check completed", map[string]string{
        "duration":        time.Since(start).String(),
        "overall_status":  overallStatus,
        "total_jobs":      fmt.Sprintf("%d", summary.TotalJobs),
        "unhealthy_jobs":  fmt.Sprintf("%d", summary.UnhealthyJobs),
    })
    
    c.JSON(statusCode, response)
}

func (jsm *JobStatusManager) GetAllJobStatuses() map[string]JobStatus {
    jsm.mu.RLock()
    defer jsm.mu.RUnlock()
    
    result := make(map[string]JobStatus)
    for name, status := range jsm.statuses {
        // Check for stalled jobs (no activity for >5 minutes)
        if time.Since(status.LastRunTime) > 5*time.Minute && status.Status == JobStatusRunning {
            status.Status = JobStatusStalled
        }
        result[name] = *status
    }
    
    return result
}
```

### 5. Background Job Metrics

**Comprehensive Job Metrics**:
```go
type BackgroundJobMetrics struct {
    jobDuration     *prometheus.HistogramVec
    jobRuns         *prometheus.CounterVec
    activeJobs      prometheus.Gauge
    pendingTransactions *prometheus.GaugeVec
    stalledJobs     prometheus.Gauge
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
        pendingTransactions: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_pending_transactions_total",
                Help: "Number of pending transactions by type",
            },
            []string{"transaction_type"}, // "btc", "icy", "swap"
        ),
        stalledJobs: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_background_jobs_stalled",
                Help: "Number of stalled background jobs",
            },
        ),
    }
}
```

### 6. Enhanced Telemetry Interface

**Extended Telemetry Interface**:
```go
type ITelemetry interface {
    // Existing methods
    IndexBtcTransaction() error
    IndexIcyTransaction() error  
    IndexIcySwapTransaction() error
    GetIcyTransactionByHash(hash string) (*model.OnchainIcyTransaction, error)
    GetBtcTransactionByInternalID(internalID string) (*model.OnchainBtcTransaction, error)
    ProcessPendingBtcTransactions() error
    ProcessSwapRequests() error
    
    // New monitoring methods
    GetPendingTransactionCounts() map[string]int64
    GetJobExecutionStats(jobName string) (JobExecutionStats, error)
    UpdateTransactionMetrics()
}

type JobExecutionStats struct {
    TotalRuns           int64         `json:"total_runs"`
    SuccessfulRuns      int64         `json:"successful_runs"`
    FailedRuns          int64         `json:"failed_runs"`
    AverageExecutionTime time.Duration `json:"average_execution_time"`
    LastExecutionTime   time.Time     `json:"last_execution_time"`
}
```

### 7. Stalled Job Detection

**Automated Stall Detection**:
```go  
func (jsm *JobStatusManager) StartStalledJobDetection(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            jsm.detectStalledJobs()
        }
    }()
}

func (jsm *JobStatusManager) detectStalledJobs() {
    jsm.mu.Lock()
    defer jsm.mu.Unlock()
    
    stalledThreshold := 5 * time.Minute
    stalledCount := 0
    
    for jobName, status := range jsm.statuses {
        if status.Status == JobStatusRunning && 
           time.Since(status.LastRunTime) > stalledThreshold {
            
            status.Status = JobStatusStalled
            stalledCount++
            
            jsm.logger.Error("Background job stalled", map[string]string{
                "job_name": jobName,
                "last_run": status.LastRunTime.Format(time.RFC3339),
                "duration_since": time.Since(status.LastRunTime).String(),
            })
        }
    }
    
    jsm.metrics.stalledJobs.Set(float64(stalledCount))
}
```

### 8. Job Performance Analysis

**Performance Tracking Integration**:
```go
func (ij *InstrumentedJob) ExecuteWithMetrics() {
    start := time.Now()
    
    // Update active jobs counter
    ij.statusManager.metrics.activeJobs.Inc()
    defer ij.statusManager.metrics.activeJobs.Dec()
    
    // Track pending transactions before job execution
    if pendingCounts := ij.getPendingTransactionCounts(); pendingCounts != nil {
        for txType, count := range pendingCounts {
            ij.statusManager.metrics.pendingTransactions.WithLabelValues(txType).Set(float64(count))
        }
    }
    
    // Execute job (existing logic)
    ij.Execute()
    
    // Update metrics after execution
    if pendingCounts := ij.getPendingTransactionCounts(); pendingCounts != nil {
        for txType, count := range pendingCounts {
            ij.statusManager.metrics.pendingTransactions.WithLabelValues(txType).Set(float64(count))
        }
    }
}

func (ij *InstrumentedJob) getPendingTransactionCounts() map[string]int64 {
    // This would integrate with telemetry service to get actual counts
    // Implementation depends on telemetry interface extension
    return map[string]int64{
        "btc":  0, // Get from telemetry
        "icy":  0, // Get from telemetry  
        "swap": 0, // Get from telemetry
    }
}
```

## Implementation Strategy

### 1. Phased Implementation

**Week 1**: Basic job status tracking
- Implement JobStatusManager and InstrumentedJob
- Add basic metrics collection
- Create jobs health endpoint

**Week 2**: Enhanced monitoring  
- Add stalled job detection
- Implement comprehensive metrics
- Integration with existing telemetry

**Week 3**: Performance optimization
- Add pending transaction tracking
- Implement job performance analysis
- Optimize memory usage and cleanup

### 2. Testing Strategy

**Unit Testing**:
- JobStatusManager thread safety
- InstrumentedJob execution and error handling
- Metrics collection accuracy

**Integration Testing**:
- Job status endpoint responses
- Cron job scheduling with instrumentation
- Stalled job detection scenarios

**Performance Testing**:
- Memory usage under load
- Job execution overhead measurement
- Concurrent job execution handling

## Consequences

### Positive
- **Visibility**: Complete insight into background job performance and health
- **Reliability**: Early detection of stalled or failing jobs
- **Debugging**: Detailed logging and metrics for troubleshooting
- **Performance**: Understanding of job execution patterns and optimization opportunities
- **Integration**: Seamless integration with existing cron and telemetry systems

### Negative
- **Memory Usage**: In-memory status tracking consumes additional memory
- **Complexity**: Additional code complexity for job instrumentation
- **Performance Overhead**: Small overhead for metrics collection and status tracking

### Risks and Mitigations
- **Memory Growth**: Job status data could grow unbounded
  - *Mitigation*: Implement status cleanup and rotation
- **Thread Safety**: Concurrent access to status data
  - *Mitigation*: Proper mutex usage and testing
- **Job Interference**: Monitoring could interfere with job execution
  - *Mitigation*: Minimal overhead design and async logging

## Alternatives Considered

1. **Database-Persisted Status**
   - Pros: Survives restarts, historical data
   - Cons: Database overhead, complexity

2. **External Job Queue (Redis/RabbitMQ)**
   - Pros: Advanced job management features
   - Cons: Additional infrastructure, migration complexity

3. **No Job Monitoring**
   - Pros: Zero implementation cost
   - Cons: No visibility into critical background processes

## References
- [robfig/cron v3 Documentation](https://pkg.go.dev/github.com/robfig/cron/v3)
- [Go Context and Cancellation](https://blog.golang.org/context)
- [Prometheus Job Monitoring Patterns](https://prometheus.io/docs/practices/instrumentation/#batch-jobs)