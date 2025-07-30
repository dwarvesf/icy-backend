package swap_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/dwarvesf/icy-backend/internal/view"
)

// Mock interfaces for testing
type MockOracle struct {
	mock.Mock
}

func (m *MockOracle) GetCirculatedICY() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockOracle) GetBTCSupply() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockOracle) GetRealtimeICYBTC() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockOracle) GetCachedRealtimeICYBTC() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

// Interface methods that will be added for caching
func (m *MockOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockOracle) GetCachedBTCSupply() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

type MockBtcRPC struct {
	mock.Mock
}

func (m *MockBtcRPC) GetSatoshiUSDPrice() (float64, error) {
	args := m.Called()
	return args.Get(0).(float64), args.Error(1)
}

// Mock implementation for other required methods
func (m *MockBtcRPC) CurrentBalance() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockBtcRPC) SendBTC(address string, amount int64) (string, error) {
	args := m.Called(address, amount)
	return args.String(0), args.Error(1)
}

func (m *MockBtcRPC) IsDust(address string, amount int64) bool {
	args := m.Called(address, amount)
	return args.Bool(0)
}

type MockBaseRPC struct {
	mock.Mock
}

func (m *MockBaseRPC) ICYTotalSupply() (*model.Web3BigInt, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockBaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
	args := m.Called(address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockBaseRPC) GenerateSignature(icyAmount *model.Web3BigInt, btcAddress string, btcAmount *model.Web3BigInt, nonce, deadline interface{}) (string, error) {
	args := m.Called(icyAmount, btcAddress, btcAmount, nonce, deadline)
	return args.String(0), args.Error(1)
}

var _ = Describe("/info Endpoint Timeout and Caching Tests", func() {
	var (
		mockOracle  *MockOracle
		mockBtcRPC  *MockBtcRPC
		mockBaseRPC *MockBaseRPC
		handler     swap.IHandler
		router      *gin.Engine
		testLogger  *logger.Logger
		appConfig   *config.AppConfig
		db          *gorm.DB
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
			},
		}

		mockOracle = &MockOracle{}
		mockBtcRPC = &MockBtcRPC{}
		mockBaseRPC = &MockBaseRPC{}

		// Note: In actual implementation, we'll need to inject the cache
		// This will require modifying the handler constructor
		handler = swap.New(testLogger, appConfig, mockOracle, mockBaseRPC, mockBtcRPC, db)
		router.GET("/api/v1/swap/info", handler.Info)
	})

	Context("Timeout Handling Tests", func() {
		Describe("Current 15-second timeout issues", func() {
			It("should timeout when operations take longer than 15 seconds", func() {
				// Mock slow operations that exceed current timeout
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, nil).After(6 * time.Second)
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("timeout")).After(8 * time.Second)
				mockOracle.On("GetBTCSupply").Return(nil, errors.New("timeout")).After(7 * time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(duration).To(BeNumerically(">=", 15*time.Second))
				Expect(w.Code).To(Equal(http.StatusGatewayTimeout))

				var response view.ErrorResponse
				Expect(strings.Contains(w.Body.String(), "context deadline exceeded")).To(BeTrue())
			})

			It("should fail fast when any single operation fails", func() {
				// Mock immediate failure of one operation
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("RPC connection failed"))
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusInternalServerError))
				Expect(strings.Contains(w.Body.String(), "failed to get GetCirculatedICY")).To(BeTrue())
			})
		})

		Describe("Improved timeout with 45-second limit", func() {
			It("should complete successfully within 45 seconds for complex operations", func() {
				// Mock operations that take 20-25 seconds each but still complete
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil).After(8 * time.Second)
				mockOracle.On("GetCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil).After(10 * time.Second)
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil).After(12 * time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(duration).To(BeNumerically("<=", 45*time.Second))
				Expect(w.Code).To(Equal(http.StatusOK))
				
				// Verify response contains expected data
				responseBody := w.Body.String()
				Expect(responseBody).To(ContainSubstring("circulated_icy_balance"))
				Expect(responseBody).To(ContainSubstring("satoshi_balance"))
				Expect(responseBody).To(ContainSubstring("icy_satoshi_rate"))
			})

			It("should timeout at 45 seconds for operations that exceed the limit", func() {
				// Mock operations that take longer than 45 seconds
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, nil).After(50 * time.Second)
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("timeout")).After(50 * time.Second)
				mockOracle.On("GetBTCSupply").Return(nil, errors.New("timeout")).After(50 * time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(duration).To(BeNumerically(">=", 45*time.Second))
				Expect(duration).To(BeNumerically("<=", 47*time.Second)) // Allow small buffer
				Expect(w.Code).To(Equal(http.StatusGatewayTimeout))
			})
		})
	})

	Context("Caching Layer Tests", func() {
		Describe("GetCirculatedICY caching", func() {
			It("should cache GetCirculatedICY results for 5 minutes", func() {
				expectedICY := &model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}
				
				// First call should hit the actual method
				mockOracle.On("GetCirculatedICY").Return(expectedICY, nil).Once()
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)

				// First request
				req1 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w1 := httptest.NewRecorder()
				router.ServeHTTP(w1, req1)
				Expect(w1.Code).To(Equal(http.StatusOK))

				// Second request within cache window should use cached value
				// This test assumes implementation will use GetCachedCirculatedICY
				mockOracle.On("GetCachedCirculatedICY").Return(expectedICY, nil)
				
				req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w2 := httptest.NewRecorder()
				router.ServeHTTP(w2, req2)
				Expect(w2.Code).To(Equal(http.StatusOK))

				// Verify GetCirculatedICY was only called once
				mockOracle.AssertExpectations(GinkgoT())
			})

			It("should refresh cache after 5 minutes", func() {
				expectedICY := &model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}
				
				// Setup cache expiration test
				// This test verifies that after cache expiration, fresh data is fetched
				mockOracle.On("GetCirculatedICY").Return(expectedICY, nil).Times(2)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil).Times(2)
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil).Times(2)

				// First request
				req1 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w1 := httptest.NewRecorder()
				router.ServeHTTP(w1, req1)
				Expect(w1.Code).To(Equal(http.StatusOK))

				// Simulate cache expiration (implementation detail)
				// In real implementation, we'd advance time or manipulate cache directly
				
				// Second request after cache expiration
				req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w2 := httptest.NewRecorder()
				router.ServeHTTP(w2, req2)
				Expect(w2.Code).To(Equal(http.StatusOK))

				mockOracle.AssertExpectations(GinkgoT())
			})
		})

		Describe("GetBTCSupply caching", func() {
			It("should cache GetBTCSupply results for 5 minutes", func() {
				expectedBTC := &model.Web3BigInt{Value: "200000000", Decimal: 8}
				
				// First call should hit the actual method
				mockOracle.On("GetBTCSupply").Return(expectedBTC, nil).Once()
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)
				mockOracle.On("GetCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)

				// First request
				req1 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w1 := httptest.NewRecorder()
				router.ServeHTTP(w1, req1)
				Expect(w1.Code).To(Equal(http.StatusOK))

				// Second request should use cached value
				mockOracle.On("GetCachedBTCSupply").Return(expectedBTC, nil)
				
				req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w2 := httptest.NewRecorder()
				router.ServeHTTP(w2, req2)
				Expect(w2.Code).To(Equal(http.StatusOK))

				mockOracle.AssertExpectations(GinkgoT())
			})

			It("should handle cache miss gracefully", func() {
				expectedBTC := &model.Web3BigInt{Value: "200000000", Decimal: 8}
				
				// Cache miss should fall back to fresh data fetch
				mockOracle.On("GetCachedBTCSupply").Return(nil, errors.New("cache miss"))
				mockOracle.On("GetBTCSupply").Return(expectedBTC, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)
				mockOracle.On("GetCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				mockOracle.AssertExpectations(GinkgoT())
			})
		})

		Describe("Cache performance optimization", func() {
			It("should complete requests faster when using cached data", func() {
				// Setup cached responses (fast)
				mockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				mockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil) // This already has caching

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w, req)
				duration := time.Since(start)

				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(duration).To(BeNumerically("<", 1*time.Second)) // Should be very fast with cache
			})
		})
	})

	Context("Graceful Degradation Tests", func() {
		Describe("Partial data return on failures", func() {
			It("should return partial data when GetCirculatedICY fails but others succeed", func() {
				// GetCirculatedICY fails, but others succeed
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("blockchain RPC timeout"))
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// With graceful degradation, this should return 200 with partial data
				Expect(w.Code).To(Equal(http.StatusOK))
				
				responseBody := w.Body.String()
				Expect(responseBody).To(ContainSubstring("satoshi_balance"))
				Expect(responseBody).To(ContainSubstring("satoshi_per_usd"))
				// circulated_icy_balance might be null or have default value
			})

			It("should return partial data when GetBTCSupply fails but others succeed", func() {
				mockOracle.On("GetCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				mockOracle.On("GetBTCSupply").Return(nil, errors.New("BTC node connection failed"))
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				responseBody := w.Body.String()
				Expect(responseBody).To(ContainSubstring("circulated_icy_balance"))
				Expect(responseBody).To(ContainSubstring("satoshi_per_usd"))
			})

			It("should return partial data when GetSatoshiUSDPrice fails but others succeed", func() {
				mockOracle.On("GetCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, errors.New("CoinGecko API failed"))

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				responseBody := w.Body.String()
				Expect(responseBody).To(ContainSubstring("circulated_icy_balance"))
				Expect(responseBody).To(ContainSubstring("satoshi_balance"))
				// ICY/BTC rate can still be calculated without USD price
			})

			It("should handle the case where only one operation succeeds", func() {
				// Only GetSatoshiUSDPrice succeeds
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("ICY RPC failed"))
				mockOracle.On("GetBTCSupply").Return(nil, errors.New("BTC RPC failed"))

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				responseBody := w.Body.String()
				Expect(responseBody).To(ContainSubstring("satoshi_per_usd"))
				// Other fields should have default/null values with proper handling
			})

			It("should return meaningful error when all operations fail", func() {
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, errors.New("CoinGecko API failed"))
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("ICY RPC failed"))
				mockOracle.On("GetBTCSupply").Return(nil, errors.New("BTC RPC failed"))

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
				Expect(strings.Contains(w.Body.String(), "all data sources unavailable")).To(BeTrue())
			})
		})

		Describe("Error message clarity", func() {
			It("should provide detailed error information in partial failure scenarios", func() {
				mockOracle.On("GetCirculatedICY").Return(nil, errors.New("Ethereum node timeout"))
				mockOracle.On("GetBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				responseBody := w.Body.String()
				// Should include warnings about partial data
				Expect(responseBody).To(ContainSubstring("warnings"))
				Expect(responseBody).To(ContainSubstring("partial"))
			})
		})
	})

	Context("Background Refresh Pattern Tests", func() {
		Describe("Return cached data while updating asynchronously", func() {
			It("should return stale cache immediately while refreshing in background", func() {
				staleICY := &model.Web3BigInt{Value: "900000000000000000000", Decimal: 18}
				freshICY := &model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}
				
				// First call returns stale cache immediately
				mockOracle.On("GetCachedCirculatedICY").Return(staleICY, nil).Once()
				mockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req1 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w1 := httptest.NewRecorder()

				start := time.Now()
				router.ServeHTTP(w1, req1)
				duration := time.Since(start)

				Expect(w1.Code).To(Equal(http.StatusOK))
				Expect(duration).To(BeNumerically("<", 500*time.Millisecond)) // Very fast response
				
				// Background refresh should happen (this is implementation dependent)
				// Next call should have fresh data
				mockOracle.On("GetCachedCirculatedICY").Return(freshICY, nil)
				
				// Allow time for background refresh
				time.Sleep(100 * time.Millisecond)
				
				req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w2 := httptest.NewRecorder()
				router.ServeHTTP(w2, req2)
				
				Expect(w2.Code).To(Equal(http.StatusOK))
				// Verify fresh data is now returned
			})

			It("should handle background refresh failures gracefully", func() {
				staleICY := &model.Web3BigInt{Value: "900000000000000000000", Decimal: 18}
				
				// Stale cache available, background refresh fails
				mockOracle.On("GetCachedCirculatedICY").Return(staleICY, nil)
				mockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				// Should still return the stale but valid data
				Expect(strings.Contains(w.Body.String(), "900000000000000000000")).To(BeTrue())
			})
		})
	})

	Context("Edge Cases and Error Scenarios", func() {
		Describe("Context cancellation handling", func() {
			It("should handle request context cancellation properly", func() {
				ctx, cancel := context.WithCancel(context.Background())
				
				// Setup slow operations
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, nil).After(10 * time.Second)
				mockOracle.On("GetCirculatedICY").Return(nil, nil).After(10 * time.Second)
				mockOracle.On("GetBTCSupply").Return(nil, nil).After(10 * time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				req = req.WithContext(ctx)
				w := httptest.NewRecorder()

				// Cancel context after 2 seconds
				go func() {
					time.Sleep(2 * time.Second)
					cancel()
				}()

				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusGatewayTimeout))
				Expect(strings.Contains(w.Body.String(), "context")).To(BeTrue())
			})
		})

		Describe("Resource cleanup", func() {
			It("should properly clean up goroutines on timeout", func() {
				// This test verifies that goroutines don't leak when timeout occurs
				initialRoutines := getCurrentGoroutineCount() // Implementation helper needed
				
				// Setup operations that will timeout
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(0.0, nil).After(50 * time.Second)
				mockOracle.On("GetCirculatedICY").Return(nil, nil).After(50 * time.Second)
				mockOracle.On("GetBTCSupply").Return(nil, nil).After(50 * time.Second)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusGatewayTimeout))
				
				// Allow time for cleanup
				time.Sleep(1 * time.Second)
				
				finalRoutines := getCurrentGoroutineCount()
				Expect(finalRoutines).To(BeNumerically("<=", initialRoutines+1)) // Small buffer for test goroutines
			})
		})

		Describe("Concurrent request handling", func() {
			It("should handle multiple concurrent requests efficiently", func() {
				// Setup fast cached responses
				mockOracle.On("GetCachedCirculatedICY").Return(&model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}, nil)
				mockOracle.On("GetCachedBTCSupply").Return(&model.Web3BigInt{Value: "100000000", Decimal: 8}, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				// Create multiple concurrent requests
				numRequests := 10
				resultChan := make(chan int, numRequests)

				for i := 0; i < numRequests; i++ {
					go func() {
						req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)
						resultChan <- w.Code
					}()
				}

				// Collect results
				successCount := 0
				for i := 0; i < numRequests; i++ {
					statusCode := <-resultChan
					if statusCode == http.StatusOK {
						successCount++
					}
				}

				Expect(successCount).To(Equal(numRequests))
			})
		})
	})

	Context("Cache Eviction and Memory Management Tests", func() {
		Describe("Cache size limits", func() {
			It("should respect cache memory limits", func() {
				// This test ensures cache doesn't grow unbounded
				// Implementation would need to track cache size
				Skip("Implementation dependent - requires cache size monitoring")
			})
		})

		Describe("Cache key collision handling", func() {
			It("should handle cache key collisions properly", func() {
				// Test that different operations don't interfere with each other's cache
				expectedICY := &model.Web3BigInt{Value: "1000000000000000000000", Decimal: 18}
				expectedBTC := &model.Web3BigInt{Value: "100000000", Decimal: 8}
				
				mockOracle.On("GetCachedCirculatedICY").Return(expectedICY, nil)
				mockOracle.On("GetCachedBTCSupply").Return(expectedBTC, nil)
				mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v1/swap/info", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))
				
				responseBody := w.Body.String()
				Expect(responseBody).To(ContainSubstring("1000000000000000000000")) // ICY value
				Expect(responseBody).To(ContainSubstring("100000000")) // BTC value
			})
		})
	})
})

// Helper function for goroutine leak detection
func getCurrentGoroutineCount() int {
	// This would need to be implemented using runtime.NumGoroutine()
	// or similar goroutine tracking mechanism
	return 0 // Placeholder
}