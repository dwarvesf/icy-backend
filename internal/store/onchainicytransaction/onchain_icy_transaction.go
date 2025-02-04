package onchainicytransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type store struct{}

func New() IStore {
	return &store{}
}

func (s *store) Create(db *gorm.DB, onchainIcyTransaction *model.OnchainIcyTransaction) (*model.OnchainIcyTransaction, error) {
	return onchainIcyTransaction, db.Create(onchainIcyTransaction).Error
}

func (s *store) GetLatestTransaction(db *gorm.DB) (*model.OnchainIcyTransaction, error) {
	var onchainIcyTransaction model.OnchainIcyTransaction
	return &onchainIcyTransaction, db.Order("created_at desc").First(&onchainIcyTransaction).Error
}
