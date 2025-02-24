package onchainicyswaptransaction

import (
	"fmt"
	"math/big"

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

// SumTotalIcyAmount calculates the total ICY amount of all OnchainIcySwapTransactions in the database
func (s *store) SumTotalIcyAmount(db *gorm.DB) (*big.Int, error) {
	var result struct {
		TotalAmount string
	}

	// Use SQL to sum the icy_amount column
	err := db.Model(&model.OnchainIcySwapTransaction{}).
		Select("SUM(icy_amount::numeric) as total_amount").
		Scan(&result).Error

	if err != nil {
		return nil, fmt.Errorf("failed to calculate total ICY amount: %v", err)
	}

	// Convert the sum to big.Int
	total, ok := new(big.Int).SetString(result.TotalAmount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid total ICY amount format: %s", result.TotalAmount)
	}

	return total, nil
}
