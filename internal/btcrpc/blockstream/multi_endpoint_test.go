package blockstream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

var _ = Describe("Blockstream Multi-Endpoint Client", func() {
	var (
		client      blockstream.IBlockStream
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

	Context("Multi-Endpoint Client Initialization", func() {
		Describe("Client Creation", func() {
			It("should create client with multiple endpoints", func() {
				server1 := createMockServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				server2 := createMockServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)
				Expect(client).NotTo(BeNil())
			})

			It("should handle single endpoint in array", func() {
				server := createMockServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)
				Expect(client).NotTo(BeNil())
			})

			It("should create client with endpoint health tracking", func() {
				server := createMockServer(http.StatusOK, `{}`)
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)
				
				// Should expose health status
				healthStatus := client.GetEndpointHealth()
				Expect(healthStatus).NotTo(BeNil())
			})
		})
	})

	Context("Endpoint Selection and Failover", func() {
		Describe("Primary Endpoint Selection", func() {
			It("should use first endpoint by default", func() {
				var server1Hits, server2Hits int
				var mu sync.Mutex

				server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server1Hits++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))

				server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server2Hits++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make a request
				_, err := client.GetBTCBalance("test-address")
				Expect(err).To(BeNil())

				// Should have used first endpoint
				mu.Lock()
				Expect(server1Hits).To(Equal(1))
				Expect(server2Hits).To(Equal(0))
				mu.Unlock()
			})

			It("should failover to next endpoint when primary fails", func() {
				var server1Hits, server2Hits int
				var mu sync.Mutex

				server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server1Hits++
					mu.Unlock()
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Server error"))
				}))

				server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server2Hits++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make a request
				balance, err := client.GetBTCBalance("test-address")
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())

				// Should have tried first endpoint, then succeeded with second
				mu.Lock()
				Expect(server1Hits).To(Equal(1))
				Expect(server2Hits).To(Equal(1))
				mu.Unlock()
			})
		})

		Describe("Round-Robin Selection", func() {
			It("should distribute load across healthy endpoints", func() {
				var server1Hits, server2Hits int
				var mu sync.Mutex

				server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server1Hits++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))

				server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server2Hits++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"funded_txo_sum": 100000, "spent_txo_sum": 0}`))
				}))
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs:    []string{server1.URL, server2.URL},
						EndpointLoadBalancing: "round-robin",
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make multiple requests
				for i := 0; i < 4; i++ {
					_, err := client.GetBTCBalance("test-address")
					Expect(err).To(BeNil())
				}

				// Should distribute load evenly
				mu.Lock()
				Expect(server1Hits).To(Equal(2))
				Expect(server2Hits).To(Equal(2))
				mu.Unlock()
			})
		})
	})

	Context("Circuit Breaker Per Endpoint", func() {
		Describe("Individual Endpoint Circuit Breakers", func() {
			It("should open circuit breaker for specific endpoint after failures", func() {
				var server1RequestCount, server2RequestCount int
				var mu sync.Mutex

				// Server 1 always fails with 429
				server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server1RequestCount++
					mu.Unlock()
					w.WriteHeader(http.StatusTooManyRequests)
					w.Write([]byte("Rate limit exceeded"))
				}))

				// Server 2 always succeeds
				server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					server2RequestCount++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("[]"))
				}))
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make multiple requests
				for i := 0; i < 5; i++ {
					transactions, err := client.GetTransactionsByAddress("test-address", "")
					Expect(err).To(BeNil())
					Expect(transactions).NotTo(BeNil())
				}

				// After circuit breaker opens for server1, only server2 should receive requests
				mu.Lock()
				Expect(server1RequestCount).To(BeNumerically("<=", 3)) // Should stop after circuit opens
				Expect(server2RequestCount).To(BeNumerically(">", 0))  // Should receive failover requests
				mu.Unlock()
			})

			It("should reset circuit breaker after timeout", func() {
				var requestCount int
				var mu sync.Mutex

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					if count <= 3 {
						// First 3 requests fail to trigger circuit breaker
						w.WriteHeader(http.StatusTooManyRequests)
						w.Write([]byte("Rate limit exceeded"))
					} else {
						// Subsequent requests succeed
						w.WriteHeader(http.StatusOK)
						w.Write([]byte("[]"))
					}
				}))
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs:        []string{server.URL},
						CircuitBreakerTimeout:     100 * time.Millisecond, // Short timeout for testing
						CircuitBreakerFailureThreshold: 3,
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Trigger circuit breaker
				for i := 0; i < 3; i++ {
					client.GetTransactionsByAddress("test-address", "")
				}

				// Wait for circuit breaker to reset
				time.Sleep(150 * time.Millisecond)

				// Should succeed after reset
				transactions, err := client.GetTransactionsByAddress("test-address", "")
				Expect(err).To(BeNil())
				Expect(transactions).NotTo(BeNil())
			})
		})
	})

	Context("Thread Safety with Multiple Endpoints", func() {
		Describe("Concurrent Endpoint Operations", func() {
			It("should handle concurrent requests with endpoint switching", func() {
				server1 := createMockServer(http.StatusOK, `{"funded_txo_sum": 100000, "spent_txo_sum": 0}`)
				server2 := createMockServer(http.StatusOK, `{"funded_txo_sum": 200000, "spent_txo_sum": 0}`)
				mockServers = append(mockServers, server1, server2)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server1.URL, server2.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				const numGoroutines = 20
				var wg sync.WaitGroup
				errors := make(chan error, numGoroutines)

				// Run concurrent requests
				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := client.GetBTCBalance("test-address")
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

			It("should handle concurrent circuit breaker state changes", func() {
				var requestCount int
				var mu sync.Mutex

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()

					// Simulate intermittent failures to test circuit breaker race conditions
					if count%3 == 0 {
						w.WriteHeader(http.StatusTooManyRequests)
						w.Write([]byte("Rate limit exceeded"))
					} else {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte("[]"))
					}
				}))
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				const numGoroutines = 30
				var wg sync.WaitGroup
				successCount := int64(0)
				var successMu sync.Mutex

				// Run concurrent requests that will trigger circuit breaker state changes
				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if _, err := client.GetTransactionsByAddress("test-address", ""); err == nil {
							successMu.Lock()
							successCount++
							successMu.Unlock()
						}
					}()
				}

				wg.Wait()

				// Should have some successes despite circuit breaker activity
				Expect(successCount).To(BeNumerically(">", 0))
			})
		})
	})

	Context("Endpoint Health Monitoring", func() {
		Describe("Health Status Tracking", func() {
			It("should track endpoint health status", func() {
				healthyServer := createMockServer(http.StatusOK, `{}`)
				unhealthyServer := createMockServer(http.StatusInternalServerError, "Error")
				mockServers = append(mockServers, healthyServer, unhealthyServer)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{healthyServer.URL, unhealthyServer.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make requests to both endpoints
				client.GetBTCBalance("test-address") // Should succeed with healthy endpoint
				
				// Check health status
				health := client.GetEndpointHealth()
				Expect(health).To(HaveLen(2))
				
				// At least one endpoint should be healthy
				healthyCount := 0
				for _, status := range health {
					if status.IsHealthy {
						healthyCount++
					}
				}
				Expect(healthyCount).To(BeNumerically(">", 0))
			})

			It("should provide detailed health metrics", func() {
				server := createMockServer(http.StatusOK, `{}`)
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make some requests
				for i := 0; i < 5; i++ {
					client.GetBTCBalance("test-address")
				}

				health := client.GetEndpointHealth()
				Expect(health).To(HaveLen(1))
				
				endpointHealth := health[0]
				Expect(endpointHealth.URL).To(Equal(server.URL))
				Expect(endpointHealth.RequestCount).To(Equal(int64(5)))
				Expect(endpointHealth.SuccessCount).To(Equal(int64(5)))
				Expect(endpointHealth.ErrorCount).To(Equal(int64(0)))
				Expect(endpointHealth.IsHealthy).To(BeTrue())
			})
		})

		Describe("Health-Based Endpoint Selection", func() {
			It("should prefer healthy endpoints", func() {
				var healthyHits, unhealthyHits int
				var mu sync.Mutex

				healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					healthyHits++
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{}`))
				}))

				unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					unhealthyHits++
					mu.Unlock()
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte("Service unavailable"))
				}))
				mockServers = append(mockServers, unhealthyServer, healthyServer)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs:           []string{unhealthyServer.URL, healthyServer.URL},
						EndpointSelectionStrategy:    "health-based",
						HealthCheckInterval:          100 * time.Millisecond,
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Make multiple requests
				for i := 0; i < 5; i++ {
					client.GetBTCBalance("test-address")
					time.Sleep(50 * time.Millisecond)
				}

				mu.Lock()
				// Should prefer healthy endpoint after initial discovery
				Expect(healthyHits).To(BeNumerically(">=", unhealthyHits))
				mu.Unlock()
			})
		})
	})

	Context("Error Handling and Retry Logic", func() {
		Describe("Retry with Exponential Backoff", func() {
			It("should retry failed requests with backoff", func() {
				var requestCount int
				var requestTimes []time.Time
				var mu sync.Mutex

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					requestTimes = append(requestTimes, time.Now())
					count := requestCount
					mu.Unlock()

					if count <= 2 {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte("Server error"))
					} else {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{}`))
					}
				}))
				mockServers = append(mockServers, server)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{server.URL},
						RetryMaxAttempts:   3,
						RetryBaseDelay:     100 * time.Millisecond,
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Should succeed after retries
				balance, err := client.GetBTCBalance("test-address")
				Expect(err).To(BeNil())
				Expect(balance).NotTo(BeNil())

				// Should have made 3 attempts with increasing delays
				mu.Lock()
				Expect(requestCount).To(Equal(3))
				Expect(len(requestTimes)).To(Equal(3))
				
				if len(requestTimes) >= 2 {
					// Check that there was a delay between retries
					delay := requestTimes[1].Sub(requestTimes[0])
					Expect(delay).To(BeNumerically(">=", 100*time.Millisecond))
				}
				mu.Unlock()
			})
		})

		Describe("Context Timeout Handling", func() {
			It("should respect context timeout", func() {
				slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(200 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{}`))
				}))
				mockServers = append(mockServers, slowServer)

				appConfig = &config.AppConfig{
					Bitcoin: config.BitcoinConfig{
						BlockstreamAPIURLs: []string{slowServer.URL},
					},
				}

				client = blockstream.NewMultiEndpoint(appConfig, testLogger)

				// Create context with short timeout
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()

				// Should timeout
				_, err := client.GetBTCBalanceWithContext(ctx, "test-address")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timeout"))
			})
		})
	})
})

// Helper functions
func createMockServer(statusCode int, responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if responseBody != "" {
			w.Write([]byte(responseBody))
		}
	}))
}

func TestBlockstreamMultiEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Blockstream Multi-Endpoint Suite")
}