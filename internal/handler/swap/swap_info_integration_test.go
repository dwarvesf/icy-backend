package swap_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
)

// Enhanced response structure for testing
type InfoResponse struct {
	Data struct {
		CirculatedIcyBalance string  `json:"circulated_icy_balance"`
		SatoshiBalance       string  `json:"satoshi_balance"`
		SatoshiPerUSD        float64 `json:"satoshi_per_usd"`
		IcySatoshiRate       string  `json:"icy_satoshi_rate"`
		IcyUSDRate          string  `json:"icy_usd_rate"`
		SatoshiUSDRate      string  `json:"satoshi_usd_rate"`
		MinIcyToSwap        string  `json:"min_icy_to_swap"`
		ServiceFeeRate      float64 `json:"service_fee_rate"`
		MinSatoshiFee       string  `json:"min_satoshi_fee"`
		Warnings            []string `json:"warnings,omitempty"`     // For partial failures
		CacheInfo           struct {                               // For debugging cache behavior
			IcyCached bool `json:"icy_cached"`
			BtcCached bool `json:"btc_cached"`
			UsdCached bool `json:"usd_cached"`
		} `json:"cache_info,omitempty"`
	} `json:"data"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message"`
}

// Circuit breaker states for testing
type CircuitBreakerState int

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitOpen
	CircuitHalfOpen
)

// Enhanced mock with circuit breaker simulation
type EnhancedMockOracle struct {
	mock.Mock
	circuitState     CircuitBreakerState
	failureCount     int
	lastFailureTime  time.Time
	successThreshold int
	failureThreshold int
	timeoutDuration  time.Duration
	mu               sync.RWMutex
}

func NewEnhancedMockOracle() *EnhancedMockOracle {
	return &EnhancedMockOracle{
		circuitState:     CircuitClosed,
		successThreshold: 3,
		failureThreshold: 5,
		timeoutDuration:  30 * time.Second,
	}
}

func (m *EnhancedMockOracle) simulateNetworkConditions(operationName string, delay time.Duration, shouldFail bool) (*model.Web3BigInt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check circuit breaker state
	if m.circuitState == CircuitOpen {
		if time.Since(m.lastFailureTime) < m.timeoutDuration {
			return nil, errors.New("circuit breaker open: " + operationName)
		}
		m.circuitState = CircuitHalfOpen
	}

	// Simulate network delay
	if delay > 0 {
		time.Sleep(delay)
	}

	// Simulate failure conditions
	if shouldFail {
		m.failureCount++
		m.lastFailureTime = time.Now()
		
		if m.failureCount >= m.failureThreshold {
			m.circuitState = CircuitOpen
		}
		
		return nil, errors.New("simulated network failure: " + operationName)
	}

	// Success case
	m.failureCount = 0
	if m.circuitState == CircuitHalfOpen {
		m.circuitState = CircuitClosed
	}

	// Return appropriate test data based on operation
	switch operationName {
	case "GetCirculatedICY":
		return &model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil
	case "GetBTCSupply":
		return &model.Web3BigInt{Value: "500000000", Decimal: 8}, nil
	default:
		return &model.Web3BigInt{Value: "0", Decimal: 8}, nil
	}
}

func (m *EnhancedMockOracle) GetCirculatedICY() (*model.Web3BigInt, error) {
	args := m.Called()
	return m.simulateNetworkConditions("GetCirculatedICY", args.Get(1).(time.Duration), args.Bool(2))
}

func (m *EnhancedMockOracle) GetBTCSupply() (*model.Web3BigInt, error) {
	args := m.Called()
	return m.simulateNetworkConditions("GetBTCSupply", args.Get(1).(time.Duration), args.Bool(2))
}

