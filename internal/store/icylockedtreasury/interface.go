package icylockedtreasury

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type IStore interface {
	All(db *gorm.DB) ([]*model.IcyLockedTreasury, error)
}
