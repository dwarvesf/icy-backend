package telemetry_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

var _ = Describe("Telemetry with BTC Multi-Endpoint Support", func() {
	var (
		telemetryService *telemetry.Telemetry
		btcRpc           btcrpc.IBtcRpc
		mockStore        *store.MockStore
		testLogger       *logger.Logger
		mockServers      []*httptest.Server
		appConfig        *config.AppConfig
	)

	BeforeEach(func() {
		testLogger = logger.New(environments.Test)
		mockStore = store.NewMockStore()
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

	Context("BTC Transaction Indexing with Multiple Endpoints", func() {
		Describe("Successful Indexing with Primary Endpoint", func() {
			It("should index BTC transactions using primary endpoint", func() {
				primaryServer := createBTCMockServer(http.StatusOK, sampleBTCTransactionsResponse)
				mockServers = append(mockServers, primaryServer)

				appConfig = createTelemetryTestConfig([]string{primaryServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Mock the last transaction ID in store
				mockStore.SetLastBTCTransactionID("test-tx-id")

				// Run BTC indexing
				err := telemetryService.IndexBTCTransactions()
				Expect(err).To(BeNil())

				// Verify that transactions were processed
				indexedTxs := mockStore.GetIndexedBTCTransactions()
				Expect(len(indexedTxs)).To(BeNumerically(">", 0))
			})

			It("should handle large volume of transactions efficiently", func() {
				primaryServer := createBTCMockServer(http.StatusOK, largeBTCTransactionsResponse)
				mockServers = append(mockServers, primaryServer)

				appConfig = createTelemetryTestConfig([]string{primaryServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				startTime := time.Now()
				err := telemetryService.IndexBTCTransactions()
				duration := time.Since(startTime)

				Expect(err).To(BeNil())
				// Should complete within reasonable time even with many transactions
				Expect(duration).To(BeNumerically("<", 10*time.Second))

				indexedTxs := mockStore.GetIndexedBTCTransactions()
				Expect(len(indexedTxs)).To(Equal(100)) // Large response has 100 transactions
			})
		})

		Describe("Failover During Transaction Indexing", func() {
			It("should failover to secondary endpoint when primary fails", func() {
				failingServer := createBTCMockServer(http.StatusInternalServerError, "Server error")
				workingServer := createBTCMockServer(http.StatusOK, sampleBTCTransactionsResponse)
				mockServers = append(mockServers, failingServer, workingServer)

				appConfig = createTelemetryTestConfig([]string{failingServer.URL, workingServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				mockStore.SetLastBTCTransactionID("test-tx-id")

				// Should succeed using secondary endpoint
				err := telemetryService.IndexBTCTransactions()
				Expect(err).To(BeNil())

				indexedTxs := mockStore.GetIndexedBTCTransactions()
				Expect(len(indexedTxs)).To(BeNumerically(">", 0))
			})

			It("should handle partial failures gracefully", func() {
				// Server that works for balance but fails for transactions
				partialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/address/tb1qtest123" {
						// Balance endpoint works
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
					} else if r.URL.Path == "/address/tb1qtest123/txs" {
						// Transaction endpoint fails
						w.WriteHeader(http.StatusServiceUnavailable)
						w.Write([]byte("Transaction service unavailable"))
					}
				}))

				backupServer := createBTCMockServer(http.StatusOK, sampleBTCTransactionsResponse)
				mockServers = append(mockServers, partialServer, backupServer)

				appConfig = createTelemetryTestConfig([]string{partialServer.URL, backupServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Should succeed by using backup for transactions
				err := telemetryService.IndexBTCTransactions()
				Expect(err).To(BeNil())
			})
		})

		Describe("Rate Limiting and Circuit Breaker Integration", func() {
			It("should handle rate limiting across multiple endpoints", func() {
				var server1Requests, server2Requests int
				var mu sync.Mutex

				rateLimitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server1Requests++
					mu.Unlock()

					if server1Requests <= 2 {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(sampleBTCTransactionsResponse))
					} else {
						w.WriteHeader(http.StatusTooManyRequests)
						w.Write([]byte("Rate limit exceeded"))
					}
				}))

				backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server2Requests++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(sampleBTCTransactionsResponse))
				}))
				mockServers = append(mockServers, rateLimitServer, backupServer)

				appConfig = createTelemetryTestConfig([]string{rateLimitServer.URL, backupServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Run multiple indexing cycles
				for i := 0; i < 5; i++ {
					err := telemetryService.IndexBTCTransactions()
					Expect(err).To(BeNil())
				}

				// Should have used backup server after rate limiting
				mu.Lock()
				Expect(server2Requests).To(BeNumerically(">", 0))
				mu.Unlock()
			})

			It("should recover from circuit breaker state", func() {
				var requestCount int
				var mu sync.Mutex

				recoveringServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					if count <= 3 {
						// First few requests fail to trigger circuit breaker
						w.WriteHeader(http.StatusTooManyRequests)
						w.Write([]byte("Rate limit exceeded"))
					} else {
						// Later requests succeed
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(sampleBTCTransactionsResponse))
					}
				}))
				mockServers = append(mockServers, recoveringServer)

				appConfig = createTelemetryTestConfig([]string{recoveringServer.URL})
				appConfig.Bitcoin.CircuitBreakerTimeout = 100 * time.Millisecond // Short timeout for testing
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Trigger circuit breaker
				for i := 0; i < 3; i++ {
					telemetryService.IndexBTCTransactions()
				}

				// Wait for circuit breaker recovery
				time.Sleep(150 * time.Millisecond)

				// Should succeed after recovery
				err := telemetryService.IndexBTCTransactions()
				Expect(err).To(BeNil())
			})
		})
	})

	Context("Cron Job Integration with Multiple Endpoints", func() {
		Describe("Scheduled Transaction Indexing", func() {
			It("should handle endpoint failures during scheduled jobs", func() {
				intermittentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Randomly succeed or fail to simulate real-world conditions
					if time.Now().UnixNano()%2 == 0 {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(sampleBTCTransactionsResponse))
					} else {
						w.WriteHeader(http.StatusServiceUnavailable)
						w.Write([]byte("Service temporarily unavailable"))
					}
				}))

				reliableServer := createBTCMockServer(http.StatusOK, sampleBTCTransactionsResponse)
				mockServers = append(mockServers, intermittentServer, reliableServer)

				appConfig = createTelemetryTestConfig([]string{intermittentServer.URL, reliableServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Simulate multiple cron job executions
				successCount := 0
				for i := 0; i < 10; i++ {
					if err := telemetryService.IndexBTCTransactions(); err == nil {
						successCount++
					}
					time.Sleep(10 * time.Millisecond) // Brief delay between executions
				}

				// Should have high success rate due to failover
				Expect(successCount).To(BeNumerically(">=", 8))
			})

			It("should maintain indexing continuity across endpoint switches", func() {
				var server1Calls, server2Calls int
				var mu sync.Mutex

				server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server1Calls++
					callCount := server1Calls
					mu.Unlock()

					if callCount <= 3 {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(sampleBTCTransactionsResponse))
					} else {
						// Start failing after 3 calls
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte("Server maintenance"))
					}
				}))

				server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server2Calls++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(sampleBTCTransactionsResponse))
				}))
				mockServers = append(mockServers, server1, server2)

				appConfig = createTelemetryTestConfig([]string{server1.URL, server2.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Run indexing jobs - should switch endpoints automatically
				for i := 0; i < 6; i++ {
					err := telemetryService.IndexBTCTransactions()
					Expect(err).To(BeNil())
				}

				// Verify both endpoints were used
				mu.Lock()
				Expect(server1Calls).To(BeNumerically(">", 0))
				Expect(server2Calls).To(BeNumerically(">", 0))
				mu.Unlock()

				// Verify transactions were continuously indexed
				indexedTxs := mockStore.GetIndexedBTCTransactions()
				Expect(len(indexedTxs)).To(BeNumerically(">", 0))
			})
		})

		Describe("Error Recovery and Resilience", func() {
			It("should recover from total endpoint failure", func() {
				// All endpoints fail initially, then recover
				var failurePhase bool = true
				var mu sync.Mutex

				recoveringServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					inFailure := failurePhase
					mu.Unlock()

					if inFailure {
						w.WriteHeader(http.StatusServiceUnavailable)
						w.Write([]byte("All services down"))
					} else {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(sampleBTCTransactionsResponse))
					}
				}))
				mockServers = append(mockServers, recoveringServer)

				appConfig = createTelemetryTestConfig([]string{recoveringServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// First attempts should fail
				err1 := telemetryService.IndexBTCTransactions()
				Expect(err1).To(HaveOccurred())

				// Simulate service recovery
				mu.Lock()
				failurePhase = false
				mu.Unlock()

				// Subsequent attempts should succeed
				err2 := telemetryService.IndexBTCTransactions()
				Expect(err2).To(BeNil())

				indexedTxs := mockStore.GetIndexedBTCTransactions()
				Expect(len(indexedTxs)).To(BeNumerically(">", 0))
			})

			It("should handle concurrent indexing jobs safely", func() {
				server := createBTCMockServer(http.StatusOK, sampleBTCTransactionsResponse)
				mockServers = append(mockServers, server)

				appConfig = createTelemetryTestConfig([]string{server.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Run multiple indexing jobs concurrently
				var wg sync.WaitGroup
				errors := make(chan error, 5)

				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := telemetryService.IndexBTCTransactions()
						errors <- err
					}()
				}

				wg.Wait()
				close(errors)

				// All should complete without errors
				for err := range errors {
					Expect(err).To(BeNil())
				}

				// Should not have duplicate transactions
				indexedTxs := mockStore.GetIndexedBTCTransactions()
				Expect(len(indexedTxs)).To(BeNumerically(">", 0))
				
				// Verify no duplicates by checking unique transaction IDs
				txIDs := make(map[string]bool)
				for _, tx := range indexedTxs {
					Expect(txIDs[tx.TransactionHash]).To(BeFalse(), "Duplicate transaction ID found: %s", tx.TransactionHash)
					txIDs[tx.TransactionHash] = true
				}
			})
		})
	})

	Context("Monitoring and Metrics", func() {
		Describe("Endpoint Health Tracking", func() {
			It("should track endpoint health during telemetry operations", func() {
				healthyServer := createBTCMockServer(http.StatusOK, sampleBTCTransactionsResponse)
				unhealthyServer := createBTCMockServer(http.StatusServiceUnavailable, "Service down")
				mockServers = append(mockServers, unhealthyServer, healthyServer)

				appConfig = createTelemetryTestConfig([]string{unhealthyServer.URL, healthyServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Run indexing to generate health data
				err := telemetryService.IndexBTCTransactions()
				Expect(err).To(BeNil())

				// Should be able to get health metrics
				healthMetrics := telemetryService.GetBTCEndpointHealth()
				Expect(healthMetrics).NotTo(BeNil())
				Expect(len(healthMetrics)).To(Equal(2))

				// At least one endpoint should be healthy
				healthyCount := 0
				for _, metric := range healthMetrics {
					if metric.IsHealthy {
						healthyCount++
					}
				}
				Expect(healthyCount).To(BeNumerically(">", 0))
			})

			It("should provide performance metrics per endpoint", func() {
				fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Respond quickly
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(sampleBTCTransactionsResponse))
				}))

				slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Respond slowly
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(sampleBTCTransactionsResponse))
				}))
				mockServers = append(mockServers, fastServer, slowServer)

				appConfig = createTelemetryTestConfig([]string{fastServer.URL, slowServer.URL})
				btcRpc = btcrpc.New(appConfig, testLogger)
				telemetryService = telemetry.New(nil, mockStore, appConfig, testLogger, btcRpc, nil)

				// Run indexing multiple times
				for i := 0; i < 3; i++ {
					telemetryService.IndexBTCTransactions()
				}

				// Should have performance metrics
				metrics := telemetryService.GetBTCEndpointPerformance()
				Expect(metrics).NotTo(BeNil())
				Expect(len(metrics)).To(Equal(2))

				// Should show performance differences
				var fastMetric, slowMetric *telemetry.EndpointPerformance
				for i, metric := range metrics {
					if metric.URL == fastServer.URL {
						fastMetric = &metrics[i]
					} else if metric.URL == slowServer.URL {
						slowMetric = &metrics[i]
					}
				}

				Expect(fastMetric).NotTo(BeNil())
				Expect(slowMetric).NotTo(BeNil())
				Expect(fastMetric.AverageResponseTime).To(BeNumerically("<", slowMetric.AverageResponseTime))
			})
		})
	})
})