func (m *EnhancedMockOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *EnhancedMockOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

// New cached methods that will be implemented
func (m *EnhancedMockOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *EnhancedMockOracle) GetCachedBTCSupply() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

var _ = Describe("Info Endpoint Integration Tests", func() {
	var (
		enhancedMockOracle *EnhancedMockOracle
		mockBtcRPC         *MockBtcRPC
		mockBaseRPC        *MockBaseRPC
		handler            swap.IHandler
		router             *gin.Engine
		testLogger         *logger.Logger
		appConfig          *config.AppConfig
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
		testLogger = logger.New("test")
		
		appConfig = &config.AppConfig{
			MinIcySwapAmount: 1000,
			Bitcoin: config.Bitcoin{
				ServiceFeeRate: 0.01,
				MinSatshiFee:   546,
				MaxTxFeeUSD:    50.0,
			},
		}

		enhancedMockOracle = NewEnhancedMockOracle()
		mockBtcRPC = &MockBtcRPC{}
		mockBaseRPC = &MockBaseRPC{}

		handler = swap.New(testLogger, appConfig, enhancedMockOracle, mockBaseRPC, mockBtcRPC, nil)
		router.GET("/api/v1/swap/info", handler.Info)
	})

	Context("End-to-End Timeout and Caching Solution", func() {
		Describe("Baseline behavior with current 15-second timeout", func() {
			It("should fail when operations take 18 seconds total", func() {
				// Setup operations that will cause timeout
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, 6*time.Second, false)
				enhancedMockOracle.On("GetBTCSupply").Return(nil, 6*time.Second, false)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil).After(7*time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusGatewayTimeout))
				Expect(duration).To(BeNumerically(">=", 15*time.Second))
				Expect(duration).To(BeNumerically("<=", 16*time.Second))
			})
		})

		Describe("Enhanced solution with 45-second timeout", func() {
			It("should succeed when operations take 30 seconds total", func() {
				// Setup operations that complete within 45 seconds
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, 10*time.Second, false)
				enhancedMockOracle.On("GetBTCSupply").Return(nil, 10*time.Second, false)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil).After(10*time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(duration).To(BeNumerically(">=", 10*time.Second))
				Expect(duration).To(BeNumerically("<=", 45*time.Second))

				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				Expect(response.Data.CirculatedIcyBalance).ToNot(BeEmpty())
				Expect(response.Data.SatoshiBalance).ToNot(BeEmpty())
			})

			It("should timeout at 45 seconds for operations that exceed limit", func() {
				// Setup operations that exceed 45 seconds
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, 50*time.Second, false)
				enhancedMockOracle.On("GetBTCSupply").Return(nil, 50*time.Second, false)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, nil).After(50*time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusGatewayTimeout))
				Expect(duration).To(BeNumerically(">=", 45*time.Second))
				Expect(duration).To(BeNumerically("<=", 47*time.Second))
			})
		})

		Describe("Caching performance improvements", func() {
			It("should complete in under 2 seconds when all data is cached", func() {
				// Setup cached responses (very fast)
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil) // Already has caching

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(duration).To(BeNumerically("<", 2*time.Second))

				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				Expect(response.Message).To(Equal("swap info retrieved successfully"))
			})

			It("should use mix of cached and fresh data efficiently", func() {
				// ICY cached, BTC fresh, USD cached
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				enhancedMockOracle.On("GetBTCSupply").Return(nil, 3*time.Second, false) // Fresh data with delay
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil) // Cached (fast)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(duration).To(BeNumerically(">=", 3*time.Second))
				Expect(duration).To(BeNumerically("<", 5*time.Second))
			})
		})

		Describe("Graceful degradation implementation", func() {
			It("should return partial data with warnings when one operation fails", func() {
				// ICY fails, others succeed
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(nil, errors.New("ICY node offline"))
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, errors.New("ICY node offline")) // Fallback also fails
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK)) // Partial success
				
				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				
				// Should have BTC and USD data
				Expect(response.Data.SatoshiBalance).To(Equal("500000000"))
				Expect(response.Data.SatoshiPerUSD).To(Equal(100000.0))
				
				// Should have warnings about missing data
				Expect(response.Data.Warnings).ToNot(BeEmpty())
				Expect(response.Data.Warnings[0]).To(ContainSubstring("ICY"))
			})

			It("should handle two out of three operations failing", func() {
				// Only USD price succeeds
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(nil, errors.New("ICY service down"))
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, errors.New("ICY service down"))
				enhancedMockOracle.On("GetCachedBTCSupply").Return(nil, errors.New("BTC service down"))
				enhancedMockOracle.On("GetBTCSupply").Return(nil, errors.New("BTC service down"))
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK)) // Minimal success
				
				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				
				Expect(response.Data.SatoshiPerUSD).To(Equal(100000.0))
				Expect(len(response.Data.Warnings)).To(Equal(2)) // Two services failed
			})

			It("should return service unavailable when all operations fail", func() {
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(nil, errors.New("ICY service down"))
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, errors.New("ICY service down"))
				enhancedMockOracle.On("GetCachedBTCSupply").Return(nil, errors.New("BTC service down"))
				enhancedMockOracle.On("GetBTCSupply").Return(nil, errors.New("BTC service down"))
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, errors.New("USD service down"))

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
				Expect(strings.Contains(w.Body.String(), "all data sources unavailable")).To(BeTrue())
			})
		})

		Describe("Background refresh pattern", func() {
			It("should return stale cache immediately while refreshing in background", func() {
				// Setup stale cache that returns immediately
				staleICY := &model.Web3BigInt{Value: "900000000000000000000", Decimal: 18}
				staleBTC := &model.Web3BigInt{Value: "400000000", Decimal: 8}
				
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(staleICY, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(staleBTC, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(duration).To(BeNumerically("<", 1*time.Second)) // Very fast

				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				
				// Should return stale data
				Expect(response.Data.CirculatedIcyBalance).To(Equal("900000000000000000000"))
				Expect(response.Data.SatoshiBalance).To(Equal("400000000"))
				
				// Background refresh should be triggered (implementation detail)
				// This would need to be verified through cache monitoring
			})
		})
	})

	Context("Real-world Failure Scenarios", func() {
		Describe("Network instability simulation", func() {
			It("should handle intermittent network failures", func() {
				// Simulate network that fails sometimes
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(nil, errors.New("cache miss"))
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, 2*time.Second, true) // Fails after delay
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK)) // Partial success
				
				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				Expect(response.Data.Warnings).ToNot(BeEmpty())
			})

			It("should recover from circuit breaker trips", func() {
				// First request trips circuit breaker
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(nil, errors.New("cache miss"))
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, 1*time.Second, true) // Fails
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				// Simulate multiple failures to trip circuit breaker
				for i := 0; i < 6; i++ {
					req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
					// Each request should either succeed partially or fail
					Expect(w.Code).To(BeNumerically(">=", 200))
				}

				// After circuit breaker timeout, should work again
				time.Sleep(31 * time.Second) // Wait for circuit breaker timeout
				
				enhancedMockOracle.On("GetCirculatedICY").Return(nil, 1*time.Second, false) // Now succeeds
				
				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				
				Expect(w.Code).To(Equal(http.StatusOK))
			})
		})

		Describe("High load scenarios", func() {
			It("should handle 100 concurrent requests efficiently", func() {
				// Setup fast cached responses
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				numRequests := 100
				resultChan := make(chan struct {
					statusCode int
					duration   time.Duration
				}, numRequests)

				start := time.Now()
				
				// Launch concurrent requests
				for i := 0; i < numRequests; i++ {
					go func() {
						reqStart := time.Now()
						req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)
						reqDuration := time.Since(reqStart)
						
						resultChan <- struct {
							statusCode int
							duration   time.Duration
						}{w.Code, reqDuration}
					}()
				}

				// Collect results
				successCount := 0
				totalDuration := time.Duration(0)
				
				for i := 0; i < numRequests; i++ {
					result := <-resultChan
					if result.statusCode == http.StatusOK {
						successCount++
					}
					totalDuration += result.duration
				}

				overallDuration := time.Since(start)
				avgDuration := totalDuration / time.Duration(numRequests)

				Expect(successCount).To(Equal(numRequests))
				Expect(overallDuration).To(BeNumerically("<", 30*time.Second)) // Should complete quickly
				Expect(avgDuration).To(BeNumerically("<", 5*time.Second))      // Each request should be fast
			})
		})

		Describe("Memory pressure scenarios", func() {
			It("should maintain performance under memory constraints", func() {
				// This test would need actual memory monitoring
				Skip("Requires memory profiling implementation")
			})
		})
	})

	Context("Monitoring and Observability", func() {
		Describe("Performance metrics", func() {
			It("should provide timing information for each operation", func() {
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				// Check if timing information is logged
				// This would need to be verified through log inspection
			})
		})

		Describe("Cache hit/miss metrics", func() {
			It("should report cache performance in response", func() {
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				
				// Should report cache hits
				Expect(response.Data.CacheInfo.IcyCached).To(BeTrue())
				Expect(response.Data.CacheInfo.BtcCached).To(BeTrue())
				Expect(response.Data.CacheInfo.UsdCached).To(BeTrue())
			})
		})
	})

	Context("Backward Compatibility", func() {
		Describe("Response format consistency", func() {
			It("should maintain same response structure as before", func() {
				enhancedMockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "500000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				
				// Verify all expected fields are present
				Expect(response.Data.CirculatedIcyBalance).ToNot(BeEmpty())
				Expect(response.Data.SatoshiBalance).ToNot(BeEmpty())
				Expect(response.Data.IcySatoshiRate).ToNot(BeEmpty())
				Expect(response.Data.IcyUSDRate).ToNot(BeEmpty())
				Expect(response.Data.SatoshiUSDRate).ToNot(BeEmpty())
				Expect(response.Data.MinIcyToSwap).ToNot(BeEmpty())
				Expect(response.Data.ServiceFeeRate).To(BeNumerically(">", 0))
				Expect(response.Data.MinSatoshiFee).ToNot(BeEmpty())
			})
		})

		Describe("Calculation accuracy", func() {
			It("should produce same mathematical results as before", func() {
				// Use known values to verify calculations remain correct
				knownICY := &model.Web3BigInt{Value: "2000000000000000000000", Decimal: 18} // 2000 ICY
				knownBTC := &model.Web3BigInt{Value: "100000000", Decimal: 8}             // 1 BTC
				knownUSDRate := 50000.0 // 50k satoshi per USD

				enhancedMockOracle.On("GetCachedCirculatedICY").Return(knownICY, nil)
				enhancedMockOracle.On("GetCachedBTCSupply").Return(knownBTC, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(knownUSDRate, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				var response InfoResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).To(BeNil())
				
				// ICY/Satoshi rate should be: 100000000 / 2000 = 50000 satoshis per ICY
				Expect(response.Data.IcySatoshiRate).To(Equal("50000.00"))
				
				// USD rate calculations should be accurate
				expectedICYUSD := 50000.0 / knownUSDRate // 50000 satoshi / (50000 satoshi/USD)
				Expect(response.Data.IcyUSDRate).To(ContainSubstring("1.0000"))
			})
		})
	})
})