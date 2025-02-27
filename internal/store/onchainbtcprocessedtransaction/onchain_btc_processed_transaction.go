package onchainbtcprocessedtransaction

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type store struct {
}

func New() IStore {
	return &store{}
}

func (s *store) Create(tx *gorm.DB, btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error) {
	btcProcessedTx.CreatedAt = time.Now()
	btcProcessedTx.UpdatedAt = time.Now()
	return btcProcessedTx, tx.Create(btcProcessedTx).Error
}

func (s *store) GetByIcyTransactionHash(tx *gorm.DB, icyTxHash string) (*model.OnchainBtcProcessedTransaction, error) {
	var btcProcessedTx model.OnchainBtcProcessedTransaction
	result := tx.Where("icy_transaction_hash = ?", icyTxHash).First(&btcProcessedTx)
	if result.Error != nil {
		return nil, result.Error
	}
	return &btcProcessedTx, nil
}

func (s *store) UpdateStatus(tx *gorm.DB, id int, status model.BtcProcessingStatus) error {
	return tx.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error
}

func (s *store) UpdateToCompleted(tx *gorm.DB, id int, btcTxHash string, networkFee int64) error {
	return tx.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":               model.BtcProcessingStatusCompleted,
		"btc_transaction_hash": btcTxHash,
		"network_fee":          fmt.Sprintf("%d", networkFee),
		"updated_at":           time.Now(),
		"processed_at":         time.Now(),
	}).Error
}

func (s *store) GetPendingTransactions(tx *gorm.DB) ([]model.OnchainBtcProcessedTransaction, error) {
	var pendingTxs []model.OnchainBtcProcessedTransaction
	err := tx.Where("status = ?", model.BtcProcessingStatusPending).Find(&pendingTxs).Error
	return pendingTxs, err
}

func (s *store) Find(db *gorm.DB, filter ListFilter) ([]*model.OnchainBtcProcessedTransaction, int64, error) {
	var transactions []*model.OnchainBtcProcessedTransaction
	var total int64

	// Start with base query
	query := db.Model(&model.OnchainBtcProcessedTransaction{})

	// Apply filters
	if filter.BTCAddress != "" {
		query = query.Where("LOWER(btc_address) = ?", strings.ToLower(filter.BTCAddress))
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EVMAddress != "" {
		query = query.Joins("LEFT JOIN onchain_icy_swap_transactions ON onchain_icy_swap_transactions.transaction_hash = onchain_btc_processed_transactions.swap_transaction_hash").
			Where("LOWER(onchain_icy_swap_transactions.from_address) = ?", strings.ToLower(filter.EVMAddress))
	}

	// Count total records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Prepare final query with preloading
	finalQuery := db.Model(&model.OnchainBtcProcessedTransaction{}).
		Preload("OnchainIcySwapTransaction")

	// Reapply all filters to final query
	if filter.BTCAddress != "" {
		finalQuery = finalQuery.Where("LOWER(btc_address) = ?", strings.ToLower(filter.BTCAddress))
	}
	if filter.Status != "" {
		finalQuery = finalQuery.Where("status = ?", filter.Status)
	}
	if filter.EVMAddress != "" {
		finalQuery = finalQuery.Joins("LEFT JOIN onchain_icy_swap_transactions ON onchain_icy_swap_transactions.transaction_hash = onchain_btc_processed_transactions.swap_transaction_hash").
			Where("LOWER(onchain_icy_swap_transactions.from_address) = ?", strings.ToLower(filter.EVMAddress))
	}

	// Apply pagination and ordering
	finalQuery = finalQuery.
		Offset(filter.Offset).
		Limit(filter.Limit).
		Order("updated_at DESC")

	// Fetch transactions
	if err := finalQuery.Find(&transactions).Error; err != nil {
		return nil, 0, err
	}

	for i := range transactions {
		subtotal, err := strconv.ParseInt(transactions[i].Subtotal, 10, 64)
		if err != nil {
			continue
		}
		svcFee, err := strconv.ParseInt(transactions[i].ServiceFee, 10, 64)
		if err != nil {
			continue
		}
		totalAmount := subtotal - svcFee
		transactions[i].Total = strconv.FormatInt(totalAmount, 10)
		transactions[i].TotalAmount = strconv.FormatInt(totalAmount, 10) // For backward compatibility
	}

	return transactions, total, nil
}
