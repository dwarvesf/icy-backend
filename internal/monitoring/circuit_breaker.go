package monitoring

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/sony/gobreaker"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// CircuitBreakerBtcRPC wraps btcrpc.IBtcRpc with circuit breaker functionality
type CircuitBreakerBtcRPC struct {
	wrapped        btcrpc.IBtcRpc
	circuitBreaker *gobreaker.CircuitBreaker
	metrics        *ExternalAPIMetrics
	logger         *logger.Logger
	timeoutConfig  TimeoutConfig
}

// CircuitBreakerBaseRPC wraps baserpc.IBaseRPC with circuit breaker functionality
type CircuitBreakerBaseRPC struct {
	wrapped        baserpc.IBaseRPC
	circuitBreaker *gobreaker.CircuitBreaker
	metrics        *ExternalAPIMetrics
	logger         *logger.Logger
	timeoutConfig  TimeoutConfig
}

// NewCircuitBreakerBtcRPC creates a new circuit breaker wrapper for BTC RPC
func NewCircuitBreakerBtcRPC(wrapped btcrpc.IBtcRpc, config CircuitBreakerConfig, metrics *ExternalAPIMetrics, logger *logger.Logger) *CircuitBreakerBtcRPC {
	return NewCircuitBreakerBtcRPCWithTimeout(wrapped, config, DefaultTimeoutConfig, metrics, logger)
}

// NewCircuitBreakerBtcRPCWithTimeout creates a new circuit breaker wrapper for BTC RPC with custom timeout config
func NewCircuitBreakerBtcRPCWithTimeout(wrapped btcrpc.IBtcRpc, config CircuitBreakerConfig, timeoutConfig TimeoutConfig, metrics *ExternalAPIMetrics, logger *logger.Logger) *CircuitBreakerBtcRPC {
	cb := &CircuitBreakerBtcRPC{
		wrapped:       wrapped,
		metrics:       metrics,
		logger:        logger,
		timeoutConfig: timeoutConfig,
	}

	settings := gobreaker.Settings{
		Name:        "btc_rpc",
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(config.ConsecutiveFailureThreshold)
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info("Circuit breaker state change", map[string]string{
				"service": name,
				"from":    from.String(),
				"to":      to.String(),
			})
			metrics.UpdateCircuitBreakerState("btc_rpc", to)
		},
	}

	cb.circuitBreaker = gobreaker.NewCircuitBreaker(settings)
	return cb
}

// NewCircuitBreakerBaseRPC creates a new circuit breaker wrapper for Base RPC
func NewCircuitBreakerBaseRPC(wrapped baserpc.IBaseRPC, config CircuitBreakerConfig, metrics *ExternalAPIMetrics, logger *logger.Logger) *CircuitBreakerBaseRPC {
	return NewCircuitBreakerBaseRPCWithTimeout(wrapped, config, DefaultTimeoutConfig, metrics, logger)
}

// NewCircuitBreakerBaseRPCWithTimeout creates a new circuit breaker wrapper for Base RPC with custom timeout config
func NewCircuitBreakerBaseRPCWithTimeout(wrapped baserpc.IBaseRPC, config CircuitBreakerConfig, timeoutConfig TimeoutConfig, metrics *ExternalAPIMetrics, logger *logger.Logger) *CircuitBreakerBaseRPC {
	cb := &CircuitBreakerBaseRPC{
		wrapped:       wrapped,
		metrics:       metrics,
		logger:        logger,
		timeoutConfig: timeoutConfig,
	}

	settings := gobreaker.Settings{
		Name:        "base_rpc",
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(config.ConsecutiveFailureThreshold)
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info("Circuit breaker state change", map[string]string{
				"service": name,
				"from":    from.String(),
				"to":      to.String(),
			})
			metrics.UpdateCircuitBreakerState("base_rpc", to)
		},
	}

	cb.circuitBreaker = gobreaker.NewCircuitBreaker(settings)
	return cb
}

