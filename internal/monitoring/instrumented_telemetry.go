package monitoring

import (
	"time"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/utils/webhook"
)

// InstrumentedTelemetry wraps the base telemetry with job monitoring capabilities
type InstrumentedTelemetry struct {
	baseTelemetry   telemetry.ITelemetry
	statusManager   *JobStatusManager
	metrics         *BackgroundJobMetrics
	logger          *logger.Logger
	config          *config.AppConfig
	webhookClient   *webhook.Client
}

// NewInstrumentedTelemetry creates a new instrumented telemetry wrapper
func NewInstrumentedTelemetry(
	baseTelemetry telemetry.ITelemetry,
	statusManager *JobStatusManager,
	metrics *BackgroundJobMetrics,
	logger *logger.Logger,
	config *config.AppConfig,
) *InstrumentedTelemetry {
	return &InstrumentedTelemetry{
		baseTelemetry: baseTelemetry,
		statusManager: statusManager,
		metrics:       metrics,
		logger:        logger,
		config:        config,
		webhookClient: webhook.New(logger),
	}
}

// IndexBtcTransaction wraps the base BTC transaction indexing with job monitoring
func (it *InstrumentedTelemetry) IndexBtcTransaction() error {
	return it.executeJobWithWebhook(
		"btc_transaction_indexing",
		it.baseTelemetry.IndexBtcTransaction,
		it.config.UptimeWebhooks.IndexBtcTransactionURL,
		10*time.Minute,
	)
}

// IndexIcyTransaction wraps the base ICY transaction indexing with job monitoring
func (it *InstrumentedTelemetry) IndexIcyTransaction() error {
	return it.executeJobWithWebhook(
		"icy_transaction_indexing",
		it.baseTelemetry.IndexIcyTransaction,
		it.config.UptimeWebhooks.IndexIcyTransactionURL,
		10*time.Minute,
	)
}

// IndexIcySwapTransaction wraps the base ICY swap transaction indexing with job monitoring
func (it *InstrumentedTelemetry) IndexIcySwapTransaction() error {
	return it.executeJobWithWebhook(
		"icy_swap_transaction_indexing",
		it.baseTelemetry.IndexIcySwapTransaction,
		it.config.UptimeWebhooks.IndexIcySwapTransactionURL,
		10*time.Minute,
	)
}

// ProcessSwapRequests wraps the base swap request processing with job monitoring
func (it *InstrumentedTelemetry) ProcessSwapRequests() error {
	return it.executeJobWithWebhook(
		"swap_request_processing",
		it.baseTelemetry.ProcessSwapRequests,
		it.config.UptimeWebhooks.ProcessSwapRequestsURL,
		15*time.Minute,
	)
}

// ProcessPendingBtcTransactions wraps the base pending BTC transaction processing with job monitoring
func (it *InstrumentedTelemetry) ProcessPendingBtcTransactions() error {
	return it.executeJobWithWebhook(
		"btc_pending_transaction_processing",
		it.baseTelemetry.ProcessPendingBtcTransactions,
		it.config.UptimeWebhooks.ProcessPendingBtcTransactionsURL,
		15*time.Minute,
	)
}

// GetIcyTransactionByHash delegates to the base telemetry without instrumentation
func (it *InstrumentedTelemetry) GetIcyTransactionByHash(hash string) (*model.OnchainIcyTransaction, error) {
	return it.baseTelemetry.GetIcyTransactionByHash(hash)
}

// GetBtcTransactionByInternalID delegates to the base telemetry without instrumentation
func (it *InstrumentedTelemetry) GetBtcTransactionByInternalID(internalID string) (*model.OnchainBtcTransaction, error) {
	return it.baseTelemetry.GetBtcTransactionByInternalID(internalID)
}

// Helper methods to get pending transaction counts
// These would be implemented if we had access to the store interface

// func (it *InstrumentedTelemetry) getBtcPendingCount() int64 {
// 	// Implementation would depend on store interface
// 	// This is a placeholder showing the pattern
// 	return -1 // Return -1 if count unavailable
// }

// func (it *InstrumentedTelemetry) getIcyPendingCount() int64 {
// 	return -1
// }

// func (it *InstrumentedTelemetry) getSwapPendingCount() int64 {
// 	return -1
// }

// executeJobWithWebhook executes a job and calls the webhook on successful completion
func (it *InstrumentedTelemetry) executeJobWithWebhook(jobName string, jobFunc func() error, webhookURL string, timeout time.Duration) error {
	job := NewInstrumentedJobWithWebhook(
		jobName,
		jobFunc,
		it.statusManager,
		it.logger,
		timeout,
		it.webhookClient,
		webhookURL,
	)

	job.Execute()
	return nil // Always return nil since error handling is done in the job
}