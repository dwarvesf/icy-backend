package health

import (
	"time"

	"github.com/dwarvesf/icy-backend/internal/monitoring"
)

// BasicHealthResponse represents the response for basic health check
type BasicHealthResponse struct {
	Message string `json:"message"`
}

// HealthResponse represents the response for detailed health checks
type HealthResponse struct {
	Status     string                    `json:"status"`
	Timestamp  time.Time                 `json:"timestamp"`
	Checks     map[string]HealthCheck    `json:"checks"`
	DurationMs int64                     `json:"duration_ms"`
}

// HealthCheck represents a single health check result
type HealthCheck struct {
	Status   string                 `json:"status"`
	Latency  int64                  `json:"latency_ms,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// JobsHealthResponse represents the response for background job health check
type JobsHealthResponse struct {
	Status     string                        `json:"status"`
	Timestamp  time.Time                     `json:"timestamp"`
	Jobs       map[string]monitoring.JobStatus `json:"jobs"`
	Summary    monitoring.JobsSummary        `json:"summary"`
	DurationMs int64                         `json:"duration_ms"`
}