package telemetry

import "github.com/dwarvesf/icy-backend/internal/model"

type ITelemetry interface {
	IndexBtcTransaction() error
	IndexIcySwapTransaction() error
	GetBtcTransactionByInternalID(internalID string) (*model.OnchainBtcTransaction, error)
	ProcessPendingBtcTransactions() error
}
