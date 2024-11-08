package oracle

import (
	"sync"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type IcyOracle struct {
	mux *sync.Mutex

	cachedICYBTC *model.Web3BigInt

	appConfig *config.AppConfig
	logger    *logger.Logger
}

// TODO: add other smaller packages if needed, e.g btcRPC or baseRPC
func New(appConfig *config.AppConfig, logger *logger.Logger) IOracle {
	o := &IcyOracle{
		mux:       &sync.Mutex{},
		appConfig: appConfig,
		logger:    logger,
	}

	// go o.startUpdateCachedRealtimeICYBTC()

	return o
}

func (o *IcyOracle) GetCirculatedICY() (*model.Web3BigInt, error) {
	mockData := model.Web3BigInt{
		Value:   "100000000000000000000000000",
		Decimal: 18,
	}
	return &mockData, nil
}

func (o *IcyOracle) GetBTCSupply() (*model.Web3BigInt, error) {
	mockData := model.Web3BigInt{
		Value:   "100000000000000000000000000",
		Decimal: 18,
	}
	return &mockData, nil
}

func (o *IcyOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error) {
	mockData := model.Web3BigInt{
		Value:   "1500000000000000000",
		Decimal: 18,
	}
	return &mockData, nil
}

func (o *IcyOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	o.mux.Lock()
	defer o.mux.Unlock()
	mockData := model.Web3BigInt{
		Value:   "1500000000000000000",
		Decimal: 18,
	}
	return &mockData, nil
}

func (o *IcyOracle) refreshCachedRealtimeICYBTC() {
	o.mux.Lock()
	defer o.mux.Unlock()

	// o.cachedICYBTC = price
}
