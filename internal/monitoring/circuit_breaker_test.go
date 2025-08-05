package monitoring

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// Mock BTC RPC for testing
type MockBtcRPC struct {
	mock.Mock
	shouldFail bool
	failCount  int
}

func (m *MockBtcRPC) Send(receiverAddress string, amount *model.Web3BigInt) (string, int64, error) {
	args := m.Called(receiverAddress, amount)
	if m.shouldFail {
		m.failCount++
		return "", 0, args.Error(2)
	}
	return args.String(0), args.Get(1).(int64), args.Error(2)
}

func (m *MockBtcRPC) CurrentBalance() (*model.Web3BigInt, error) {
	args := m.Called()
	if m.shouldFail {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Web3BigInt), args.Error(1)
}

func (m *MockBtcRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error) {
	args := m.Called(address, fromTxId)
	if m.shouldFail {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.OnchainBtcTransaction), args.Error(1)
}

func (m *MockBtcRPC) EstimateFees() (map[string]float64, error) {
	args := m.Called()
	if m.shouldFail {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]float64), args.Error(1)
}

func (m *MockBtcRPC) GetSatoshiUSDPrice() (float64, error) {
	args := m.Called()
	if m.shouldFail {
		return 0, args.Error(1)
	}
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockBtcRPC) IsDust(address string, amount int64) bool {
	args := m.Called(address, amount)
	return args.Bool(0)
}

func createWeb3BigInt(value string) *model.Web3BigInt {
	return &model.Web3BigInt{
		Value:   value,
		Decimal: 8,
	}
}

func setupTestLogger() *logger.Logger {
	return logger.New("test")
}

func TestCircuitBreaker_InitialState(t *testing.T) {
	// Arrange
	config := CircuitBreakerConfig{
		MaxRequests:               5,
		Interval:                  30 * time.Second,
		Timeout:                   60 * time.Second,
		ConsecutiveFailureThreshold: 3,
	}

	metrics := NewExternalAPIMetrics()
	registry := prometheus.NewRegistry()
	metrics.MustRegister(registry)

	mockBtcRPC := &MockBtcRPC{}
	cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())

	// Act & Assert
	assert.Equal(t, gobreaker.StateClosed, cb.circuitBreaker.State())

	// Verify initial metrics
	metricFamilies, err := registry.Gather()
	assert.NoError(t, err)

	for _, mf := range metricFamilies {
		if mf.GetName() == "icy_backend_circuit_breaker_state" {
			metric := mf.GetMetric()[0]
			assert.Equal(t, float64(gobreaker.StateClosed), metric.GetGauge().GetValue())
		}
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	// Arrange
	config := CircuitBreakerConfig{
		MaxRequests:               5,
		Interval:                  30 * time.Second,
		Timeout:                   60 * time.Second,
		ConsecutiveFailureThreshold: 3,
	}

	metrics := NewExternalAPIMetrics()
	registry := prometheus.NewRegistry()
	metrics.MustRegister(registry)

	mockBtcRPC := &MockBtcRPC{shouldFail: true}
	mockBtcRPC.On("Send", mock.Anything, mock.Anything).Return("", int64(0), errors.New("API unavailable"))
	
	cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())

	// Act - Trigger consecutive failures
	var lastErr error
	for i := 0; i < 3; i++ {
		_, _, lastErr = cb.Send("test_address", createWeb3BigInt("1000"))
		assert.Error(t, lastErr)
	}

	// Assert - Circuit breaker should be open
	assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())

	// Verify metrics
	metricFamilies, err := registry.Gather()
	assert.NoError(t, err)

	errorCountFound := false
	stateFound := false

	for _, mf := range metricFamilies {
		switch mf.GetName() {
		case "icy_backend_external_api_calls_total":
			for _, metric := range mf.GetMetric() {
				labels := metric.GetLabel()
				if getLabelValue(labels, "status") == "error" {
					errorCountFound = true
					assert.Equal(t, float64(3), metric.GetCounter().GetValue())
				}
			}
		case "icy_backend_circuit_breaker_state":
			stateFound = true
			metric := mf.GetMetric()[0]
			assert.Equal(t, float64(gobreaker.StateOpen), metric.GetGauge().GetValue())
		}
	}

	assert.True(t, errorCountFound, "Error count metric not found")
	assert.True(t, stateFound, "Circuit breaker state metric not found")
}

