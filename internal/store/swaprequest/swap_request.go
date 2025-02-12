package swaprequest

import (
	"time"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type Store struct {
}

func New() IStore {
	return &Store{}
}

func (s *Store) Create(tx *gorm.DB, swapRequest *model.SwapRequest) (*model.SwapRequest, error) {
	return swapRequest, tx.Create(swapRequest).Error
}

func (s *Store) GetByIcyTx(tx *gorm.DB, icyTx string) (*model.SwapRequest, error) {
	var swapRequest model.SwapRequest
	err := tx.Where("icy_tx = ?", icyTx).First(&swapRequest).Error
	if err != nil {
		return nil, err
	}
	return &swapRequest, nil
}

func (s *Store) FindPendingSwapRequests(tx *gorm.DB) ([]model.SwapRequest, error) {
	var swapRequests []model.SwapRequest
	err := tx.Where("status = ?", "pending").Find(&swapRequests).Error
	if err != nil {
		return nil, err
	}
	return swapRequests, nil
}

func (s *Store) UpdateStatus(tx *gorm.DB, icyTx, status string) error {
	return tx.Model(&model.SwapRequest{}).
		Where("icy_tx = ?", icyTx).
		Updates(map[string]interface{}{
			"status":       status,
			"processed_at": time.Now(),
		}).Error
}