// executeWithTimeout executes a function with timeout and metrics recording
func (cb *CircuitBreakerBtcRPC) executeWithTimeout(operation string, fn func() (interface{}, error)) (interface{}, error) {
	start := time.Now()
	
	// Determine timeout based on operation type
	var timeout time.Duration
	switch operation {
	case "health_check":
		timeout = cb.timeoutConfig.HealthCheckTimeout
	default:
		timeout = cb.timeoutConfig.RequestTimeout
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	done := make(chan struct{})
	var result interface{}
	var err error
	
	go func() {
		defer close(done)
		result, err = fn()
	}()
	
	select {
	case <-done:
		duration := time.Since(start).Seconds()
		status := "success"
		if err != nil {
			status = "error"
			cb.logError("btc_rpc", operation, duration, err)
		}
		cb.metrics.RecordAPICall("btc_rpc", operation, status, duration)
		return result, err
		
	case <-ctx.Done():
		cb.metrics.RecordTimeout("btc_rpc", operation)
		cb.logError("btc_rpc", operation, time.Since(start).Seconds(), ctx.Err())
		return nil, fmt.Errorf("timeout: %v", ctx.Err())
	}
}

// executeWithTimeoutBase executes a function with timeout and metrics recording for Base RPC
func (cb *CircuitBreakerBaseRPC) executeWithTimeout(operation string, fn func() (interface{}, error)) (interface{}, error) {
	start := time.Now()
	
	// Determine timeout based on operation type
	var timeout time.Duration
	switch operation {
	case "health_check":
		timeout = cb.timeoutConfig.HealthCheckTimeout
	default:
		timeout = cb.timeoutConfig.RequestTimeout
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	done := make(chan struct{})
	var result interface{}
	var err error
	
	go func() {
		defer close(done)
		result, err = fn()
	}()
	
	select {
	case <-done:
		duration := time.Since(start).Seconds()
		status := "success"
		if err != nil {
			status = "error"
			cb.logError("base_rpc", operation, duration, err)
		}
		cb.metrics.RecordAPICall("base_rpc", operation, status, duration)
		return result, err
		
	case <-ctx.Done():
		cb.metrics.RecordTimeout("base_rpc", operation)
		cb.logError("base_rpc", operation, time.Since(start).Seconds(), ctx.Err())
		return nil, fmt.Errorf("timeout: %v", ctx.Err())
	}
}

// BTC RPC Methods with Circuit Breaker

func (cb *CircuitBreakerBtcRPC) Send(receiverAddress string, amount *model.Web3BigInt) (string, int64, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("send", func() (interface{}, error) {
			txHash, fee, err := cb.wrapped.Send(receiverAddress, amount)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"txHash": txHash,
				"fee":    fee,
			}, nil
		})
	})
	
	if err != nil {
		return "", 0, err
	}
	
	resultMap := result.(map[string]interface{})
	return resultMap["txHash"].(string), resultMap["fee"].(int64), nil
}

