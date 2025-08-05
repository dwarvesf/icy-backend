# Task 3: Application-Level Metrics Implementation Specification

**Date**: 2025-08-05  
**Task**: Application-Level Metrics (Instrumentation Monitoring)  
**Priority**: High  
**Estimated Effort**: 5-6 days  

## Overview

Implement comprehensive Prometheus metrics collection for HTTP requests, business logic operations (Oracle data, Swap operations), and external API calls. This provides instrumentation monitoring for performance analysis, business insights, and system optimization.

## Functional Requirements

### 1. HTTP Request Metrics

**Purpose**: Monitor HTTP endpoint performance and usage patterns  
**Implementation**: Gin middleware for automatic instrumentation  
**Performance Target**: < 1ms overhead per request  

**Metrics to Collect**:
- Request duration (histogram)
- Request count (counter)  
- Active requests (gauge)
- Error rates by endpoint and status code

### 2. Business Logic Metrics

**Purpose**: Monitor cryptocurrency-specific operations and oracle data  
**Implementation**: Instrumentation within business logic methods  
**Focus Areas**: Oracle calculations, swap operations, financial data freshness  

**Metrics to Collect**:
- Oracle data age and calculation duration
- Swap operation counts and processing times
- Financial operation success/failure rates

### 3. External API Metrics Integration

**Purpose**: Integrate with Task 2 circuit breaker metrics  
**Implementation**: Metrics collection within circuit breaker wrappers  
**Focus Areas**: API performance, failure rates, circuit breaker states  

## Technical Specification

### 1. HTTP Metrics Middleware

```go
package monitoring

import (
    "strconv"
    "strings"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"
)

type HTTPMetrics struct {
    requestsTotal   *prometheus.CounterVec
    requestDuration *prometheus.HistogramVec
    activeRequests  prometheus.Gauge
    requestSize     *prometheus.HistogramVec
    responseSize    *prometheus.HistogramVec
}

func NewHTTPMetrics() *HTTPMetrics {
    return &HTTPMetrics{
        requestsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_http_requests_total",
                Help: "Total number of HTTP requests",
            },
            []string{"method", "endpoint", "status"},
        ),
        requestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_http_request_duration_seconds",
                Help: "HTTP request duration in seconds",
                Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
            },
            []string{"method", "endpoint"},
        ),
        activeRequests: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_http_active_requests",
                Help: "Current number of active HTTP requests",
            },
        ),
        requestSize: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_http_request_size_bytes",
                Help: "HTTP request size in bytes",
                Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
            },
            []string{"method", "endpoint"},
        ),
        responseSize: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_http_response_size_bytes",
                Help: "HTTP response size in bytes",
                Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
            },
            []string{"method", "endpoint"},
        ),
    }
}

func (m *HTTPMetrics) MustRegister(registry *prometheus.Registry) {
    registry.MustRegister(
        m.requestsTotal,
        m.requestDuration,
        m.activeRequests,
        m.requestSize,
        m.responseSize,
    )
}

func (m *HTTPMetrics) Middleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        // Increment active requests
        m.activeRequests.Inc()
        defer m.activeRequests.Dec()
        
        // Record request size
        if c.Request.ContentLength > 0 {
            m.requestSize.WithLabelValues(
                c.Request.Method,
                normalizeEndpoint(c.FullPath()),
            ).Observe(float64(c.Request.ContentLength))
        }
        
        // Process request
        c.Next()
        
        // Record metrics after request completion
        duration := time.Since(start).Seconds()
        method := c.Request.Method
        endpoint := normalizeEndpoint(c.FullPath())
        status := strconv.Itoa(c.Writer.Status())
        
        // Record request count and duration
        m.requestsTotal.WithLabelValues(method, endpoint, status).Inc()
        m.requestDuration.WithLabelValues(method, endpoint).Observe(duration)
        
        // Record response size
        responseSize := c.Writer.Size()
        if responseSize > 0 {
            m.responseSize.WithLabelValues(method, endpoint).Observe(float64(responseSize))
        }
    }
}

// normalizeEndpoint groups similar endpoints to control cardinality
func normalizeEndpoint(path string) string {
    if path == "" {
        return "unknown"
    }
    
    switch {
    case path == "/healthz":
        return "/healthz"
    case strings.HasPrefix(path, "/api/v1/health"):
        return "/api/v1/health/*"
    case strings.HasPrefix(path, "/api/v1/oracle"):
        return "/api/v1/oracle/*"
    case strings.HasPrefix(path, "/api/v1/swap"):
        return "/api/v1/swap/*"
    case strings.HasPrefix(path, "/api/v1/transactions"):
        return "/api/v1/transactions"
    case strings.HasPrefix(path, "/swagger"):
        return "/swagger/*"
    case strings.HasPrefix(path, "/metrics"):
        return "/metrics"
    default:
        return "other"
    }
}
```

