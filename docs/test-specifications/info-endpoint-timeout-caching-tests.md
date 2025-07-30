# Test Specifications: /info Endpoint Timeout and Caching Improvements

## Overview

This document defines comprehensive test cases for implementing timeout handling and caching improvements to fix the `/info` endpoint timeout issue. The tests follow Test-Driven Development (TDD) principles and will guide the implementation of the complete solution.

## Problem Statement

The `/info` endpoint currently times out with "context deadline exceeded" due to:
1. **Missing caching** for `GetCirculatedICY()` and `GetBTCSupply()` operations
2. **15-second timeout insufficient** for 3 parallel complex operations
3. **Fail-fast architecture** with no graceful degradation
4. **Complex operations** involving multiple RPC calls, DB queries, and external APIs

## Test Files Created

### 1. Handler Level Tests
**File**: `/internal/handler/swap/swap_info_timeout_test.go`

#### Test Categories:
- **Timeout Handling Tests**
  - Current 15-second timeout failure scenarios
  - Enhanced 45-second timeout success scenarios
  - Context cancellation handling
  - Resource cleanup verification

- **Caching Layer Tests**
  - `GetCirculatedICY()` caching (5-minute TTL)
  - `GetBTCSupply()` caching (5-minute TTL)
  - Cache miss graceful handling
  - Performance optimization validation

- **Graceful Degradation Tests**
  - Partial data return when operations fail
  - Meaningful error messages for partial failures
  - Service unavailable when all operations fail

- **Background Refresh Pattern Tests**
  - Stale cache return with async refresh
  - Background refresh failure handling

- **Edge Cases and Error Scenarios**
  - Concurrent request handling
  - Goroutine leak prevention
  - Cache corruption handling

#### Key Test Scenarios:
```go
// Example test structure
It("should complete successfully within 45 seconds for complex operations", func() {
    // Mock operations that take 20-25 seconds each but complete
    mockBtcRPC.On("GetSatoshiUSDPrice").Return(100000.0, nil).After(8 * time.Second)
    mockOracle.On("GetCirculatedICY").Return(expectedICY, nil).After(10 * time.Second)
    mockOracle.On("GetBTCSupply").Return(expectedBTC, nil).After(12 * time.Second)
    
    // Should complete within 45 seconds and return success
    Expect(w.Code).To(Equal(http.StatusOK))
    Expect(duration).To(BeNumerically("<=", 45*time.Second))
})
```

### 2. Oracle Layer Tests
**File**: `/internal/oracle/oracle_caching_test.go`

#### Test Categories:
- **Cache Miss Behavior**
  - Fresh data fetching on first call
  - Error handling during data fetch
  - Mochi Pay API integration caching

- **Cache Hit Behavior**
  - Cached data return within cache window
  - Cache performance improvements

- **Cache Expiration**
  - 5-minute TTL enforcement
  - Automatic refresh after expiration

- **Performance Tests**
  - Response time improvements with cache
  - Memory efficiency validation

- **Cache Management**
  - Manual cache invalidation
  - Selective cache clearing
  - Cache corruption handling

- **Concurrent Access Tests**
  - Thread-safe cache operations
  - Race condition prevention

### 3. Integration Tests
**File**: `/internal/handler/swap/swap_info_integration_test.go`

#### Test Categories:
- **End-to-End Solution Testing**
  - Complete timeout and caching solution validation
  - Real-world failure scenario simulation
  - Circuit breaker integration

- **Enhanced Mock with Circuit Breaker**
  ```go
  type EnhancedMockOracle struct {
      circuitState     CircuitBreakerState
      failureCount     int
      lastFailureTime  time.Time
      // ... circuit breaker logic
  }
  ```

- **Performance Under Load**
  - 100 concurrent request handling
  - Memory pressure scenarios
  - High-load response times

- **Real-world Scenarios**
  - Network instability simulation
  - Intermittent failures
  - Circuit breaker recovery

- **Monitoring and Observability**
  - Performance metrics collection
  - Cache hit/miss ratio tracking
  - Response format consistency

### 4. Enhanced Oracle Interface Tests
**File**: `/internal/oracle/interface_enhanced_test.go`

#### New Interface Methods to Implement:
```go
type IEnhancedOracle interface {
    oracle.IOracle // Embed existing interface
    
    // Cached methods with timeout and fallback support
    GetCachedCirculatedICY() (*model.Web3BigInt, error)
    GetCachedBTCSupply() (*model.Web3BigInt, error)
    
    // Context-aware methods for timeout handling
    GetCirculatedICYWithContext(ctx context.Context) (*model.Web3BigInt, error)
    GetBTCSupplyWithContext(ctx context.Context) (*model.Web3BigInt, error)
    
    // Background refresh methods
    RefreshCirculatedICYAsync() error
    RefreshBTCSupplyAsync() error
    
    // Cache management
    ClearCirculatedICYCache() error
    ClearBTCSupplyCache() error
    ClearAllCaches() error
    
    // Health monitoring
    IsCirculatedICYCacheHealthy() bool
    IsBTCSupplyCacheHealthy() bool
    GetCacheStatistics() *CacheStatistics
}
```

#### Test Categories:
- **Cached Data Retrieval**
- **Context-Aware Operations**
- **Background Refresh Operations**
- **Cache Management Operations**
- **Statistics and Monitoring**
- **Error Handling and Edge Cases**

### 5. BTC RPC Caching Tests
**File**: `/internal/btcrpc/caching_improvements_test.go`

#### Test Categories:
- **GetSatoshiUSDPrice Enhanced Caching**
  - Current 1-minute cache validation
  - Stale-while-revalidate pattern
  - Background refresh implementation

