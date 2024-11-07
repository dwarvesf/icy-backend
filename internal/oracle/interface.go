package oracle

import "github.com/dwarvesf/icy-backend/internal/model"

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
}
