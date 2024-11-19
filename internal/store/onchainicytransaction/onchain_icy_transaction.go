package onchainicytransaction

import (
	"github.com/dwarvesf/icy-backend/internal/model"
	"gorm.io/gorm"
)

type store struct{}

func New() IStore {
	return &store{}
}

func (s *store) Create(db *gorm.DB, onchainIcyTransaction *model.OnchainIcyTransaction) (*model.OnchainIcyTransaction, error) {
	return onchainIcyTransaction, db.Create(onchainIcyTransaction).Error
}