### 2. Business Logic Metrics

```go
package monitoring

type BusinessMetrics struct {
    // Oracle metrics
    oracleDataAge             *prometheus.GaugeVec
    oracleCalculationDuration *prometheus.HistogramVec
    oracleOperations          *prometheus.CounterVec
    
    // Swap metrics
    swapOperationsTotal       *prometheus.CounterVec
    swapProcessingDuration    *prometheus.HistogramVec
    swapAmountDistribution    *prometheus.HistogramVec
    
    // Treasury metrics
    treasuryBalance           *prometheus.GaugeVec
    circulatedSupply          prometheus.Gauge
}

func NewBusinessMetrics() *BusinessMetrics {
    return &BusinessMetrics{
        oracleDataAge: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_oracle_data_age_seconds",
                Help: "Age of oracle data in seconds",
            },
            []string{"data_type"}, // "btc_price", "icy_supply", "ratio"
        ),
        oracleCalculationDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_oracle_calculation_duration_seconds",
                Help: "Oracle calculation duration in seconds",
                Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2},
            },
            []string{"calculation_type"}, // "ratio", "treasury", "circulation"
        ),
        oracleOperations: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_oracle_operations_total",
                Help: "Total oracle operations",
            },
            []string{"operation", "status"}, // operation: "calculate", "fetch", "cache"
        ),
        swapOperationsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_swap_operations_total",
                Help: "Total swap operations",
            },
            []string{"operation", "status"}, // operation: "create", "process", "complete"
        ),
        swapProcessingDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_swap_processing_duration_seconds",
                Help: "Swap processing duration in seconds",
                Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
            },
            []string{"operation"}, // "validation", "signature", "execution"
        ),
        swapAmountDistribution: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_swap_amount_distribution",
                Help: "Distribution of swap amounts (sanitized ranges)",
                Buckets: []float64{0.001, 0.01, 0.1, 1, 10, 100, 1000}, // BTC amounts
            },
            []string{"currency"}, // "btc", "icy"
        ),
        treasuryBalance: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_treasury_balance",
                Help: "Current treasury balance",
            },
            []string{"currency"}, // "btc", "icy"
        ),
        circulatedSupply: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_circulated_supply",
                Help: "Current circulated ICY supply",
            },
        ),
    }
}

func (m *BusinessMetrics) MustRegister(registry *prometheus.Registry) {
    registry.MustRegister(
        m.oracleDataAge,
        m.oracleCalculationDuration,
        m.oracleOperations,
        m.swapOperationsTotal,
        m.swapProcessingDuration,
        m.swapAmountDistribution,
        m.treasuryBalance,
        m.circulatedSupply,
    )
}
```

### 3. Oracle Handler Instrumentation

