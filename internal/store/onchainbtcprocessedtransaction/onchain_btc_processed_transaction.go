package onchainbtcprocessedtransaction

import (
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type store struct {
	db *gorm.DB
}

func New(db *gorm.DB) IStore {
	return &store{db: db}
}

func (s *store) Create(btcProcessedTx *model.OnchainBtcProcessedTransaction) (*model.OnchainBtcProcessedTransaction, error) {
	btcProcessedTx.CreatedAt = time.Now()
	btcProcessedTx.UpdatedAt = time.Now()
	return btcProcessedTx, s.db.Create(btcProcessedTx).Error
}

func (s *store) GetByIcyTransactionHash(icyTxHash string) (*model.OnchainBtcProcessedTransaction, error) {
	var btcProcessedTx model.OnchainBtcProcessedTransaction
	result := s.db.Where("icy_transaction_hash = ?", icyTxHash).First(&btcProcessedTx)
	if result.Error != nil {
		return nil, result.Error
	}
	return &btcProcessedTx, nil
}

func (s *store) UpdateStatus(id int, status model.BtcProcessingStatus) error {
	return s.db.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error
}

func (s *store) UpdateToCompleted(id int, btcTxHash string) error {
	return s.db.Model(&model.OnchainBtcProcessedTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":               model.BtcProcessingStatusCompleted,
		"btc_transaction_hash": btcTxHash,
		"updated_at":           time.Now(),
	}).Error
}

func (s *store) GetPendingTransactions() ([]model.OnchainBtcProcessedTransaction, error) {
	var pendingTxs []model.OnchainBtcProcessedTransaction
	err := s.db.Where("status = ?", model.BtcProcessingStatusPending).Find(&pendingTxs).Error
	return pendingTxs, err
}
