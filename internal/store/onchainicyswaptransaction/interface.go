package onchainicyswaptransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type Store interface {
	Create(db *gorm.DB, transaction *model.OnchainIcySwapTransaction) (*model.OnchainIcySwapTransaction, error)
	GetByTransactionHash(db *gorm.DB, hash string) (*model.OnchainIcySwapTransaction, error)
	GetLatestTransaction(db *gorm.DB) (*model.OnchainIcySwapTransaction, error)
}