```go
package oracle

import (
    "time"
    
    "github.com/dwarvesf/icy-backend/internal/monitoring"
)

type InstrumentedOracleHandler struct {
    handler.IOracleHandler
    metrics *monitoring.BusinessMetrics
    logger  *logger.Logger
}

func NewInstrumentedOracleHandler(
    handler handler.IOracleHandler,
    metrics *monitoring.BusinessMetrics,
    logger *logger.Logger,
) *InstrumentedOracleHandler {
    return &InstrumentedOracleHandler{
        IOracleHandler: handler,
        metrics:        metrics,
        logger:         logger,
    }
}

func (ioh *InstrumentedOracleHandler) GetCirculatedICY(c *gin.Context) {
    start := time.Now()
    operation := "circulated_icy"
    
    // Execute original handler
    ioh.IOracleHandler.GetCirculatedICY(c)
    
    // Record metrics
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ioh.metrics.oracleCalculationDuration.WithLabelValues("circulation").Observe(duration.Seconds())
    ioh.metrics.oracleOperations.WithLabelValues(operation, status).Inc()
    
    // Update data age if successful
    if status == "success" {
        ioh.metrics.oracleDataAge.WithLabelValues("icy_supply").Set(0) // Fresh data
    }
    
    ioh.logger.Info("Oracle operation completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
    })
}

func (ioh *InstrumentedOracleHandler) GetTreasusyBTC(c *gin.Context) {
    start := time.Now()
    operation := "treasury_btc"
    
    ioh.IOracleHandler.GetTreasusyBTC(c)
    
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ioh.metrics.oracleCalculationDuration.WithLabelValues("treasury").Observe(duration.Seconds())
    ioh.metrics.oracleOperations.WithLabelValues(operation, status).Inc()
    
    if status == "success" {
        ioh.metrics.oracleDataAge.WithLabelValues("btc_treasury").Set(0)
    }
    
    ioh.logger.Info("Oracle operation completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
    })
}

func (ioh *InstrumentedOracleHandler) GetICYBTCRatio(c *gin.Context) {
    start := time.Now()
    operation := "icy_btc_ratio"
    
    ioh.IOracleHandler.GetICYBTCRatio(c)
    
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ioh.metrics.oracleCalculationDuration.WithLabelValues("ratio").Observe(duration.Seconds())
    ioh.metrics.oracleOperations.WithLabelValues(operation, status).Inc()
    
    if status == "success" {
        ioh.metrics.oracleDataAge.WithLabelValues("ratio").Set(0)
    }
    
    ioh.logger.Info("Oracle operation completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
    })
}

func (ioh *InstrumentedOracleHandler) GetICYBTCRatioCached(c *gin.Context) {
    start := time.Now()
    operation := "icy_btc_ratio_cached"
    
    ioh.IOracleHandler.GetICYBTCRatioCached(c)
    
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ioh.metrics.oracleCalculationDuration.WithLabelValues("ratio_cached").Observe(duration.Seconds())
    ioh.metrics.oracleOperations.WithLabelValues(operation, status).Inc()
    
    ioh.logger.Info("Oracle operation completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
        "cached":    "true",
    })
}
```

### 4. Swap Handler Instrumentation

```go
package swap

import (
    "encoding/json"
    "math/big"
    "time"
    
    "github.com/dwarvesf/icy-backend/internal/monitoring"
    "github.com/dwarvesf/icy-backend/internal/model"
)

type InstrumentedSwapHandler struct {
    handler.ISwapHandler
    metrics       *monitoring.BusinessMetrics
    logger        *logger.Logger
    dataSanitizer *monitoring.DataSanitizer
}

func NewInstrumentedSwapHandler(
    handler handler.ISwapHandler,
    metrics *monitoring.BusinessMetrics,
    logger *logger.Logger,
) *InstrumentedSwapHandler {
    return &InstrumentedSwapHandler{
        ISwapHandler:  handler,
        metrics:       metrics,
        logger:        logger,
        dataSanitizer: monitoring.NewDataSanitizer(logger),
    }
}

func (ish *InstrumentedSwapHandler) CreateSwapRequest(c *gin.Context) {
    start := time.Now()
    operation := "create_swap_request"
    
    // Capture request data for metrics (before handler execution)
    var requestData map[string]interface{}
    if body, err := c.GetRawData(); err == nil {
        json.Unmarshal(body, &requestData)
        // Reset body for handler
        c.Request.Body = io.NopCloser(bytes.NewReader(body))
    }
    
    // Execute original handler
    ish.ISwapHandler.CreateSwapRequest(c)
    
    // Record metrics
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ish.metrics.swapProcessingDuration.WithLabelValues("validation").Observe(duration.Seconds())
    ish.metrics.swapOperationsTotal.WithLabelValues(operation, status).Inc()
    
    // Record sanitized amount distribution if available
    if requestData != nil {
        if btcAmount, ok := requestData["btc_amount"].(string); ok {
            if amount, success := new(big.Float).SetString(btcAmount); success {
                btcFloat, _ := amount.Float64()
                ish.metrics.swapAmountDistribution.WithLabelValues("btc").Observe(btcFloat)
            }
        }
        
        if icyAmount, ok := requestData["icy_amount"].(string); ok {
            if amount, success := new(big.Float).SetString(icyAmount); success {
                icyFloat, _ := amount.Float64()
                ish.metrics.swapAmountDistribution.WithLabelValues("icy").Observe(icyFloat)
            }
        }
    }
    
    ish.logger.Info("Swap operation completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
    })
}

func (ish *InstrumentedSwapHandler) GenerateSignature(c *gin.Context) {
    start := time.Now()
    operation := "generate_signature"
    
    ish.ISwapHandler.GenerateSignature(c)
    
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ish.metrics.swapProcessingDuration.WithLabelValues("signature").Observe(duration.Seconds())
    ish.metrics.swapOperationsTotal.WithLabelValues(operation, status).Inc()
    
    ish.logger.Info("Signature generation completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
    })
}

func (ish *InstrumentedSwapHandler) Info(c *gin.Context) {
    start := time.Now()
    operation := "swap_info"
    
    ish.ISwapHandler.Info(c)
    
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    ish.metrics.swapProcessingDuration.WithLabelValues("info").Observe(duration.Seconds())
    ish.metrics.swapOperationsTotal.WithLabelValues(operation, status).Inc()
}
```

