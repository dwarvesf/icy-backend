package onchainicytransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type ListFilter struct {
	Limit     int
	Offset    int
	FromAddr  string
	ToAddr    string
	TxType    string
	Status    string
	StartTime int64
	EndTime   int64
}

type IStore interface {
	Create(db *gorm.DB, onchainIcyTransaction *model.OnchainIcyTransaction) (*model.OnchainIcyTransaction, error)
	GetLatestTransaction(db *gorm.DB) (*model.OnchainIcyTransaction, error)
	GetByTransactionHash(db *gorm.DB, txHash string) (*model.OnchainIcyTransaction, error)
}
