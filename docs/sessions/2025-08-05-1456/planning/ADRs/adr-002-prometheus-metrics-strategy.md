# ADR-002: Prometheus Metrics Strategy

**Date**: 2025-08-05  
**Status**: Proposed  
**Deciders**: Project Team  
**Context**: Phase 1 Monitoring Implementation  

## Context

The ICY Backend requires comprehensive metrics collection for monitoring performance, business operations, and system health. We need to implement Prometheus metrics that provide valuable insights while maintaining low cardinality to prevent performance degradation and high storage costs.

## Decision

### 1. Prometheus Client Library Selection

**Decision**: Use `github.com/prometheus/client_golang` (official Prometheus Go client)

**Rationale**:
- **Official Support**: Maintained by Prometheus team, guaranteed compatibility
- **Performance**: Optimized for Go applications with minimal overhead
- **Feature Complete**: Supports all metric types and advanced features
- **Community**: Widely adopted with extensive documentation and examples
- **Integration**: Native integration with HTTP handlers and middleware

### 2. Metric Naming Convention

**Standard**: Follow Prometheus naming best practices with `icy_backend_` prefix

**Convention**:
```
icy_backend_<component>_<metric_name>_<unit>
```

**Examples**:
- `icy_backend_http_requests_total`
- `icy_backend_http_request_duration_seconds`
- `icy_backend_oracle_calculation_duration_seconds`
- `icy_backend_swap_operations_total`

### 3. Cardinality Management Strategy

**Critical Decision**: Maintain strict cardinality limits to prevent metric explosion

**Guidelines**:
- **Maximum Labels per Metric**: 4 labels
- **Maximum Label Values**: 20 values per label (except status codes)
- **Prohibited Labels**: User IDs, session IDs, timestamps, dynamic values
- **Approved Labels**: HTTP methods, endpoints (grouped), status codes, operation types

**Label Grouping Strategy**:
```go
// Group similar endpoints to reduce cardinality
func normalizeEndpoint(path string) string {
    switch {
    case strings.HasPrefix(path, "/api/v1/transactions"):
        return "/api/v1/transactions"
    case strings.HasPrefix(path, "/api/v1/oracle/"):
        return "/api/v1/oracle/*"
    case strings.HasPrefix(path, "/api/v1/swap/"):
        return "/api/v1/swap/*"
    case strings.HasPrefix(path, "/api/v1/health/"):
        return "/api/v1/health/*"
    default:
        return path
    }
}
```

### 4. HTTP Request Instrumentation

**Middleware Implementation**:
```go
type Metrics struct {
    requestsTotal   *prometheus.CounterVec
    requestDuration *prometheus.HistogramVec
    activeRequests  prometheus.Gauge
}

func NewMetrics() *Metrics {
    return &Metrics{
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
    }
}
```

**Custom Histogram Buckets**: Optimized for API response time characteristics:
- Fast responses: 5ms, 10ms, 25ms (health checks, simple queries)
- Normal responses: 50ms, 100ms, 250ms (business logic)
- Slow responses: 500ms, 1s, 2.5s (complex operations, external APIs)
- Timeout boundaries: 5s, 10s (timeouts and error conditions)

### 5. Business Logic Metrics

**Oracle Metrics**:
```go
var (
    oracleDataAge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "icy_backend_oracle_data_age_seconds",
            Help: "Age of oracle data in seconds",
        },
        []string{"data_type"}, // "btc_price", "icy_supply", "ratio"
    )
    
    oracleCalculationDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "icy_backend_oracle_calculation_duration_seconds",
            Help: "Oracle calculation duration in seconds", 
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2},
        },
        []string{"calculation_type"}, // "ratio", "treasury", "circulation"
    )
)
```

**Swap Operation Metrics**:
```go
var (
    swapOperationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "icy_backend_swap_operations_total",
            Help: "Total number of swap operations",
        },
        []string{"operation", "status"}, // operation: "create", "process", "complete"
    )
    
    swapProcessingDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "icy_backend_swap_processing_duration_seconds",
            Help: "Swap processing duration in seconds",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
        },
        []string{"operation"}, // "validation", "signature", "execution"
    )
)
```

### 6. External API Metrics

