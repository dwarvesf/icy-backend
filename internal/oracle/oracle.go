package oracle

import (
	"sync"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type IcyOracle struct {
	mux *sync.Mutex

	cachedICYBTC *model.Web3BigInt
	db           *gorm.DB
	store        *store.Store
	appConfig    *config.AppConfig
	logger       *logger.Logger
	btcRpc       btcrpc.IBtcRpc
	baseRpc      baserpc.IBaseRPC
}

// TODO: add other smaller packages if needed, e.g btcRPC or baseRPC
func New(db *gorm.DB, store *store.Store, appConfig *config.AppConfig, logger *logger.Logger, btcRpc btcrpc.IBtcRpc, baseRpc baserpc.IBaseRPC) IOracle {
	o := &IcyOracle{
		db:        db,
		store:     store,
		mux:       &sync.Mutex{},
		appConfig: appConfig,
		logger:    logger,
		btcRpc:    btcRpc,
		baseRpc:   baseRpc,
	}

	// go o.startUpdateCachedRealtimeICYBTC()

	return o
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

	icybtcRate, err := getConversionRatio(circulatedICY, btcSupply)
	if err != nil {
		o.logger.Error("[GetRealtimeICYBTC][getConversionRatio]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	o.updateCachedRealtimeICYBTC(icybtcRate)

	return icybtcRate, nil

}

func (o *IcyOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	o.mux.Lock()
	defer o.mux.Unlock()
	return o.cachedICYBTC, nil
}

func (o *IcyOracle) updateCachedRealtimeICYBTC(number *model.Web3BigInt) {
	o.mux.Lock()
	defer o.mux.Unlock()
	o.cachedICYBTC = number
}
