package onchainicytransaction

import (
	"errors"

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
	result := db.Order("block_number desc").First(&onchainIcyTransaction)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &onchainIcyTransaction, nil
}

func (s *store) GetByTransactionHash(db *gorm.DB, txHash string) (*model.OnchainIcyTransaction, error) {
	var onchainIcyTransaction model.OnchainIcyTransaction
	result := db.Where("transaction_hash = ?", txHash).First(&onchainIcyTransaction)
	if result.Error != nil {
		return nil, result.Error
	}
	return &onchainIcyTransaction, nil
}
