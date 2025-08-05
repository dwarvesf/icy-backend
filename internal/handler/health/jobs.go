package health

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/monitoring"
)

// Jobs handles the background jobs health check endpoint
// @Summary Background jobs health check
// @Description Validates background job status and performance
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} JobsHealthResponse
// @Failure 503 {object} JobsHealthResponse
// @Router /api/v1/health/jobs [get]
func (h *HealthHandler) Jobs(c *gin.Context) {
	start := time.Now()

	// Handle case where job status manager is not available
	if h.jobStatusManager == nil {
		response := JobsHealthResponse{
			Status:     "unhealthy",
			Timestamp:  time.Now(),
			Jobs:       make(map[string]monitoring.JobStatus),
			Summary:    monitoring.JobsSummary{},
			DurationMs: time.Since(start).Milliseconds(),
		}
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

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
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Jobs:       jobs,
		Summary:    summary,
		DurationMs: time.Since(start).Milliseconds(),
	}

	statusCode := http.StatusOK
	if overallStatus == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	} else if overallStatus == "degraded" {
		statusCode = http.StatusPartialContent // 206
	}

	// Log health check
	h.logger.Info("Jobs health check completed", map[string]string{
		"overall_status": overallStatus,
		"duration":       fmt.Sprintf("%dms", response.DurationMs),
		"total_jobs":     fmt.Sprintf("%d", summary.TotalJobs),
		"unhealthy_jobs": fmt.Sprintf("%d", summary.UnhealthyJobs),
		"stalled_jobs":   fmt.Sprintf("%d", summary.StalledJobs),
		"running_jobs":   fmt.Sprintf("%d", summary.RunningJobs),
	})

	c.JSON(statusCode, response)
}