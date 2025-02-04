package onchainicytransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IStore interface {
	Create(db *gorm.DB, onchainIcyTransaction *model.OnchainIcyTransaction) (*model.OnchainIcyTransaction, error)
	GetLatestTransaction(db *gorm.DB) (*model.OnchainIcyTransaction, error)
}