### 5. Transaction Handler Instrumentation

```go
package transaction

type InstrumentedTransactionHandler struct {
    handler.ITransactionHandler
    metrics *monitoring.BusinessMetrics
    logger  *logger.Logger
}

func NewInstrumentedTransactionHandler(
    handler handler.ITransactionHandler,
    metrics *monitoring.BusinessMetrics,
    logger *logger.Logger,
) *InstrumentedTransactionHandler {
    return &InstrumentedTransactionHandler{
        ITransactionHandler: handler,
        metrics:            metrics,
        logger:             logger,
    }
}

func (ith *InstrumentedTransactionHandler) GetTransactions(c *gin.Context) {
    start := time.Now()
    operation := "get_transactions"
    
    ith.ITransactionHandler.GetTransactions(c)
    
    duration := time.Since(start)
    status := "success"
    if c.Writer.Status() >= 400 {
        status = "error"
    }
    
    // Record as oracle operation since it's data retrieval
    ith.metrics.oracleOperations.WithLabelValues(operation, status).Inc()
    ith.metrics.oracleCalculationDuration.WithLabelValues("transaction_query").Observe(duration.Seconds())
    
    ith.logger.Info("Transaction query completed", map[string]string{
        "operation": operation,
        "duration":  duration.String(),
        "status":    status,
    })
}
```

### 6. Metrics Registry and HTTP Handler

```go
package monitoring

import (
    "net/http"
    
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsRegistry struct {
    registry        *prometheus.Registry
    httpMetrics     *HTTPMetrics
    businessMetrics *BusinessMetrics
    externalMetrics *ExternalAPIMetrics
}

func NewMetricsRegistry() *MetricsRegistry {
    registry := prometheus.NewRegistry()
    
    httpMetrics := NewHTTPMetrics()
    businessMetrics := NewBusinessMetrics()
    externalMetrics := NewExternalAPIMetrics()
    
    // Register all metrics
    httpMetrics.MustRegister(registry)
    businessMetrics.MustRegister(registry)
    externalMetrics.MustRegister(registry)
    
    return &MetricsRegistry{
        registry:        registry,
        httpMetrics:     httpMetrics,
        businessMetrics: businessMetrics,
        externalMetrics: externalMetrics,
    }
}

func (mr *MetricsRegistry) HTTPMetrics() *HTTPMetrics {
    return mr.httpMetrics
}

func (mr *MetricsRegistry) BusinessMetrics() *BusinessMetrics {
    return mr.businessMetrics
}

func (mr *MetricsRegistry) ExternalMetrics() *ExternalAPIMetrics {
    return mr.externalMetrics
}

func (mr *MetricsRegistry) Handler() http.Handler {
    return promhttp.HandlerFor(
        mr.registry,
        promhttp.HandlerOpts{
            Registry:      mr.registry,
            Timeout:       5 * time.Second,
            ErrorHandling: promhttp.ContinueOnError,
        },
    )
}

func (mr *MetricsRegistry) GetMetricsCount() int {
    metricFamilies, _ := mr.registry.Gather()
    count := 0
    for _, mf := range metricFamilies {
        count += len(mf.GetMetric())
    }
    return count
}
```

## Integration Requirements

### 1. Server Initialization Update

