package onchainbtctransaction

import (
	"github.com/dwarvesf/icy-backend/internal/model"
	"gorm.io/gorm"
)

type store struct{}

func New() IStore {
	return &store{}
}

func (s *store) Create(db *gorm.DB, onchainBtcTransaction *model.OnchainBtcTransaction) (*model.OnchainBtcTransaction, error) {
	return onchainBtcTransaction, db.Create(onchainBtcTransaction).Error
}
