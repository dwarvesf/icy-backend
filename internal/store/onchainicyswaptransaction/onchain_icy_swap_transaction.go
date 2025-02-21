package onchainicyswaptransaction

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type store struct{}

func New() Store {
	return &store{}
}

func (s *store) Create(db *gorm.DB, transaction *model.OnchainIcySwapTransaction) (*model.OnchainIcySwapTransaction, error) {
	if err := db.Create(transaction).Error; err != nil {
		return nil, err
	}
	return transaction, nil
}

func (s *store) GetByTransactionHash(db *gorm.DB, hash string) (*model.OnchainIcySwapTransaction, error) {
	var transaction model.OnchainIcySwapTransaction
	if err := db.Where("transaction_hash = ?", hash).First(&transaction).Error; err != nil {
		return nil, err
	}
	return &transaction, nil
}

func (s *store) GetLatestTransaction(db *gorm.DB) (*model.OnchainIcySwapTransaction, error) {
	var transaction model.OnchainIcySwapTransaction
	if err := db.Order("block_number desc").First(&transaction).Error; err != nil {
		return nil, err
	}
	return &transaction, nil
}
