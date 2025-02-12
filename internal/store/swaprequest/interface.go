package swaprequest

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IStore interface {
	Create(tx *gorm.DB, swapRequest *model.SwapRequest) (*model.SwapRequest, error)
	GetByIcyTx(tx *gorm.DB, icyTx string) (*model.SwapRequest, error)
	UpdateStatus(tx *gorm.DB, icyTx, status string) error
}