func TestCircuitBreaker_EstimateFees_CircuitOpen(t *testing.T) {
	// Arrange
	config := CircuitBreakerConfig{
		MaxRequests:               5,
		Interval:                  30 * time.Second,
		Timeout:                   60 * time.Second,
		ConsecutiveFailureThreshold: 2,
	}

	mockBtcRPC := &MockBtcRPC{shouldFail: true}
	mockBtcRPC.On("EstimateFees").Return(map[string]float64{}, errors.New("network error"))
	
	metrics := NewExternalAPIMetrics()
	cb := NewCircuitBreakerBtcRPC(mockBtcRPC, config, metrics, setupTestLogger())

	// Force circuit breaker to open
	for i := 0; i < 2; i++ {
		_, err := cb.EstimateFees()
		assert.Error(t, err)
	}
	assert.Equal(t, gobreaker.StateOpen, cb.circuitBreaker.State())

	// Act - Call when circuit is open
	fees, err := cb.EstimateFees()

	// Assert - Should fail immediately with circuit breaker error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
	assert.Nil(t, fees)
}

func TestErrorClassification_NetworkErrors(t *testing.T) {
	tests := []struct {
		name         string
		error        error
		expectedType APIErrorType
	}{
		{
			name:         "Timeout error",
			error:        errors.New("request timeout after 5s"),
			expectedType: ErrorTypeTimeout,
		},
		{
			name:         "Network error",
			error:        errors.New("network unreachable"),
			expectedType: ErrorTypeNetworkError,
		},
		{
			name:         "Server error",
			error:        errors.New("HTTP 500 Internal Server Error"),
			expectedType: ErrorTypeServerError,
		},
		{
			name:         "Client error",
			error:        errors.New("HTTP 400 Bad Request"),
			expectedType: ErrorTypeClientError,
		},
		{
			name:         "Unknown error",
			error:        errors.New("unexpected error occurred"),
			expectedType: ErrorTypeUnknown,
		},
		{
			name:         "Nil error",
			error:        nil,
			expectedType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.error)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

func TestCircuitBreakerConfig_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    CircuitBreakerConfig
		shouldErr bool
	}{
		{
			name: "Valid configuration",
			config: CircuitBreakerConfig{
				MaxRequests:               5,
				Interval:                  30 * time.Second,
				Timeout:                   60 * time.Second,
				ConsecutiveFailureThreshold: 3,
			},
			shouldErr: false,
		},
		{
			name: "Zero max requests",
			config: CircuitBreakerConfig{
				MaxRequests:               0,
				Interval:                  30 * time.Second,
				Timeout:                   60 * time.Second,
				ConsecutiveFailureThreshold: 3,
			},
			shouldErr: true,
		},
		{
			name: "Zero failure threshold",
			config: CircuitBreakerConfig{
				MaxRequests:               5,
				Interval:                  30 * time.Second,
				Timeout:                   60 * time.Second,
				ConsecutiveFailureThreshold: 0,
			},
			shouldErr: true,
		},
		{
			name: "Negative timeout",
			config: CircuitBreakerConfig{
				MaxRequests:               5,
				Interval:                  30 * time.Second,
				Timeout:                   -1 * time.Second,
				ConsecutiveFailureThreshold: 3,
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCircuitBreakerConfig(tt.config)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCircuitBreakerConfig_DefaultValues(t *testing.T) {
	// Test that default configurations are applied correctly
	configs := map[string]CircuitBreakerConfig{
		"blockstream_api": CircuitBreakerConfigs["blockstream_api"],
		"base_rpc":       CircuitBreakerConfigs["base_rpc"],
	}

	for serviceName, config := range configs {
		t.Run(serviceName, func(t *testing.T) {
			assert.True(t, config.MaxRequests > 0, "MaxRequests should be positive")
			assert.True(t, config.Interval > 0, "Interval should be positive")
			assert.True(t, config.Timeout > 0, "Timeout should be positive")
			assert.True(t, config.ConsecutiveFailureThreshold > 0, "ConsecutiveFailureThreshold should be positive")

			// Service-specific assertions
			switch serviceName {
			case "blockstream_api":
				assert.Equal(t, uint32(5), config.MaxRequests)
				assert.Equal(t, 30*time.Second, config.Interval)
				assert.Equal(t, 60*time.Second, config.Timeout)
				assert.Equal(t, 3, config.ConsecutiveFailureThreshold)

			case "base_rpc":
				assert.Equal(t, uint32(3), config.MaxRequests)
				assert.Equal(t, 45*time.Second, config.Interval)
				assert.Equal(t, 120*time.Second, config.Timeout)
				assert.Equal(t, 5, config.ConsecutiveFailureThreshold)
			}
		})
	}
}

// Helper function to get label value from Prometheus metric labels
func getLabelValue(labels []*dto.LabelPair, name string) string {
	for _, label := range labels {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}