package monitoring

import (
	"time"
)

// CircuitBreakerConfig defines the configuration for circuit breakers
type CircuitBreakerConfig struct {
	MaxRequests               uint32        `json:"max_requests"`
	Interval                  time.Duration `json:"interval"`
	Timeout                   time.Duration `json:"timeout"`
	ConsecutiveFailureThreshold int         `json:"consecutive_failure_threshold"`
}

// TimeoutConfig defines timeout configurations for different operations
type TimeoutConfig struct {
	ConnectionTimeout  time.Duration `json:"connection_timeout"`
	RequestTimeout     time.Duration `json:"request_timeout"`
	HealthCheckTimeout time.Duration `json:"health_check_timeout"`
}

// APIErrorType represents different types of API errors for classification
type APIErrorType string

const (
	ErrorTypeTimeout      APIErrorType = "timeout"
	ErrorTypeNetworkError APIErrorType = "network_error"
	ErrorTypeServerError  APIErrorType = "server_error"
	ErrorTypeClientError  APIErrorType = "client_error"
	ErrorTypeUnknown      APIErrorType = "unknown"
)

// CircuitBreakerConfigs provides default configurations for different services
var CircuitBreakerConfigs = map[string]CircuitBreakerConfig{
	"blockstream_api": {
		MaxRequests:               5,
		Interval:                  30 * time.Second,
		Timeout:                   60 * time.Second,
		ConsecutiveFailureThreshold: 3,
	},
	"base_rpc": {
		MaxRequests:               3,
		Interval:                  45 * time.Second,
		Timeout:                   120 * time.Second,
		ConsecutiveFailureThreshold: 5,
	},
}

// DefaultTimeoutConfig provides default timeout configurations
var DefaultTimeoutConfig = TimeoutConfig{
	ConnectionTimeout:  5 * time.Second,
	RequestTimeout:     10 * time.Second,
	HealthCheckTimeout: 3 * time.Second,
}