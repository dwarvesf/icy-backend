package store

import "github.com/dwarvesf/icy-backend/internal/store/icylockedtreasury"

type Store struct {
	IcyLockedTreasury icylockedtreasury.IStore
}

func New() *Store {
	return &Store{
		IcyLockedTreasury: icylockedtreasury.New(),
	}
}
