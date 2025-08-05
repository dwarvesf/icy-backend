package monitoring

import (
	"time"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// InstrumentedTelemetry wraps the base telemetry with job monitoring capabilities
type InstrumentedTelemetry struct {
	baseTelemetry telemetry.ITelemetry
	statusManager *JobStatusManager
	metrics       *BackgroundJobMetrics
	logger        *logger.Logger
}

// NewInstrumentedTelemetry creates a new instrumented telemetry wrapper
func NewInstrumentedTelemetry(
	baseTelemetry telemetry.ITelemetry,
	statusManager *JobStatusManager,
	metrics *BackgroundJobMetrics,
	logger *logger.Logger,
) *InstrumentedTelemetry {
	return &InstrumentedTelemetry{
		baseTelemetry: baseTelemetry,
		statusManager: statusManager,
		metrics:       metrics,
		logger:        logger,
	}
}

// IndexBtcTransaction wraps the base BTC transaction indexing with job monitoring
func (it *InstrumentedTelemetry) IndexBtcTransaction() error {
	job := NewInstrumentedJob(
		"btc_transaction_indexing",
		it.baseTelemetry.IndexBtcTransaction,
		it.statusManager,
		it.logger,
		10*time.Minute, // 10 minute timeout
	)

	job.Execute()

	// Update pending transaction count if available  
	// Note: Implementation would depend on having access to store interface
	// This is a placeholder for now
	// if pendingCount := it.getBtcPendingCount(); pendingCount >= 0 {
	//     it.metrics.pendingTransactions.WithLabelValues("btc").Set(float64(pendingCount))
	// }

	return nil // Always return nil since error handling is done in the job
}

// IndexIcyTransaction wraps the base ICY transaction indexing with job monitoring
func (it *InstrumentedTelemetry) IndexIcyTransaction() error {
	job := NewInstrumentedJob(
		"icy_transaction_indexing",
		it.baseTelemetry.IndexIcyTransaction,
		it.statusManager,
		it.logger,
		10*time.Minute,
	)

	job.Execute()

	// Update pending transaction count if available
	// if pendingCount := it.getIcyPendingCount(); pendingCount >= 0 {
	//     it.metrics.pendingTransactions.WithLabelValues("icy").Set(float64(pendingCount))
	// }

	return nil
}

// IndexIcySwapTransaction wraps the base ICY swap transaction indexing with job monitoring
func (it *InstrumentedTelemetry) IndexIcySwapTransaction() error {
	job := NewInstrumentedJob(
		"icy_swap_transaction_indexing",
		it.baseTelemetry.IndexIcySwapTransaction,
		it.statusManager,
		it.logger,
		10*time.Minute,
	)

	job.Execute()

	return nil
}

// ProcessSwapRequests wraps the base swap request processing with job monitoring
func (it *InstrumentedTelemetry) ProcessSwapRequests() error {
	job := NewInstrumentedJob(
		"swap_request_processing",
		it.baseTelemetry.ProcessSwapRequests,
		it.statusManager,
		it.logger,
		15*time.Minute,
	)

	job.Execute()

	// Update pending transaction count if available
	// if pendingCount := it.getSwapPendingCount(); pendingCount >= 0 {
	//     it.metrics.pendingTransactions.WithLabelValues("swap").Set(float64(pendingCount))
	// }

	return nil
}

// ProcessPendingBtcTransactions wraps the base pending BTC transaction processing with job monitoring
func (it *InstrumentedTelemetry) ProcessPendingBtcTransactions() error {
	job := NewInstrumentedJob(
		"btc_pending_transaction_processing",
		it.baseTelemetry.ProcessPendingBtcTransactions,
		it.statusManager,
		it.logger,
		15*time.Minute,
	)

	job.Execute()

	return nil
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