- **New Caching for Other Operations**
  - `CurrentBalance()` caching (1-2 minutes)
  - Fee estimation caching (30 seconds)
  - Endpoint-specific cache behavior

- **Performance Optimization**
  - Memory-efficient caching
  - LRU eviction policies
  - Cache warming strategies

- **Multi-endpoint Integration**
  - Endpoint failover with cache preservation
  - Cache sharing across healthy endpoints

- **Monitoring and Debugging**
  - Cache metrics tracking
  - Performance monitoring
  - Health status endpoints

## Implementation Requirements

### 1. Timeout Handling
- [ ] Increase timeout from 15s to 45s
- [ ] Implement proper context cancellation
- [ ] Add goroutine cleanup mechanisms
- [ ] Implement resource leak prevention

### 2. Caching Layer
```go
// New cache methods needed in Oracle
func (o *IcyOracle) GetCachedCirculatedICY() (*model.Web3BigInt, error) {
    // Try cache first, fallback to fresh data
    if cached, found := o.cache.Get("circulated_icy"); found {
        return cached.(*model.Web3BigInt), nil
    }
    
    // Cache miss - fetch fresh data
    result, err := o.GetCirculatedICY()
    if err == nil {
        o.cache.Set("circulated_icy", result, 5*time.Minute)
    }
    return result, err
}
```

### 3. Graceful Degradation
```go
// Enhanced info handler with partial failure support
func (h *handler) InfoWithGracefulDegradation(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
    defer cancel()

    // Collect partial results
    results := make(map[string]interface{})
    warnings := make([]string, 0)
    
    // Try each operation, collect what works
    if icyResult, err := h.oracle.GetCachedCirculatedICY(); err == nil {
        results["circulated_icy_balance"] = icyResult.Value
    } else {
        warnings = append(warnings, "ICY data unavailable: " + err.Error())
    }
    
    // Return partial data with warnings
    response := map[string]interface{}{
        "data": results,
        "warnings": warnings,
    }
    
    statusCode := http.StatusOK
    if len(results) == 0 {
        statusCode = http.StatusServiceUnavailable
    }
    
    c.JSON(statusCode, response)
}
```

### 4. Background Refresh Pattern
```go
// Background refresh implementation
func (o *IcyOracle) startBackgroundRefresh() {
    go func() {
        ticker := time.NewTicker(4 * time.Minute) // Refresh before 5min expiry
        defer ticker.Stop()
        
        for range ticker.C {
            // Refresh in background, don't block
            go o.refreshCirculatedICYAsync()
            go o.refreshBTCSupplyAsync()
        }
    }()
}
```

## Test Execution Strategy

### Phase 1: Core Timeout Tests
1. Run timeout handling tests to establish baseline
2. Verify current 15-second timeout behavior
3. Implement 45-second timeout solution
4. Validate timeout improvement tests

### Phase 2: Caching Implementation
1. Implement Oracle caching methods
2. Run cache behavior tests
3. Validate cache TTL and performance
4. Test cache miss/hit scenarios

### Phase 3: Graceful Degradation
1. Implement partial failure handling
2. Run graceful degradation tests
3. Validate error messaging
4. Test service availability scenarios

### Phase 4: Integration Testing
1. Run end-to-end integration tests
2. Validate complete solution performance
3. Test under load conditions
4. Verify monitoring capabilities

### Phase 5: BTC RPC Enhancements
1. Implement enhanced BTC RPC caching
2. Run BTC RPC caching tests
3. Validate multi-endpoint behavior
4. Test performance improvements

## Success Criteria

### Performance Targets
- [ ] `/info` endpoint completes in <45 seconds for complex operations
- [ ] Cached responses complete in <2 seconds
- [ ] 100 concurrent requests handled efficiently
- [ ] Memory usage remains stable under load

### Reliability Targets
- [ ] Graceful degradation when 1-2 operations fail
- [ ] Service remains available with partial data
- [ ] No goroutine leaks during timeout scenarios
- [ ] Cache corruption handled gracefully

### Monitoring Targets
- [ ] Cache hit ratio >80% in steady state
- [ ] Response time metrics tracked
- [ ] Error rates monitored per operation
- [ ] Cache performance statistics available

## Running the Tests

```bash
# Run all timeout and caching tests
go test ./internal/handler/swap/... -v -run="Info.*Timeout|Info.*Caching"

# Run oracle caching tests
go test ./internal/oracle/... -v -run=".*Caching.*"

# Run integration tests
go test ./internal/handler/swap/... -v -run=".*Integration.*"

# Run BTC RPC caching tests
go test ./internal/btcrpc/... -v -run=".*Caching.*"

# Run with race detection
go test -race ./internal/handler/swap/... ./internal/oracle/... ./internal/btcrpc/...
```

## Monitoring and Alerting

### Metrics to Track
- Response time percentiles (p50, p95, p99)
- Cache hit/miss ratios
- Timeout occurrence rates
- Partial failure rates
- Memory usage trends

### Alerts to Configure
- `/info` endpoint timeout rate >5%
- Cache hit ratio <70%
- All data sources failing
- Memory usage >80% of limit
- Response time p95 >10 seconds

## Conclusion

These comprehensive test cases provide a complete roadmap for implementing timeout handling and caching improvements for the `/info` endpoint. The tests are designed to:

1. **Validate current problems** - Demonstrate existing timeout issues
2. **Guide implementation** - Define exact behavior expected
3. **Ensure reliability** - Cover edge cases and error scenarios
4. **Verify performance** - Confirm improvements meet targets
5. **Enable monitoring** - Provide observability into system behavior

By following TDD principles with these test cases, the implementation will be robust, well-tested, and maintainable.