func (cb *CircuitBreakerBtcRPC) CurrentBalance() (*model.Web3BigInt, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("current_balance", func() (interface{}, error) {
			return cb.wrapped.CurrentBalance()
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.(*model.Web3BigInt), nil
}

func (cb *CircuitBreakerBtcRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("get_transactions", func() (interface{}, error) {
			return cb.wrapped.GetTransactionsByAddress(address, fromTxId)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.([]model.OnchainBtcTransaction), nil
}

func (cb *CircuitBreakerBtcRPC) EstimateFees() (map[string]float64, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("estimate_fees", func() (interface{}, error) {
			return cb.wrapped.EstimateFees()
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.(map[string]float64), nil
}

func (cb *CircuitBreakerBtcRPC) GetSatoshiUSDPrice() (float64, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("get_price", func() (interface{}, error) {
			return cb.wrapped.GetSatoshiUSDPrice()
		})
	})
	
	if err != nil {
		return 0, err
	}
	
	return result.(float64), nil
}

func (cb *CircuitBreakerBtcRPC) IsDust(address string, amount int64) bool {
	return cb.wrapped.IsDust(address, amount)
}

// Base RPC Methods with Circuit Breaker

func (cb *CircuitBreakerBaseRPC) Client() *ethclient.Client {
	return cb.wrapped.Client()
}

func (cb *CircuitBreakerBaseRPC) GetContractAddress() common.Address {
	return cb.wrapped.GetContractAddress()
}

func (cb *CircuitBreakerBaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("icy_balance_of", func() (interface{}, error) {
			return cb.wrapped.ICYBalanceOf(address)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.(*model.Web3BigInt), nil
}

func (cb *CircuitBreakerBaseRPC) ICYTotalSupply() (*model.Web3BigInt, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("icy_total_supply", func() (interface{}, error) {
			return cb.wrapped.ICYTotalSupply()
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.(*model.Web3BigInt), nil
}

func (cb *CircuitBreakerBaseRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainIcyTransaction, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("get_transactions", func() (interface{}, error) {
			return cb.wrapped.GetTransactionsByAddress(address, fromTxId)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.([]model.OnchainIcyTransaction), nil
}

func (cb *CircuitBreakerBaseRPC) Swap(icyAmount *model.Web3BigInt, btcAddress string, btcAmount *model.Web3BigInt) (*types.Transaction, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("swap", func() (interface{}, error) {
			return cb.wrapped.Swap(icyAmount, btcAddress, btcAmount)
		})
	})
	
	if err != nil {
		return nil, err
	}
	
	return result.(*types.Transaction), nil
}

func (cb *CircuitBreakerBaseRPC) GenerateSignature(icyAmount *model.Web3BigInt, btcAddress string, btcAmount *model.Web3BigInt, nonce *big.Int, deadline *big.Int) (string, error) {
	result, err := cb.circuitBreaker.Execute(func() (interface{}, error) {
		return cb.executeWithTimeout("generate_signature", func() (interface{}, error) {
			return cb.wrapped.GenerateSignature(icyAmount, btcAddress, btcAmount, nonce, deadline)
		})
	})
	
	if err != nil {
		return "", err
	}
	
	return result.(string), nil
}

// Helper functions

func (cb *CircuitBreakerBtcRPC) logError(service, operation string, duration float64, err error) {
	cb.logger.Error("External API call failed", map[string]string{
		"service":    service,
		"operation":  operation,
		"duration":   strconv.FormatFloat(duration, 'f', 3, 64),
		"error":      err.Error(),
		"error_type": string(classifyError(err)),
		"cb_state":   cb.circuitBreaker.State().String(),
	})
}

func (cb *CircuitBreakerBaseRPC) logError(service, operation string, duration float64, err error) {
	cb.logger.Error("External API call failed", map[string]string{
		"service":    service,
		"operation":  operation,
		"duration":   strconv.FormatFloat(duration, 'f', 3, 64),
		"error":      err.Error(),
		"error_type": string(classifyError(err)),
		"cb_state":   cb.circuitBreaker.State().String(),
	})
}

// classifyError classifies errors into different types for metrics and logging
func classifyError(err error) APIErrorType {
	if err == nil {
		return ""
	}
	
	errMsg := strings.ToLower(err.Error())
	
	// Timeout errors
	if strings.Contains(errMsg, "timeout") || 
	   strings.Contains(errMsg, "deadline exceeded") ||
	   strings.Contains(errMsg, "context canceled") {
		return ErrorTypeTimeout
	}
	
	// Network errors
	if strings.Contains(errMsg, "network") ||
	   strings.Contains(errMsg, "connection") ||
	   strings.Contains(errMsg, "unreachable") ||
	   strings.Contains(errMsg, "dns") {
		return ErrorTypeNetworkError
	}
	
	// Server errors (5xx)
	if strings.Contains(errMsg, "500") ||
	   strings.Contains(errMsg, "502") ||
	   strings.Contains(errMsg, "503") ||
	   strings.Contains(errMsg, "504") ||
	   strings.Contains(errMsg, "internal server error") ||
	   strings.Contains(errMsg, "bad gateway") ||
	   strings.Contains(errMsg, "service unavailable") ||
	   strings.Contains(errMsg, "gateway timeout") {
		return ErrorTypeServerError
	}
	
	// Client errors (4xx)
	if strings.Contains(errMsg, "400") ||
	   strings.Contains(errMsg, "401") ||
	   strings.Contains(errMsg, "403") ||
	   strings.Contains(errMsg, "404") ||
	   strings.Contains(errMsg, "429") ||
	   strings.Contains(errMsg, "bad request") ||
	   strings.Contains(errMsg, "unauthorized") ||
	   strings.Contains(errMsg, "forbidden") ||
	   strings.Contains(errMsg, "not found") ||
	   strings.Contains(errMsg, "rate limit") {
		return ErrorTypeClientError
	}
	
	return ErrorTypeUnknown
}

// validateCircuitBreakerConfig validates circuit breaker configuration
func validateCircuitBreakerConfig(config CircuitBreakerConfig) error {
	if config.MaxRequests == 0 {
		return fmt.Errorf("max_requests must be greater than 0")
	}
	
	if config.ConsecutiveFailureThreshold <= 0 {
		return fmt.Errorf("consecutive_failure_threshold must be greater than 0")
	}
	
	if config.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}
	
	if config.Interval < 0 {
		return fmt.Errorf("interval must be non-negative")
	}
	
	return nil
}