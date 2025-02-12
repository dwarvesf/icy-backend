package telemetry

import "github.com/dwarvesf/icy-backend/internal/model"

type ITelemetry interface {
	IndexBtcTransaction() error
	IndexIcyTransaction() error
	GetIcyTransactionByHash(hash string) (*model.OnchainIcyTransaction, error)
	GetBtcTransactionByInternalID(internalID string) (*model.OnchainBtcTransaction, error)
}
