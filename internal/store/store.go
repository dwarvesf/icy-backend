package store

import (
	"github.com/dwarvesf/icy-backend/internal/store/icylockedtreasury"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtctransaction"
	"github.com/dwarvesf/icy-backend/internal/store/onchainicytransaction"
)

type Store struct {
	IcyLockedTreasury     icylockedtreasury.IStore
	OnchainBtcTransaction onchainbtctransaction.IStore
	OnchainIcyTransaction onchainicytransaction.IStore
}

func New() *Store {
	return &Store{
		IcyLockedTreasury:     icylockedtreasury.New(),
		OnchainBtcTransaction: onchainbtctransaction.New(),
		OnchainIcyTransaction: onchainicytransaction.New(),
	}
}
