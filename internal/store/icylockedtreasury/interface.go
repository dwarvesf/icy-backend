package icylockedtreasury

import (
	"github.com/dwarvesf/icy-backend/internal/model"
	"gorm.io/gorm"
)

type IStore interface {
	All(db *gorm.DB) ([]*model.IcyLockedTreasury, error)
}
