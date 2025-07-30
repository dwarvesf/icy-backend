package oracle

import (
	"context"
	"time"

	"github.com/dwarvesf/icy-backend/internal/model"
)

type CacheStatistics struct {
	CirculatedICYHits   int64     `json:"circulated_icy_hits"`
	CirculatedICYMisses int64     `json:"circulated_icy_misses"`
	BTCSupplyHits       int64     `json:"btc_supply_hits"`
	BTCSupplyMisses     int64     `json:"btc_supply_misses"`
	LastRefresh         time.Time `json:"last_refresh"`
}

type IOracle interface {
	// GetCirculatedICY returns the number of circulated ICY
	// excludes the ICY that is locked in the treasury
	GetCirculatedICY() (*model.Web3BigInt, error)

	// GetBTCSupply returns the total supply of BTC in treasury wallet
	GetBTCSupply() (*model.Web3BigInt, error)

	// GetRealtimeICYBTC returns the realtime ICY/BTC price
	GetRealtimeICYBTC() (*model.Web3BigInt, error)

	// GetCachedRealtimeICYBTC returns the cached realtime ICY/BTC price
	GetCachedRealtimeICYBTC() (*model.Web3BigInt, error)

	// Enhanced caching methods for timeout handling
	GetCachedCirculatedICY() (*model.Web3BigInt, error)
	GetCachedBTCSupply() (*model.Web3BigInt, error)
	GetCirculatedICYWithContext(ctx context.Context) (*model.Web3BigInt, error)
	GetBTCSupplyWithContext(ctx context.Context) (*model.Web3BigInt, error)
	RefreshCirculatedICYAsync() error
	RefreshBTCSupplyAsync() error
	ClearAllCaches() error
	GetCacheStatistics() *CacheStatistics
}