// Helper functions and mock data
func createBTCMockServer(statusCode int, responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if responseBody != "" {
			w.Write([]byte(responseBody))
		}
	}))
}

func createTelemetryTestConfig(endpoints []string) *config.AppConfig {
	return &config.AppConfig{
		Bitcoin: config.BitcoinConfig{
			BlockstreamAPIURLs:             endpoints,
			WalletWIF:                      "test-wif",
			EndpointTimeout:                30 * time.Second,
			EndpointRetryDelay:             1 * time.Second, // Short for testing
			CircuitBreakerFailureThreshold: 3,
			CircuitBreakerTimeout:          5 * time.Second, // Short for testing
		},
		Blockchain: config.BlockchainConfig{
			BTCTreasuryAddress: "tb1qtest123",
		},
		IndexInterval: "2m",
		ApiServer: config.ApiServerConfig{
			AppEnv: "test",
		},
	}
}

const sampleBTCTransactionsResponse = `[
	{
		"txid": "tx1",
		"status": {"confirmed": true, "block_time": 1640995200},
		"vin": [{"prevout": {"scriptpubkey_address": "other-address", "value": 50000}}],
		"vout": [{"scriptpubkey_address": "tb1qtest123", "value": 45000}],
		"fee": 5000
	},
	{
		"txid": "tx2",
		"status": {"confirmed": true, "block_time": 1640995300},
		"vin": [{"prevout": {"scriptpubkey_address": "tb1qtest123", "value": 30000}}],
		"vout": [{"scriptpubkey_address": "recipient-address", "value": 25000}],
		"fee": 5000
	}
]`

const largeBTCTransactionsResponse = generateLargeBTCResponse()

func generateLargeBTCResponse() string {
	transactions := make([]string, 100)
	for i := 0; i < 100; i++ {
		transactions[i] = fmt.Sprintf(`{
			"txid": "tx%d",
			"status": {"confirmed": true, "block_time": %d},
			"vin": [{"prevout": {"scriptpubkey_address": "other-address", "value": %d}}],
			"vout": [{"scriptpubkey_address": "tb1qtest123", "value": %d}],
			"fee": 5000
		}`, i, 1640995200+i*60, 50000+i*1000, 45000+i*1000)
	}
	return "[" + strings.Join(transactions, ",") + "]"
}

func TestTelemetryBtcMultiEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Telemetry BTC Multi-Endpoint Suite")
}