```go
// In internal/server/server.go
func Init() {
    // ... existing initialization
    
    // Create metrics registry
    metricsRegistry := monitoring.NewMetricsRegistry()
    
    // Create instrumented handlers
    oracleHandler := oracle.NewInstrumentedOracleHandler(
        oracle.New(/* existing params */),
        metricsRegistry.BusinessMetrics(),
        logger,
    )
    
    swapHandler := swap.NewInstrumentedSwapHandler(
        swap.New(/* existing params */),
        metricsRegistry.BusinessMetrics(),
        logger,
    )
    
    transactionHandler := transaction.NewInstrumentedTransactionHandler(
        transaction.New(/* existing params */),
        metricsRegistry.BusinessMetrics(),
        logger,
    )
    
    // ... rest of initialization
    
    httpServer := http.NewHttpServer(
        appConfig,
        logger,
        oracleHandler, // Use instrumented handlers
        baseRpcWithCB,
        btcRpcWithCB,
        db,
        metricsRegistry, // Pass metrics registry
    )
    httpServer.Run()
}
```

### 2. HTTP Server Integration

```go
// In internal/transport/http/http.go
func NewHttpServer(
    appConfig *config.AppConfig, 
    logger *logger.Logger,
    oracle oracle.IOracle, 
    baseRPC baserpc.IBaseRPC, 
    btcRPC btcrpc.IBtcRpc,
    db *gorm.DB,
    metricsRegistry *monitoring.MetricsRegistry, // Add metrics registry
) *gin.Engine {
    r := gin.New()
    r.Use(
        gin.LoggerWithWriter(gin.DefaultWriter, "/healthz", "/metrics"),
        gin.Recovery(),
        metricsRegistry.HTTPMetrics().Middleware(), // Add metrics middleware
    )
    setupCORS(r, appConfig)
    
    // Add API key middleware
    r.Use(apiKeyMiddleware(appConfig))
    
    // Add metrics endpoint
    r.GET("/metrics", gin.WrapH(metricsRegistry.Handler()))
    
    // ... rest of setup
    
    return r
}
```

### 3. Handler Factory Update

```go
// In internal/handler/handler.go
type Handler struct {
    OracleHandler      oracle.IOracleHandler      // Use interface types
    SwapHandler        swap.ISwapHandler
    TransactionHandler transaction.ITransactionHandler
    HealthHandler      health.IHealthHandler
}

func New(
    appConfig *config.AppConfig,
    logger *logger.Logger,
    oracle oracle.IOracle,
    baseRPC baserpc.IBaseRPC,
    btcRPC btcrpc.IBtcRpc,
    db *gorm.DB,
    metricsRegistry *monitoring.MetricsRegistry,
) *Handler {
    // Create base handlers
    baseOracleHandler := oracle.New(appConfig, logger, oracle)
    baseSwapHandler := swap.New(appConfig, logger, oracle, baseRPC, btcRPC, db)
    baseTransactionHandler := transaction.New(appConfig, logger, db)
    
    return &Handler{
        OracleHandler: oracle.NewInstrumentedOracleHandler(
            baseOracleHandler,
            metricsRegistry.BusinessMetrics(),
            logger,
        ),
        SwapHandler: swap.NewInstrumentedSwapHandler(
            baseSwapHandler,  
            metricsRegistry.BusinessMetrics(),
            logger,
        ),
        TransactionHandler: transaction.NewInstrumentedTransactionHandler(
            baseTransactionHandler,
            metricsRegistry.BusinessMetrics(),
            logger,
        ),
        HealthHandler: health.New(appConfig, logger, db, btcRPC, baseRPC),
    }
}
```

## Data Sanitization Strategy

### 1. Sensitive Data Protection

