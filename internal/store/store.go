package store

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/store/icylockedtreasury"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtctransaction"
	"github.com/dwarvesf/icy-backend/internal/store/onchainicytransaction"
	"github.com/dwarvesf/icy-backend/internal/store/swaprequest"
)

type Store struct {
	IcyLockedTreasury              icylockedtreasury.IStore
	OnchainBtcTransaction          onchainbtctransaction.IStore
	OnchainIcyTransaction          onchainicytransaction.IStore
	OnchainBtcProcessedTransaction onchainbtcprocessedtransaction.IStore
	SwapRequest                    swaprequest.IStore
}

func New(db *gorm.DB) *Store {
	return &Store{
		IcyLockedTreasury:              icylockedtreasury.New(),
		OnchainBtcTransaction:          onchainbtctransaction.New(),
		OnchainIcyTransaction:          onchainicytransaction.New(),
		OnchainBtcProcessedTransaction: onchainbtcprocessedtransaction.New(),
		SwapRequest:                    swaprequest.New(),
	}
}
