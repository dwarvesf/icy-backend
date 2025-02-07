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

func (s *store) GetLatestTransaction(db *gorm.DB) (*model.OnchainBtcTransaction, error) {
	var onchainBtcTransaction model.OnchainBtcTransaction
	return &onchainBtcTransaction, db.Order("created_at desc").First(&onchainBtcTransaction).Error
}

func (s *store) GetByInternalID(db *gorm.DB, internalID string) (*model.OnchainBtcTransaction, error) {
	var tx model.OnchainBtcTransaction
	err := db.Where("internal_id = ?", internalID).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}
