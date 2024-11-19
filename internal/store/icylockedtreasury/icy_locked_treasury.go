package icylockedtreasury

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type store struct{}

func New() IStore {
	return &store{}
}

func (s *store) All(db *gorm.DB) ([]*model.IcyLockedTreasury, error) {
	var icyLockedTreasuries []*model.IcyLockedTreasury
	return icyLockedTreasuries, db.Find(&icyLockedTreasuries).Error
}
