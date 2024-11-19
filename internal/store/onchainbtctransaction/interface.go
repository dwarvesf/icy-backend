package onchainbtctransaction

import (
	"github.com/dwarvesf/icy-backend/internal/model"
	"gorm.io/gorm"
)

type IStore interface {
	Create(db *gorm.DB, onchainBtcTransaction *model.OnchainBtcTransaction) (*model.OnchainBtcTransaction, error)
}