**API Call Instrumentation**:
```go
var (
    externalAPIDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "icy_backend_external_api_duration_seconds",
            Help: "External API call duration in seconds",
            Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
        },
        []string{"api_name", "operation"}, // api_name: "blockstream", "base_rpc"
    )
    
    externalAPICalls = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "icy_backend_external_api_calls_total", 
            Help: "Total number of external API calls",
        },
        []string{"api_name", "status"}, // status: "success", "error", "timeout"
    )
    
    circuitBreakerState = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "icy_backend_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"api_name"},
    )
)
```

### 7. Background Job Metrics

**Cron Job Instrumentation**:
```go
var (
    backgroundJobDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "icy_backend_background_job_duration_seconds",
            Help: "Background job execution duration in seconds",
            Buckets: []float64{1, 5, 10, 30, 60, 300, 600}, // 1s to 10min
        },
        []string{"job_name", "status"},
    )
    
    backgroundJobRuns = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "icy_backend_background_job_runs_total",
            Help: "Total number of background job runs",
        },
        []string{"job_name", "status"}, // status: "success", "error"
    )
    
    pendingTransactions = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "icy_backend_pending_transactions_total",
            Help: "Number of pending transactions",
        },
        []string{"transaction_type"}, // "btc", "icy", "swap"
    )
)
```

### 8. Metrics Registry and Lifecycle Management

**Custom Registry**:
```go
type MetricsRegistry struct {
    registry *prometheus.Registry
    metrics  *Metrics
}

func NewMetricsRegistry() *MetricsRegistry {
    registry := prometheus.NewRegistry()
    metrics := NewMetrics()
    
    // Register all metrics
    registry.MustRegister(
        metrics.requestsTotal,
        metrics.requestDuration,
        metrics.activeRequests,
        // ... other metrics
    )
    
    return &MetricsRegistry{
        registry: registry,
        metrics:  metrics,
    }
}
```

**Handler Integration**:
```go
func (mr *MetricsRegistry) Handler() http.Handler {
    return promhttp.HandlerFor(
        mr.registry,
        promhttp.HandlerOpts{
            Registry: mr.registry,
            Timeout:  5 * time.Second,
        },
    )
}
```

## Implementation Strategy

### 1. Phased Rollout
1. **Week 1**: HTTP request metrics middleware
2. **Week 2**: External API instrumentation  
3. **Week 3**: Background job metrics
4. **Week 4**: Business logic metrics and optimization

### 2. Performance Requirements
- **HTTP Middleware Overhead**: < 1ms per request
- **Memory Usage**: < 50MB for all metrics
- **Cardinality Monitoring**: Alert if any metric exceeds 1000 series

### 3. Integration Points
- **Gin Middleware**: HTTP request instrumentation
- **Interface Wrappers**: External API instrumentation  
- **Telemetry Service**: Background job instrumentation
- **Handler Methods**: Business logic instrumentation

## Consequences

### Positive
- **Low Cardinality**: Prevents metric explosion and performance issues
- **Comprehensive Coverage**: Metrics for all critical system components
- **Performance Optimized**: Custom buckets and efficient collection
- **Standardized**: Consistent naming and labeling conventions

### Negative  
- **Implementation Complexity**: Requires careful instrumentation throughout codebase
- **Maintenance Overhead**: Need to maintain metric definitions and cardinality
- **Storage Requirements**: Additional infrastructure for Prometheus server

### Risks and Mitigations
- **Cardinality Explosion**: Could cause memory issues and high storage costs
  - *Mitigation*: Strict labeling guidelines and monitoring dashboards
- **Performance Impact**: Metrics collection could slow down requests
  - *Mitigation*: Efficient middleware and selective instrumentation
- **Information Exposure**: Metrics could leak sensitive business data
  - *Mitigation*: Careful metric design and access control

## Alternatives Considered

1. **StatsD + Graphite**
   - Pros: Push-based, lower client overhead
   - Cons: Less powerful querying, additional infrastructure

2. **InfluxDB Time Series**
   - Pros: High cardinality support, powerful querying
   - Cons: More complex setup, different ecosystem

3. **Custom Metrics Solution**
   - Pros: Complete control, minimal dependencies
   - Cons: High development cost, no ecosystem benefits

## References
- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [Go Client Library Documentation](https://github.com/prometheus/client_golang)
- [Cardinality Management Guide](https://www.robustperception.io/cardinality-is-key)