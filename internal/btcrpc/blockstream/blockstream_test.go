package blockstream

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/stretchr/testify/assert"
)

func TestBlockstream_GetTransactionsByAddress_HandlesRateLimitWith429(t *testing.T) {
	// Track request count and timing
	requestCount := 0
	var requestTimes []time.Time

	// Create mock server that returns 429 on first call, 200 on second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		requestTimes = append(requestTimes, time.Now())

		if requestCount == 1 {
			// First request: return 429 Too Many Requests
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
			return
		}

		// Second request: return success with empty transaction list
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	// Create blockstream client
	cfg := &config.AppConfig{
		Bitcoin: config.BitcoinConfig{
			BlockstreamAPIURL: server.URL,
		},
	}
	testLogger := logger.New(environments.Test)
	client := New(cfg, testLogger)

	// Act: Call GetTransactionsByAddress
	startTime := time.Now()
	transactions, err := client.GetTransactionsByAddress("test-address", "")
	duration := time.Since(startTime)

	// Assert: Should succeed after retry with appropriate delay
	assert.NoError(t, err, "Should eventually succeed after 429 retry")
	assert.NotNil(t, transactions, "Should return transactions array")
	assert.Len(t, transactions, 0, "Should return empty array for this test")
	assert.Equal(t, 2, requestCount, "Should make exactly 2 requests (1 fail + 1 success)")

	// Assert: Should have waited at least 30 seconds between requests (exponential backoff)
	if len(requestTimes) >= 2 {
		timeBetweenRequests := requestTimes[1].Sub(requestTimes[0])
		assert.GreaterOrEqual(t, timeBetweenRequests, 30*time.Second, 
			"Should wait at least 30 seconds after 429 error")
	}

	// Assert: Total duration should be at least 30 seconds due to backoff
	assert.GreaterOrEqual(t, duration, 30*time.Second,
		"Total call duration should include backoff delay")
}

func TestBlockstream_GetTransactionsByAddress_ExponentialBackoffProgression(t *testing.T) {
	// Track request count and timing for multiple 429 responses
	requestCount := 0
	var requestTimes []time.Time

	// Create mock server that returns 429 twice, then 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		requestTimes = append(requestTimes, time.Now())

		if requestCount <= 2 {
			// First two requests: return 429 Too Many Requests
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
			return
		}

		// Third request: return success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	// Create blockstream client
	cfg := &config.AppConfig{
		Bitcoin: config.BitcoinConfig{
			BlockstreamAPIURL: server.URL,
		},
	}
	testLogger := logger.New(environments.Test)
	client := New(cfg, testLogger)

	// Act: Call GetTransactionsByAddress
	startTime := time.Now()
	transactions, err := client.GetTransactionsByAddress("test-address", "")
	duration := time.Since(startTime)

	// Assert: Should succeed after retries
	assert.NoError(t, err, "Should eventually succeed after multiple 429 retries")
	assert.NotNil(t, transactions, "Should return transactions array")
	assert.Equal(t, 3, requestCount, "Should make exactly 3 requests (2 fails + 1 success)")

	// Assert: Should follow exponential backoff progression
	if len(requestTimes) >= 3 {
		// First backoff: should be ~30 seconds
		firstBackoff := requestTimes[1].Sub(requestTimes[0])
		assert.GreaterOrEqual(t, firstBackoff, 30*time.Second, 
			"First backoff should be at least 30 seconds")
		assert.LessOrEqual(t, firstBackoff, 35*time.Second, 
			"First backoff should not exceed 35 seconds")

		// Second backoff: should be ~60 seconds  
		secondBackoff := requestTimes[2].Sub(requestTimes[1])
		assert.GreaterOrEqual(t, secondBackoff, 60*time.Second,
			"Second backoff should be at least 60 seconds")
		assert.LessOrEqual(t, secondBackoff, 65*time.Second,
			"Second backoff should not exceed 65 seconds")
	}

	// Assert: Total duration should be at least 90 seconds (30s + 60s)
	assert.GreaterOrEqual(t, duration, 90*time.Second,
		"Total duration should include both backoff delays")
}

func TestBlockstream_GetTransactionsByAddress_CircuitBreakerOpensAfterConsecutive429s(t *testing.T) {
	// Track request count for circuit breaker test
	requestCount := 0

	// Create mock server that always returns 429
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limit exceeded"))
	}))
	defer server.Close()

	// Create blockstream client
	cfg := &config.AppConfig{
		Bitcoin: config.BitcoinConfig{
			BlockstreamAPIURL: server.URL,
		},
	}
	testLogger := logger.New(environments.Test)
	client := New(cfg, testLogger)

	// Act: Call GetTransactionsByAddress - should fail after max retries
	_, err := client.GetTransactionsByAddress("test-address", "")

	// Assert: Should fail after exhausting retries
	assert.Error(t, err, "Should fail after max retries with rate limiting")
	assert.Contains(t, err.Error(), "429", "Error should mention rate limiting")
	
	// Now test that subsequent calls fail immediately due to circuit breaker
	startTime := time.Now()
	_, err2 := client.GetTransactionsByAddress("test-address", "")
	quickCallDuration := time.Since(startTime)

	// Assert: Second call should fail quickly (circuit breaker open)
	assert.Error(t, err2, "Should fail immediately when circuit breaker is open")
	assert.Contains(t, err2.Error(), "circuit breaker", "Error should mention circuit breaker")
	assert.Less(t, quickCallDuration, 5*time.Second, 
		"Circuit breaker should fail fast, not wait for backoff")
}