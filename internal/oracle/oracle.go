package oracle

import (
	"math"
	"math/big"
	"sync"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"gorm.io/gorm"
)

type IcyOracle struct {
	mux *sync.Mutex

	cachedICYBTC *model.Web3BigInt
	database     *gorm.DB
	store        *store.Store
	appConfig    *config.AppConfig
	logger       *logger.Logger
	btcRpc       btcrpc.IBtcRpc
	baseRpc      baserpc.IBaseRpc
}

// TODO: add other smaller packages if needed, e.g btcRPC or baseRPC
func New(database *gorm.DB, store *store.Store, appConfig *config.AppConfig, logger *logger.Logger, btcRpc btcrpc.IBtcRpc, baseRpc baserpc.IBaseRpc) IOracle {
	o := &IcyOracle{
		database:  database,
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
	icyTreasuries, err := o.store.IcyLockedTreasury.All(o.database)
	if err != nil {
		o.logger.Error("[IcyOracle][GetCirculatedICY]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	totalSupply, err := o.baseRpc.ICYTotalSupply()
	if err != nil {
		o.logger.Error("[IcyOracle][GetCirculatedICY]", map[string]string{
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
	btcBalance, err := o.btcRpc.BalanceOf(o.appConfig.Blockchain.BTCTreasuryAddress)
	if err != nil {
		o.logger.Error("[IcyOracle][GetBTCSupply]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}
	return btcBalance, nil
}

func (o *IcyOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error) {
	circulatedICY, err := o.GetCirculatedICY()
	if err != nil {
		o.logger.Error("[IcyOracle][GetRealtimeICYBTC]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	btcSupply, err := o.GetBTCSupply()
	if err != nil {
		o.logger.Error("[IcyOracle][GetRealtimeICYBTC]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	icyFloat := circulatedICY.ToFloat()
	btcFloat := btcSupply.ToFloat()

	if btcFloat == 0 {
		o.logger.Error("[IcyOracle][GetRealtimeICYBTC]", map[string]string{
			"error": "BTC supply is zero",
		})
		return &model.Web3BigInt{
			Value:   "0",
			Decimal: 18,
		}, nil
	}

	ratio := icyFloat / btcFloat

	ratioFloat := new(big.Float).SetFloat64(ratio)

	multiplier := new(big.Float).SetFloat64(math.Pow(10, 18))
	ratioFloat.Mul(ratioFloat, multiplier)

	ratioInt := new(big.Int)
	ratioFloat.Int(ratioInt)

	o.cachedICYBTC = &model.Web3BigInt{
		Value:   ratioInt.String(),
		Decimal: 18,
	}

	return &model.Web3BigInt{
		Value:   ratioInt.String(),
		Decimal: 18,
	}, nil

}

func (o *IcyOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	o.mux.Lock()
	defer o.mux.Unlock()
	return o.cachedICYBTC, nil
}
