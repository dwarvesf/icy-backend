package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
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
	// Cache statistics (atomic for thread safety)
	circulatedICYHits   int64
	circulatedICYMisses int64
	btcSupplyHits       int64
	btcSupplyMisses     int64
	lastRefresh         int64 // Unix timestamp
}

// TODO: add other smaller packages if needed, e.g btcRPC or baseRPC
func New(db *gorm.DB, store *store.Store, appConfig *config.AppConfig, logger *logger.Logger, btcRpc btcrpc.IBtcRpc, baseRpc baserpc.IBaseRPC) IOracle {
	return &IcyOracle{
		db:          db,
		store:       store,
		appConfig:   appConfig,
		logger:      logger,
		btcRpc:      btcRpc,
		baseRpc:     baseRpc,
		cache:       cache.New(5*time.Minute, 10*time.Minute), // 5-minute cache, 10-minute cleanup
		cacheMux:    &sync.Mutex{},
		lastRefresh: time.Now().Unix(),
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
		mochiPayICY = &model.Web3BigInt{
			Value:   "0",
			Decimal: 18,
		}
	}

	return totalSupply.Sub(sum).Add(mochiPayICY), nil
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

// Enhanced caching methods for timeout handling

func (o *IcyOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) {
	// Try to get from cache first
	if cached, found := o.cache.Get("circulated_icy"); found {
		atomic.AddInt64(&o.circulatedICYHits, 1)
		if balance, ok := cached.(*model.Web3BigInt); ok {
			o.logger.Info("[GetCachedCirculatedICY] cache hit", nil)
			return balance, nil
		}
	}

	// Cache miss - get fresh data
	atomic.AddInt64(&o.circulatedICYMisses, 1)
	o.logger.Info("[GetCachedCirculatedICY] cache miss, fetching fresh data", nil)
	
	balance, err := o.GetCirculatedICY()
	if err != nil {
		return nil, err
	}

	// Cache the result for 5 minutes
	o.cache.Set("circulated_icy", balance, cache.DefaultExpiration)
	atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())
	
	return balance, nil
}

func (o *IcyOracle) GetCachedBTCSupply() (*model.Web3BigInt, error) {
	// Try to get from cache first
	if cached, found := o.cache.Get("btc_supply"); found {
		atomic.AddInt64(&o.btcSupplyHits, 1)
		if balance, ok := cached.(*model.Web3BigInt); ok {
			o.logger.Info("[GetCachedBTCSupply] cache hit", nil)
			return balance, nil
		}
	}

	// Cache miss - get fresh data
	atomic.AddInt64(&o.btcSupplyMisses, 1)
	o.logger.Info("[GetCachedBTCSupply] cache miss, fetching fresh data", nil)

	balance, err := o.GetBTCSupply()
	if err != nil {
		return nil, err
	}

	// Cache the result for 5 minutes
	o.cache.Set("btc_supply", balance, cache.DefaultExpiration)
	atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())

	return balance, nil
}

func (o *IcyOracle) GetCirculatedICYWithContext(ctx context.Context) (*model.Web3BigInt, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try cached version first
	if cached, found := o.cache.Get("circulated_icy"); found {
		if balance, ok := cached.(*model.Web3BigInt); ok {
			return balance, nil
		}
	}

	// Use a channel to handle the operation with context
	resultCh := make(chan struct {
		balance *model.Web3BigInt
		err     error
	}, 1)

	go func() {
		balance, err := o.GetCirculatedICY()
		resultCh <- struct {
			balance *model.Web3BigInt
			err     error
		}{balance, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		// Cache the result
		o.cache.Set("circulated_icy", result.balance, cache.DefaultExpiration)
		atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())
		return result.balance, nil
	}
}

func (o *IcyOracle) GetBTCSupplyWithContext(ctx context.Context) (*model.Web3BigInt, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try cached version first
	if cached, found := o.cache.Get("btc_supply"); found {
		if balance, ok := cached.(*model.Web3BigInt); ok {
			return balance, nil
		}
	}

	// Use a channel to handle the operation with context
	resultCh := make(chan struct {
		balance *model.Web3BigInt
		err     error
	}, 1)

	go func() {
		balance, err := o.GetBTCSupply()
		resultCh <- struct {
			balance *model.Web3BigInt
			err     error
		}{balance, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		// Cache the result
		o.cache.Set("btc_supply", result.balance, cache.DefaultExpiration)
		atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())
		return result.balance, nil
	}
}

func (o *IcyOracle) RefreshCirculatedICYAsync() error {
	go func() {
		balance, err := o.GetCirculatedICY()
		if err != nil {
			o.logger.Error("[RefreshCirculatedICYAsync] failed to refresh", map[string]string{
				"error": err.Error(),
			})
			return
		}
		
		o.cache.Set("circulated_icy", balance, cache.DefaultExpiration)
		atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())
		o.logger.Info("[RefreshCirculatedICYAsync] cache refreshed successfully", nil)
	}()
	
	return nil
}

func (o *IcyOracle) RefreshBTCSupplyAsync() error {
	go func() {
		balance, err := o.GetBTCSupply()
		if err != nil {
			o.logger.Error("[RefreshBTCSupplyAsync] failed to refresh", map[string]string{
				"error": err.Error(),
			})
			return
		}
		
		o.cache.Set("btc_supply", balance, cache.DefaultExpiration)
		atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())
		o.logger.Info("[RefreshBTCSupplyAsync] cache refreshed successfully", nil)
	}()
	
	return nil
}

func (o *IcyOracle) ClearAllCaches() error {
	o.cache.Flush()
	
	// Reset statistics
	atomic.StoreInt64(&o.circulatedICYHits, 0)
	atomic.StoreInt64(&o.circulatedICYMisses, 0)
	atomic.StoreInt64(&o.btcSupplyHits, 0)
	atomic.StoreInt64(&o.btcSupplyMisses, 0)
	atomic.StoreInt64(&o.lastRefresh, time.Now().Unix())
	
	o.logger.Info("[ClearAllCaches] all caches cleared", nil)
	return nil
}

func (o *IcyOracle) GetCacheStatistics() *CacheStatistics {
	return &CacheStatistics{
		CirculatedICYHits:   atomic.LoadInt64(&o.circulatedICYHits),
		CirculatedICYMisses: atomic.LoadInt64(&o.circulatedICYMisses),
		BTCSupplyHits:       atomic.LoadInt64(&o.btcSupplyHits),
		BTCSupplyMisses:     atomic.LoadInt64(&o.btcSupplyMisses),
		LastRefresh:         time.Unix(atomic.LoadInt64(&o.lastRefresh), 0),
	}
}
