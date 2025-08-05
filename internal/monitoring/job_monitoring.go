package monitoring

import (
	"context"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// JobExecutionStatus represents different job execution states
type JobExecutionStatus string

const (
	JobStatusPending JobExecutionStatus = "pending"
	JobStatusRunning JobExecutionStatus = "running"
	JobStatusSuccess JobExecutionStatus = "success"
	JobStatusFailed  JobExecutionStatus = "failed"
	JobStatusStalled JobExecutionStatus = "stalled"
)

// JobStatus contains complete status information for a background job
type JobStatus struct {
	JobName             string                 `json:"job_name"`
	Status              JobExecutionStatus     `json:"status"`
	LastRunTime         time.Time              `json:"last_run_time"`
	LastDuration        time.Duration          `json:"last_duration_ms"`
	NextRunTime         time.Time              `json:"next_run_time,omitempty"`
	SuccessCount        int64                  `json:"success_count"`
	FailureCount        int64                  `json:"failure_count"`
	ConsecutiveFailures int64                  `json:"consecutive_failures"`
	LastError           string                 `json:"last_error,omitempty"`
	AverageExecution    time.Duration          `json:"average_execution_ms"`
	MaxExecutionTime    time.Duration          `json:"max_execution_ms"`
	MinExecutionTime    time.Duration          `json:"min_execution_ms"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// JobsSummary provides an overview of all job statuses
type JobsSummary struct {
	TotalJobs      int       `json:"total_jobs"`
	RunningJobs    int       `json:"running_jobs"`
	HealthyJobs    int       `json:"healthy_jobs"`
	UnhealthyJobs  int       `json:"unhealthy_jobs"`
	StalledJobs    int       `json:"stalled_jobs"`
	LastUpdateTime time.Time `json:"last_update_time"`
}

// JobStatusManager manages job status tracking with thread-safe operations
type JobStatusManager struct {
	mu               sync.RWMutex
	statuses         map[string]*JobStatus
	logger           *logger.Logger
	metrics          *BackgroundJobMetrics
	stalledThreshold time.Duration
	cleanupInterval  time.Duration
	retentionPeriod  time.Duration
}

// NewJobStatusManager creates a new job status manager instance
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

// RegisterJob registers a new job for monitoring
func (jsm *JobStatusManager) RegisterJob(jobName string) {
	jsm.mu.Lock()
	defer jsm.mu.Unlock()

	if _, exists := jsm.statuses[jobName]; !exists {
		jsm.statuses[jobName] = &JobStatus{
			JobName:          jobName,
			Status:           JobStatusPending,
			Metadata:         make(map[string]interface{}),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
			MinExecutionTime: time.Duration(math.MaxInt64),
		}

		jsm.logger.Info("Job registered for monitoring", map[string]string{
			"job_name": jobName,
		})
	}
}

// StartJob marks a job as started and updates its status
func (jsm *JobStatusManager) StartJob(jobName string) {
	jsm.mu.Lock()
	defer jsm.mu.Unlock()

	status, exists := jsm.statuses[jobName]
	if !exists {
		jsm.statuses[jobName] = &JobStatus{
			JobName:          jobName,
			Status:           JobStatusRunning,
			LastRunTime:      time.Now(),
			UpdatedAt:        time.Now(),
			CreatedAt:        time.Now(),
			Metadata:         make(map[string]interface{}),
			MinExecutionTime: time.Duration(math.MaxInt64),
		}
		status = jsm.statuses[jobName]
	} else {
		status.Status = JobStatusRunning
		status.LastRunTime = time.Now()
		status.UpdatedAt = time.Now()
	}

	// Update metrics
	jsm.metrics.activeJobs.Inc()

	jsm.logger.Info("Job started", map[string]string{
		"job_name":   jobName,
		"start_time": status.LastRunTime.Format(time.RFC3339),
	})
}

// CompleteJob marks a job as completed and updates all relevant statistics
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

		// Add error type to metadata
		if status.Metadata == nil {
			status.Metadata = make(map[string]interface{})
		}
		status.Metadata["error_type"] = classifyJobError(err)

		// Record metrics
		jsm.metrics.jobRuns.WithLabelValues(jobName, "error").Inc()
		jsm.metrics.jobDuration.WithLabelValues(jobName, "failed").Observe(duration.Seconds())

		jsm.logger.Error("Job failed", map[string]string{
			"job_name":             jobName,
			"duration":             duration.String(),
			"error":                err.Error(),
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

// GetJobStatus returns the current status of a specific job
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

// GetAllJobStatuses returns the current status of all jobs
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

// GetJobsSummary returns a summary of all job statuses
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

// startStalledJobDetection starts the background stalled job detection process
func (jsm *JobStatusManager) startStalledJobDetection() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		jsm.detectStalledJobs()
	}
}

// detectStalledJobs checks for jobs that have been running longer than the threshold
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

// startPeriodicCleanup starts the background cleanup process
func (jsm *JobStatusManager) startPeriodicCleanup() {
	ticker := time.NewTicker(jsm.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		jsm.cleanupOldStatuses()
	}
}

// cleanupOldStatuses removes old job statuses to prevent memory leaks
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

// InstrumentedJob wraps a job function with monitoring and error handling
type InstrumentedJob struct {
	jobName       string
	jobFunc       func() error
	statusManager *JobStatusManager
	logger        *logger.Logger
	timeout       time.Duration
}

// NewInstrumentedJob creates a new instrumented job wrapper
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

// Execute runs the job with monitoring, timeout, and panic recovery
func (ij *InstrumentedJob) Execute() {
	// Start job tracking
	ij.statusManager.StartJob(ij.jobName)

	// Setup timeout context
	ctx, cancel := context.WithTimeout(context.Background(), ij.timeout)
	defer cancel()

	// Execute job with timeout and panic recovery
	var err error
	var metadata map[string]interface{}

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr := fmt.Errorf("job panicked: %v", r)
				metadata = map[string]interface{}{
					"panic":       fmt.Sprintf("%v", r),
					"stack_trace": string(debug.Stack()),
					"error_type":  "panic",
				}

				ij.logger.Error("Job panicked", map[string]string{
					"job_name": ij.jobName,
					"panic":    fmt.Sprintf("%v", r),
				})
				
				done <- panicErr
			}
		}()
		done <- ij.jobFunc()
	}()

	select {
	case err = <-done:
		// Job completed (or panicked)
		if err != nil && metadata == nil {
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

	// Complete job tracking
	ij.statusManager.CompleteJob(ij.jobName, err, metadata)
}

// BackgroundJobMetrics contains all Prometheus metrics for background job monitoring
type BackgroundJobMetrics struct {
	jobDuration         *prometheus.HistogramVec
	jobRuns             *prometheus.CounterVec
	activeJobs          prometheus.Gauge
	stalledJobs         prometheus.Gauge
	pendingTransactions *prometheus.GaugeVec
	jobExecutionHistory *prometheus.CounterVec
	jobTimeouts         *prometheus.CounterVec
}

// NewBackgroundJobMetrics creates a new instance of background job metrics
func NewBackgroundJobMetrics() *BackgroundJobMetrics {
	return &BackgroundJobMetrics{
		jobDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "icy_backend_background_job_duration_seconds",
				Help:    "Background job execution duration in seconds",
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

// MustRegister registers all background job metrics with the provided registry
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

// classifyJobError classifies errors into different types for better monitoring
func classifyJobError(err error) string {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "timeout"), strings.Contains(errStr, "deadline"):
		return "timeout"
	case strings.Contains(errStr, "database"), strings.Contains(errStr, "sql"):
		return "database"
	case strings.Contains(errStr, "connection"), strings.Contains(errStr, "network"):
		return "network"
	case strings.Contains(errStr, "external"), strings.Contains(errStr, "api"):
		return "external_api"
	case strings.Contains(errStr, "panic"):
		return "panic"
	default:
		return "unknown"
	}
}