```go
package monitoring

import (
    "crypto/sha256"
    "fmt"
    "math/big"
    "regexp"
)

type DataSanitizer struct {
    logger *logger.Logger
}

func NewDataSanitizer(logger *logger.Logger) *DataSanitizer {
    return &DataSanitizer{logger: logger}
}

func (ds *DataSanitizer) SanitizeAddress(address string) string {
    if len(address) <= 16 {
        return "[REDACTED]"
    }
    return address[:8] + "..." + address[len(address)-8:]
}

func (ds *DataSanitizer) SanitizeAmount(amount *model.Web3BigInt) string {
    if amount == nil {
        return "unknown"
    }
    
    val := amount.Value()
    
    // Convert to BTC units for comparison
    btcVal := new(big.Float).SetInt(val)
    btcVal.Quo(btcVal, big.NewFloat(100000000)) // Convert satoshi to BTC
    
    switch {
    case btcVal.Cmp(big.NewFloat(0.001)) < 0:
        return "micro"
    case btcVal.Cmp(big.NewFloat(0.01)) < 0:
        return "small"
    case btcVal.Cmp(big.NewFloat(0.1)) < 0:
        return "medium"
    case btcVal.Cmp(big.NewFloat(1.0)) < 0:
        return "large"
    default:
        return "xlarge"
    }
}

func (ds *DataSanitizer) HashSensitiveData(data string) string {
    hash := sha256.Sum256([]byte(data))
    return fmt.Sprintf("hash_%x", hash[:8]) // First 8 bytes as hex
}

func (ds *DataSanitizer) SanitizeErrorMessage(err error) string {
    if err == nil {
        return ""
    }
    
    errStr := err.Error()
    
    // Remove sensitive patterns
    patterns := []struct {
        regex       string
        replacement string
    }{
        {`\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b`, "[BTC_ADDRESS]"},     // Bitcoin addresses
        {`\b0x[a-fA-F0-9]{40}\b`, "[ETH_ADDRESS]"},                   // Ethereum addresses
        {`\b[0-9a-fA-F]{64}\b`, "[HASH]"},                            // Transaction hashes
        {`"amount":\s*"[^"]*"`, `"amount":"[REDACTED]"`},             // Amount values
        {`private.*key`, "[PRIVATE_KEY]"},                            // Private key mentions
        {`secret`, "[SECRET]"},                                       // Secret mentions
    }
    
    for _, pattern := range patterns {
        re := regexp.MustCompile(pattern.regex)
        errStr = re.ReplaceAllString(errStr, pattern.replacement)
    }
    
    return errStr
}
```

## Testing Requirements

### 1. Unit Tests

**File**: `internal/monitoring/http_metrics_test.go`
```go
func TestHTTPMetrics_Middleware_Success(t *testing.T) {
    // Test successful request metrics collection
}

func TestHTTPMetrics_Middleware_Error(t *testing.T) {
    // Test error request metrics collection
}

func TestHTTPMetrics_EndpointNormalization(t *testing.T) {
    // Test endpoint path normalization for cardinality control
}
```

**File**: `internal/monitoring/business_metrics_test.go`
```go
func TestBusinessMetrics_OracleInstrumentation(t *testing.T) {
    // Test oracle handler instrumentation
}

func TestBusinessMetrics_SwapInstrumentation(t *testing.T) {
    // Test swap handler instrumentation
}

func TestDataSanitizer_SensitiveDataProtection(t *testing.T) {
    // Test sensitive data sanitization
}
```

### 2. Integration Tests

**File**: `internal/monitoring/metrics_integration_test.go`

Test metrics collection with real HTTP requests and business operations.

### 3. Performance Tests

- Validate middleware overhead < 1ms per request
- Test memory usage under high cardinality scenarios
- Benchmark metrics collection performance

## Performance Requirements

- HTTP middleware overhead: < 1ms per request
- Business logic instrumentation overhead: < 0.5ms per operation
- Memory usage for all metrics: < 50MB under normal load
- Metrics cardinality: < 1000 series per metric family

## Documentation Requirements

### 1. Metrics Documentation

Create comprehensive metrics documentation including:
- Metric names and descriptions
- Label meanings and expected values
- Query examples for common use cases
- Alert threshold recommendations

### 2. Dashboard Recommendations

Provide Grafana dashboard JSON configurations for:
- HTTP request monitoring
- Business operations dashboard
- Oracle data freshness
- Swap operation analytics

## Acceptance Criteria

- [ ] HTTP metrics middleware implemented with < 1ms overhead
- [ ] All handler methods instrumented with business metrics
- [ ] Sensitive data properly sanitized in all metrics and logs
- [ ] Prometheus metrics endpoint accessible at `/metrics`
- [ ] Cardinality limits enforced (< 1000 series per metric)
- [ ] Comprehensive error handling and logging
- [ ] Data sanitization prevents PII exposure
- [ ] Performance requirements met under load
- [ ] Unit test coverage > 90%
- [ ] Integration tests validate metrics accuracy
- [ ] Memory usage remains bounded under high load