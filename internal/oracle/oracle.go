package oracle

import (
	"encoding/json"
	"fmt"
	"net/http"
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
		cache:     cache.New(1*time.Minute, 2*time.Minute),
		cacheMux:  &sync.Mutex{},
	}
}

// MochiPayResponse represents the response structure from the Mochi Pay API
type MochiPayResponse struct {
	Data struct {
		Icy struct {
			Value   string `json:"value"`
			Decimal int    `json:"decimal"`
			ChainID string `json:"chain_id"`
		} `json:"icy"`
	} `json:"data"`
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

	// Fetch circulated ICY from Mochi Pay API
	mochiPayICY, err := o.getMochiPayCirculatedICY()
	if err != nil {
		o.logger.Error("[GetCirculatedICY][getMochiPayCirculatedICY]", map[string]string{
			"error": err.Error(),
		})
		// Continue with calculation even if Mochi Pay API fails
	} else if mochiPayICY != nil {
		// Add Mochi Pay ICY to the sum of locked treasuries
		sum = sum.Add(mochiPayICY)
	}

	return totalSupply.Sub(sum), nil
}

// getMochiPayCirculatedICY fetches the circulated ICY from the Mochi Pay API
func (o *IcyOracle) getMochiPayCirculatedICY() (*model.Web3BigInt, error) {
	// Make HTTP request to Mochi Pay API
	resp, err := http.Get(o.appConfig.MochiConfig.MochiPayAPIURL + "/api/v1/console-tokens/icy/circulated")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data from Mochi Pay API: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response status code is OK
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mochi Pay API returned non-OK status: %d", resp.StatusCode)
	}

	// Parse the response
	var mochiPayResp MochiPayResponse
	if err := json.NewDecoder(resp.Body).Decode(&mochiPayResp); err != nil {
		return nil, fmt.Errorf("failed to decode Mochi Pay API response: %w", err)
	}

	// Extract the ICY value
	icyValue := mochiPayResp.Data.Icy.Value
	icyDecimal := mochiPayResp.Data.Icy.Decimal

	// Create a Web3BigInt with the ICY value
	return &model.Web3BigInt{
		Value:   icyValue,
		Decimal: icyDecimal,
	}, nil
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
