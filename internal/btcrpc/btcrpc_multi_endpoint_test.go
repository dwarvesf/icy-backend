package btcrpc_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

var _ = Describe("BtcRpc Multi-Endpoint Support", func() {
	var (
		btcRpc      btcrpc.IBtcRpc
		testLogger  *logger.Logger
		mockServers []*httptest.Server
		appConfig   *config.AppConfig
	)

	BeforeEach(func() {
		testLogger = logger.New(environments.Test)
	})

	AfterEach(func() {
		// Clean up mock servers
		for _, server := range mockServers {
			if server != nil {
				server.Close()
			}
		}
		mockServers = nil
	})

	Context("Configuration and Initialization", func() {
		Describe("Single Endpoint Configuration (Backward Compatibility)", func() {
			It("should initialize successfully with single BlockstreamAPIURL", func() {
				server := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURL:  server.URL,
						BlockstreamAPIURLs: nil, // Not set
						WalletWIF:          "test-wif",
						MaxTxFeeUSD:        10.0,
						ServiceFeeRate:     0.01,
						MinSatshiFee:       1000,
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)
				Expect(btcRpc).NotTo(BeNil())

				// Should work with current balance call
				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})

			It("should prefer BlockstreamAPIURLs over single BlockstreamAPIURL when both are set", func() {
				primaryServer := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				fallbackServer := createMockBlockstreamServer(http.StatusInternalServerError, "")
				mockServers = append(mockServers, primaryServer, fallbackServer)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURL:  fallbackServer.URL, // Should be ignored
						BlockstreamAPIURLs: []string{primaryServer.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)
				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})
		})

		Describe("Multiple Endpoint Configuration", func() {
			It("should initialize successfully with multiple BlockstreamAPIURLs", func() {
				server1 := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				server2 := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)
				Expect(btcRpc).NotTo(BeNil())
			})

			It("should handle empty endpoint list gracefully", func() {
				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{},
						WalletWIF:          "test-wif",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)
				Expect(btcRpc).NotTo(BeNil())

				// Should fail when trying to make requests
				_, err := btcRpc.CurrentBalance()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no available endpoints"))
			})
		})
	})

	Context("Failover Behavior", func() {
		Describe("Primary Endpoint Failure", func() {
			It("should failover to secondary endpoint when primary fails", func() {
				failingServer := createMockBlockstreamServer(http.StatusInternalServerError, "Server error")
				workingServer := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, failingServer, workingServer)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{failingServer.URL, workingServer.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)
				
				// Should succeed using secondary endpoint
				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})

			It("should fail when all endpoints are unavailable", func() {
				server1 := createMockBlockstreamServer(http.StatusInternalServerError, "Server error")
				server2 := createMockBlockstreamServer(http.StatusServiceUnavailable, "Service unavailable")
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)
				
				_, err := btcRpc.CurrentBalance()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("all endpoints failed"))
			})
		})

		Describe("Endpoint Recovery", func() {
			It("should retry failed endpoints after recovery period", func() {
				var requestCount int
				var mu sync.Mutex

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					if count <= 2 {
						// First two requests fail
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte("Server error"))
					} else {
						// Subsequent requests succeed
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
					}
				}))
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
						WalletWIF:          "test-wif",
						EndpointRetryDelay: time.Second, // Short delay for testing
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)

				// First call should fail
				_, err1 := btcRpc.CurrentBalance()
				Expect(err1).To(HaveOccurred())

				// Wait for retry delay
				time.Sleep(1500 * time.Millisecond)

				// Second call should succeed
				balance, err2 := btcRpc.CurrentBalance()
				Expect(err2).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})
		})
	})

	Context("Circuit Breaker Functionality", func() {
		Describe("Circuit Breaker Opening", func() {
			It("should open circuit breaker after consecutive 429 errors", func() {
				var requestCount int
				var mu sync.Mutex

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					mu.Unlock()

					// Always return 429
					w.WriteHeader(http.StatusTooManyRequests)
					w.Write([]byte("Rate limit exceeded"))
				}))
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)

				// Make calls until circuit breaker opens
				_, err1 := btcRpc.GetTransactionsByAddress("test-address", "")
				Expect(err1).To(HaveOccurred())

				// Subsequent call should fail quickly due to circuit breaker
				startTime := time.Now()
				_, err2 := btcRpc.GetTransactionsByAddress("test-address", "")
				duration := time.Since(startTime)

				Expect(err2).To(HaveOccurred())
				Expect(err2.Error()).To(ContainSubstring("circuit breaker"))
				Expect(duration).To(BeNumerically("<", 5*time.Second))
			})

			It("should reset circuit breaker after successful response", func() {
				var requestCount int
				var mu sync.Mutex

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					if count <= 3 {
						// First three requests return 429
						w.WriteHeader(http.StatusTooManyRequests)
						w.Write([]byte("Rate limit exceeded"))
					} else {
						// Fourth request succeeds
						w.WriteHeader(http.StatusOK)
						w.Write([]byte("[]"))
					}
				}))
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
						WalletWIF:          "test-wif",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)

				// Trigger circuit breaker
				_, err1 := btcRpc.GetTransactionsByAddress("test-address", "")
				Expect(err1).To(HaveOccurred())

				// Wait for circuit breaker auto-recovery (10 minutes in real implementation)
				// For testing, we'll manually reset or use shorter timeout
				time.Sleep(100 * time.Millisecond)

				// Should eventually succeed and reset circuit breaker
				transactions, err2 := btcRpc.GetTransactionsByAddress("test-address", "")
				Expect(err2).To(BeNil())
				Expect(transactions).NotTo(BeNil())
			})
		})

		Describe("Per-Endpoint Circuit Breakers", func() {
			It("should isolate circuit breakers per endpoint", func() {
				rateLimitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTooManyRequests)
					w.Write([]byte("Rate limit exceeded"))
				}))

				workingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("[]"))
				}))
				mockServers = append(mockServers, rateLimitServer, workingServer)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{rateLimitServer.URL, workingServer.URL},
						WalletWIF:          "test-wif",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)

				// Should succeed by using second endpoint after first fails
				transactions, err := btcRpc.GetTransactionsByAddress("test-address", "")
				Expect(err).To(BeNil())
				Expect(transactions).NotTo(BeNil())
			})
		})
	})

	Context("Thread Safety", func() {
		Describe("Concurrent Operations", func() {
			It("should handle concurrent requests safely", func() {
				server := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)

				const numGoroutines = 10
				var wg sync.WaitGroup
				errors := make(chan error, numGoroutines)

				// Run concurrent balance checks
				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := btcRpc.CurrentBalance()
						errors <- err
					}()
				}

				wg.Wait()
				close(errors)

				// All should succeed
				for err := range errors {
					Expect(err).To(BeNil())
				}
			})

			It("should handle concurrent endpoint switching safely", func() {
				var requestCount int
				var mu sync.Mutex

				server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					if count%2 == 0 {
						// Fail every other request
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
					}
				}))

				server2 := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 200000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
						WalletWIF:          "test-wif",
					},
					Blockchain: config.BlockchainConfig{
						BTCTreasuryAddress: "tb1qtest123",
					},
					ApiServer: config.ApiServerConfig{
						AppEnv: "test",
					},
				}

				btcRpc = btcrpc.New(appConfig, testLogger)

				const numGoroutines = 20
				var wg sync.WaitGroup
				results := make(chan *model.Web3BigInt, numGoroutines)
				errors := make(chan error, numGoroutines)

				// Run concurrent requests that will trigger endpoint switching
				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						balance, err := btcRpc.CurrentBalance()
						results <- balance
						errors <- err
					}()
				}

				wg.Wait()
				close(results)
				close(errors)

				successCount := 0
				for err := range errors {
					if err == nil {
						successCount++
					}
				}

				// Most requests should succeed due to failover
				Expect(successCount).To(BeNumerically(">", numGoroutines/2))
			})
		})
	})

	Context("Core Operations with Multiple Endpoints", func() {
		var workingServer1, workingServer2, failingServer *httptest.Server

		BeforeEach(func() {
			workingServer1 = createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
			workingServer2 = createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
			failingServer = createMockBlockstreamServer(http.StatusInternalServerError, "Server error")
			mockServers = append(mockServers, workingServer1, workingServer2, failingServer)
		})

		Describe("CurrentBalance", func() {
			It("should succeed with primary endpoint", func() {
				appConfig = createTestConfig([]string{workingServer1.URL, workingServer2.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
				Expect(balance.Value).To(Equal("100000"))
			})

			It("should failover when primary endpoint fails", func() {
				appConfig = createTestConfig([]string{failingServer.URL, workingServer1.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})
		})

		Describe("GetTransactionsByAddress", func() {
			It("should handle failover for transaction queries", func() {
				// Configure first server to fail, second to succeed
				txServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`[]`))
				}))
				mockServers = append(mockServers, txServer)

				appConfig = createTestConfig([]string{failingServer.URL, txServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				transactions, err := btcRpc.GetTransactionsByAddress("tb1qtest123", "")
				Expect(err).To(BeNil())
				Expect(transactions).NotTo(BeNil())
			})
		})

		Describe("EstimateFees", func() {
			It("should handle failover for fee estimation", func() {
				feeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"1": 10.0, "2": 8.0, "6": 5.0}`))
				}))
				mockServers = append(mockServers, feeServer)

				appConfig = createTestConfig([]string{failingServer.URL, feeServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				fees, err := btcRpc.EstimateFees()
				Expect(err).To(BeNil())
				Expect(fees).NotTo(BeNil())
				Expect(fees["1"]).To(Equal(10.0))
			})
		})
	})

	Context("Error Handling and Recovery", func() {
		Describe("Network Errors", func() {
			It("should handle connection timeouts", func() {
				// Create server that delays response
				slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second) // Simulate slow response
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))

				fastServer := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, slowServer, fastServer)

				appConfig = createTestConfig([]string{slowServer.URL, fastServer.URL})
				appConfig.Bitcoin.RequestTimeout = time.Second // Short timeout
				btcRpc = btcrpc.New(appConfig, testLogger)

				// Should failover to fast server
				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})

			It("should handle DNS resolution failures", func() {
				// Use invalid URL that will cause DNS failure
				invalidURL := "http://nonexistent-domain-12345.com"
				workingServer := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, workingServer)

				appConfig = createTestConfig([]string{invalidURL, workingServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				balance, err := btcRpc.CurrentBalance()
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())
			})
		})

		Describe("Partial Failures", func() {
			It("should handle mixed endpoint health", func() {
				healthyServer := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				unhealthyServer := createMockBlockstreamServer(http.StatusServiceUnavailable, "Service down")
				partialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Randomly succeed or fail
					if time.Now().UnixNano()%2 == 0 {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
					} else {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}))
				mockServers = append(mockServers, healthyServer, unhealthyServer, partialServer)

				appConfig = createTestConfig([]string{unhealthyServer.URL, partialServer.URL, healthyServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				// Multiple calls should eventually succeed
				successCount := 0
				for i := 0; i < 5; i++ {
					if _, err := btcRpc.CurrentBalance(); err == nil {
						successCount++
					}
				}

				Expect(successCount).To(BeNumerically(">", 0))
			})
		})
	})

	Context("Performance Under Load", func() {
		Describe("High Frequency Requests", func() {
			It("should maintain performance with endpoint switching", func() {
				server := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server)

				appConfig = createTestConfig([]string{server.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				const numRequests = 100
				startTime := time.Now()

				var wg sync.WaitGroup
				for i := 0; i < numRequests; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						btcRpc.CurrentBalance()
					}()
				}

				wg.Wait()
				duration := time.Since(startTime)

				// Should complete within reasonable time
				Expect(duration).To(BeNumerically("<", 30*time.Second))
			})

			It("should handle burst requests with rate limiting", func() {
				var requestCount int
				var mu sync.Mutex
				
				rateLimitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					if count > 10 { // Simulate rate limiting after 10 requests
						w.WriteHeader(http.StatusTooManyRequests)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))

				backupServer := createMockBlockstreamServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, rateLimitServer, backupServer)

				appConfig = createTestConfig([]string{rateLimitServer.URL, backupServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)

				const numRequests = 20
				var wg sync.WaitGroup
				successCount := int64(0)
				var successMu sync.Mutex

				for i := 0; i < numRequests; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if _, err := btcRpc.CurrentBalance(); err == nil {
							successMu.Lock()
							successCount++
							successMu.Unlock()
						}
					}()
				}

				wg.Wait()

				// Should have high success rate due to failover
				Expect(successCount).To(BeNumerically(">", numRequests/2))
			})
		})
	})
})

// Helper functions
func createMockBlockstreamServer(statusCode int, responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if responseBody != "" {
			w.Write([]byte(responseBody))
		}
	}))
}

func createTestConfig(endpoints []string) *config.AppConfig {
	return &config.AppConfig{
		Bitcoin: config.BitcoinConfig{
			BlockstreamAPIURLs: endpoints,
			WalletWIF:          "test-wif",
			MaxTxFeeUSD:        10.0,
			ServiceFeeRate:     0.01,
			MinSatshiFee:       1000,
			RequestTimeout:     30 * time.Second,
			EndpointRetryDelay: 5 * time.Minute,
		},
		Blockchain: config.BlockchainConfig{
			BTCTreasuryAddress: "tb1qtest123",
		},
		ApiServer: config.ApiServerConfig{
			AppEnv: "test",
		},
	}
}

func TestBtcRpcMultiEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BtcRpc Multi-Endpoint Suite")
}