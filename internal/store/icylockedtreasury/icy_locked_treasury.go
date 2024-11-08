package icylockedtreasury

import (
	"github.com/dwarvesf/icy-backend/internal/model"
	"gorm.io/gorm"
)

type store struct{}

func New() IStore {
	return &store{}
}

func (s *store) All(db *gorm.DB) ([]*model.IcyLockedTreasury, error) {
	var icyLockedTreasuries []*model.IcyLockedTreasury
	err := db.Find(&icyLockedTreasuries).Error
	if err != nil {
		return nil, err
	}
	return icyLockedTreasuries, nil
}
