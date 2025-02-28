package oracle

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type IcyOracle struct {
	db        *gorm.DB
	store     *store.Store
	appConfig *config.AppConfig
	logger    *logger.Logger
	btcRpc    btcrpc.IBtcRpc
	baseRpc   baserpc.IBaseRPC
	cache     *cache.Cache
	cacheMux  *sync.Mutex
}

// TODO: add other smaller packages if needed, e.g btcRPC or baseRPC
func New(db *gorm.DB, store *store.Store, appConfig *config.AppConfig, logger *logger.Logger, btcRpc btcrpc.IBtcRpc, baseRpc baserpc.IBaseRPC) IOracle {
	return &IcyOracle{
		db:        db,
		store:     store,
		appConfig: appConfig,
		logger:    logger,
		btcRpc:    btcRpc,
		baseRpc:   baseRpc,
		cache:     cache.New(5*time.Minute, 10*time.Minute),
		cacheMux:  &sync.Mutex{},
	}
}

func (o *IcyOracle) GetCirculatedICY() (*model.Web3BigInt, error) {
	icyTreasuries, err := o.store.IcyLockedTreasury.All(o.db)
	if err != nil {
		o.logger.Error("[GetCirculatedICY][ICyLockedTreasury]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	totalSupply, err := o.baseRpc.ICYTotalSupply()
	if err != nil {
		o.logger.Error("[GetCirculatedICY][ICYTotalSupply]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}
	sum := &model.Web3BigInt{
		Value:   "0",
		Decimal: 18,
	}
	for _, treasury := range icyTreasuries {
		balance, err := o.baseRpc.ICYBalanceOf(treasury.Address)
		if err != nil {
			o.logger.Error("[IcyOracle][GetCirculatedICY]", map[string]string{
				"error": err.Error(),
			})
			return nil, err
		}
		sum = sum.Add(balance)
	}

	return totalSupply.Sub(sum), nil
}

func (o *IcyOracle) GetBTCSupply() (*model.Web3BigInt, error) {
	btcBalance, err := o.btcRpc.CurrentBalance()
	if err != nil {
		o.logger.Error("[GetBTCSupply][CurrentBalance]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}
	return btcBalance, nil
}

func (o *IcyOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error) {
	o.cacheMux.Lock()
	defer o.cacheMux.Unlock()

	// If not in cache, calculate
	circulatedICY, err := o.GetCirculatedICY()
	if err != nil {
		o.logger.Error("[GetRealtimeICYBTC][GetCirculatedICY]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	btcSupply, err := o.GetBTCSupply()
	if err != nil {
		o.logger.Error("[GetRealtimeICYBTC][GetBTCSupply]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	icySatRate, err := getConversionRatio(circulatedICY, btcSupply)
	if err != nil {
		o.logger.Error("[GetRealtimeICYBTC][getConversionRatio]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	// Cache the new rate
	o.cache.Set("icysat_rate", icySatRate, cache.DefaultExpiration)

	return icySatRate, nil
}

func (o *IcyOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	// Try to get from cache first
	if cachedRate, found := o.cache.Get("icysat_rate"); found {
		if rate, ok := cachedRate.(*model.Web3BigInt); ok {
			return rate, nil
		}
	}

	return o.GetRealtimeICYBTC()
}
