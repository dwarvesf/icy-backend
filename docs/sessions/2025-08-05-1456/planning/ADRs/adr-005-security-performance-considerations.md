# ADR-005: Security and Performance Considerations for Monitoring System

**Date**: 2025-08-05  
**Status**: Proposed  
**Deciders**: Project Team  
**Context**: Phase 1 Monitoring Implementation  

## Context

The ICY Backend is a cryptocurrency/financial system that requires strict security and performance standards. The monitoring system must provide comprehensive observability while maintaining security boundaries, protecting sensitive data, and ensuring minimal performance overhead. This ADR addresses critical security and performance considerations for the monitoring implementation.

## Decision

### 1. Security Framework for Monitoring

**Data Classification and Protection**:
```go
type SensitiveDataType string

const (
    // Prohibited in logs/metrics - NEVER expose
    PrivateKeysData    SensitiveDataType = "private_keys"
    UserSecretsData    SensitiveDataType = "user_secrets"
    APIKeysData        SensitiveDataType = "api_keys"
    
    // Restricted - Hash or truncate before exposure
    UserAddressesData  SensitiveDataType = "user_addresses"
    TransactionHashData SensitiveDataType = "transaction_hash"
    AmountData         SensitiveDataType = "amount_data"
    
    // Safe for exposure in monitoring
    SystemMetricsData  SensitiveDataType = "system_metrics"
    StatusData         SensitiveDataType = "status_data"
)

type DataSanitizer struct {
    logger *logger.Logger
}

func (ds *DataSanitizer) SanitizeAddress(address string) string {
    if len(address) <= 8 {
        return "***"
    }
    return address[:4] + "..." + address[len(address)-4:]
}

func (ds *DataSanitizer) SanitizeAmount(amount *model.Web3BigInt) string {
    // For monitoring, only expose ranges, not exact amounts
    val := amount.Value()
    switch {
    case val.Cmp(big.NewInt(1000000)) < 0: // < 0.01 BTC
        return "small"
    case val.Cmp(big.NewInt(100000000)) < 0: // < 1 BTC  
        return "medium"
    default:
        return "large"
    }
}

func (ds *DataSanitizer) SanitizeError(err error) string {
    errStr := err.Error()
    
    // Remove potential sensitive data from error messages
    sensitivePatterns := []string{
        `\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b`, // Bitcoin addresses
        `\b0x[a-fA-F0-9]{40}\b`,                // Ethereum addresses
        `\bprivate.*key\b`,                     // Private key mentions
        `\bsecret\b`,                           // Secret mentions
    }
    
    for _, pattern := range sensitivePatterns {
        re := regexp.MustCompile(pattern)
        errStr = re.ReplaceAllString(errStr, "[REDACTED]")
    }
    
    return errStr
}
```

### 2. Metrics Endpoint Security

**Access Control Strategy**:
```go
func metricsAuthMiddleware(appConfig *config.AppConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Allow internal network access without auth (for Prometheus)
        clientIP := c.ClientIP()
        if isInternalNetwork(clientIP) {
            c.Next()
            return
        }
        
        // Require API key for external access
        apiKey := c.GetHeader("Authorization")
        if apiKey == "" || !validateMetricsAPIKey(apiKey, appConfig) {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            c.Abort()
            return
        }
        
        c.Next()
    }
}

func isInternalNetwork(ip string) bool {
    // Define internal network ranges
    internalRanges := []string{
        "10.0.0.0/8",
        "172.16.0.0/12", 
        "192.168.0.0/16",
        "127.0.0.0/8", // localhost
    }
    
    clientIP := net.ParseIP(ip)
    if clientIP == nil {
        return false
    }
    
    for _, cidr := range internalRanges {
        _, network, err := net.ParseCIDR(cidr)
        if err != nil {
            continue
        }
        if network.Contains(clientIP) {
            return true
        }
    }
    
    return false
}

// Separate metrics endpoint configuration
func setupMetricsEndpoint(r *gin.Engine, metricsRegistry *MetricsRegistry, appConfig *config.AppConfig) {
    // Create separate route group for metrics
    metrics := r.Group("/metrics")
    metrics.Use(metricsAuthMiddleware(appConfig))
    metrics.Use(ratelimitMiddleware(10, time.Minute)) // Rate limit metrics access
    
    metrics.GET("", gin.WrapH(metricsRegistry.Handler()))
}
```

### 3. Performance Optimization Framework

**Monitoring Overhead Budgets**:
```go
type PerformanceBudget struct {
    MaxLatencyOverhead    time.Duration // Max additional latency per request
    MaxMemoryOverhead     int64         // Max additional memory usage (bytes)
    MaxCPUOverheadPercent float64       // Max additional CPU usage (%)
    MaxMetricCardinality  int           // Max unique metric series
}

var MonitoringBudgets = map[string]PerformanceBudget{
    "http_middleware": {
        MaxLatencyOverhead:    500 * time.Microsecond, // 0.5ms
        MaxMemoryOverhead:     1024 * 1024,            // 1MB
        MaxCPUOverheadPercent: 0.5,                    // 0.5%
        MaxMetricCardinality:  200,                    // Max 200 series
    },
    "background_jobs": {
        MaxLatencyOverhead:    10 * time.Millisecond, // 10ms
        MaxMemoryOverhead:     5 * 1024 * 1024,       // 5MB
        MaxCPUOverheadPercent: 1.0,                   // 1%
        MaxMetricCardinality:  50,                    // Max 50 series
    },
    "health_checks": {
        MaxLatencyOverhead:    50 * time.Millisecond, // 50ms
        MaxMemoryOverhead:     512 * 1024,            // 512KB
        MaxCPUOverheadPercent: 0.1,                   // 0.1%
        MaxMetricCardinality:  20,                    // Max 20 series
    },
}
```

### 4. Efficient Metrics Collection

**Optimized Middleware Implementation**:
```go
type OptimizedMetricsMiddleware struct {
    metrics        *Metrics
    sampleRate     float64 // Sample rate for expensive operations
    fastPathCache  map[string]string // Cache for path normalization
    cacheMutex     sync.RWMutex
}

func NewOptimizedMetricsMiddleware(metrics *Metrics) *OptimizedMetricsMiddleware {
    return &OptimizedMetricsMiddleware{
        metrics:       metrics,
        sampleRate:    1.0, // Start with 100% sampling
        fastPathCache: make(map[string]string),
    }
}

func (omm *OptimizedMetricsMiddleware) GinMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        // Fast path for metrics collection
        method := c.Request.Method
        path := omm.getCachedNormalizedPath(c.FullPath())
        
        // Increment active requests (very fast operation)
        omm.metrics.activeRequests.Inc()
        defer omm.metrics.activeRequests.Dec()
        
        // Process request
        c.Next()
        
        // Collect metrics (optimized)
        duration := time.Since(start)
        status := strconv.Itoa(c.Writer.Status())
        
        // Use sampling for very frequent operations
        if omm.shouldSample() {
            omm.metrics.requestsTotal.WithLabelValues(method, path, status).Inc()
            omm.metrics.requestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
        }
    }
}

func (omm *OptimizedMetricsMiddleware) getCachedNormalizedPath(path string) string {
    omm.cacheMutex.RLock()
    normalized, exists := omm.fastPathCache[path]
    omm.cacheMutex.RUnlock()
    
    if exists {
        return normalized
    }
    
    // Compute normalized path
    normalized = normalizeEndpoint(path)
    
    omm.cacheMutex.Lock()
    // Prevent cache from growing too large
    if len(omm.fastPathCache) < 100 {
        omm.fastPathCache[path] = normalized
    }
    omm.cacheMutex.Unlock()
    
    return normalized
}

func (omm *OptimizedMetricsMiddleware) shouldSample() bool {
    return rand.Float64() < omm.sampleRate
}
```

### 5. Memory Management and Cleanup

**Bounded Metrics Storage**:
```go
type BoundedMetricsRegistry struct {
    registry          *prometheus.Registry
    metricsSeries     map[string]int // Track series count per metric
    seriesMutex       sync.RWMutex
    maxSeriesPerMetric int
    cleanupInterval   time.Duration
}

func NewBoundedMetricsRegistry(maxSeriesPerMetric int) *BoundedMetricsRegistry {
    bmr := &BoundedMetricsRegistry{
        registry:           prometheus.NewRegistry(),
        metricsSeries:      make(map[string]int),
        maxSeriesPerMetric: maxSeriesPerMetric,
        cleanupInterval:    1 * time.Hour,
    }
    
    // Start periodic cleanup
    go bmr.startPeriodicCleanup()
    
    return bmr
}

func (bmr *BoundedMetricsRegistry) MustRegister(collectors ...prometheus.Collector) {
    // Add cardinality checking before registration
    for _, collector := range collectors {
        if bmr.wouldExceedCardinality(collector) {
            log.Printf("Warning: Metric registration would exceed cardinality limits")
            continue
        }
    }
    
    bmr.registry.MustRegister(collectors...)
}

func (bmr *BoundedMetricsRegistry) startPeriodicCleanup() {
    ticker := time.NewTicker(bmr.cleanupInterval)
    defer ticker.Stop()
    
    for range ticker.C {
        bmr.cleanupUnusedMetrics()
    }
}

func (bmr *BoundedMetricsRegistry) cleanupUnusedMetrics() {
    // Implementation would analyze metric usage and clean up unused series
    // This is a placeholder for the cleanup logic
    bmr.seriesMutex.Lock()
    defer bmr.seriesMutex.Unlock()
    
    // Reset counters that haven't been updated recently
    // This would require more sophisticated tracking in a real implementation
}
```

### 6. Cryptocurrency-Specific Security Measures

**Financial Data Protection**:
```go
type FinancialMetricsSanitizer struct {
    dataSanitizer *DataSanitizer
    logger        *logger.Logger
}

func (fms *FinancialMetricsSanitizer) RecordSwapMetrics(operation string, amount *model.Web3BigInt, address string, err error) {
    // Sanitize all PII before recording metrics
    sanitizedAmount := fms.dataSanitizer.SanitizeAmount(amount)
    sanitizedAddress := fms.dataSanitizer.SanitizeAddress(address)
    
    labels := prometheus.Labels{
        "operation":    operation,
        "amount_range": sanitizedAmount,
        "status":       "success",
    }
    
    if err != nil {
        labels["status"] = "error"
        labels["error_type"] = classifyFinancialError(err)
        
        // Log error with sanitized data
        fms.logger.Error("Swap operation failed", map[string]string{
            "operation":        operation,
            "amount_range":     sanitizedAmount,
            "address_prefix":   sanitizedAddress,
            "sanitized_error":  fms.dataSanitizer.SanitizeError(err),
        })
    }
    
    swapOperationsTotal.With(labels).Inc()
}

func classifyFinancialError(err error) string {
    errStr := strings.ToLower(err.Error())
    
    switch {
    case strings.Contains(errStr, "insufficient"):
        return "insufficient_funds"
    case strings.Contains(errStr, "timeout"):
        return "timeout"
    case strings.Contains(errStr, "network"):
        return "network_error"
    case strings.Contains(errStr, "validation"):
        return "validation_error"
    default:
        return "unknown"
    }
}
```

### 7. Audit Logging for Monitoring Access

**Monitoring Access Audit**:
```go
type MonitoringAuditLogger struct {
    logger *logger.Logger
}

func (mal *MonitoringAuditLogger) LogMetricsAccess(c *gin.Context) {
    mal.logger.Info("Metrics endpoint accessed", map[string]string{
        "client_ip":    c.ClientIP(),
        "user_agent":   c.GetHeader("User-Agent"),
        "timestamp":    time.Now().Format(time.RFC3339),
        "endpoint":     c.Request.URL.Path,
        "method":       c.Request.Method,
        "auth_method":  mal.getAuthMethod(c),
    })
}

func (mal *MonitoringAuditLogger) LogHealthCheckAccess(c *gin.Context, endpoint string, duration time.Duration) {
    mal.logger.Info("Health check accessed", map[string]string{
        "client_ip":  c.ClientIP(),
        "endpoint":   endpoint,
        "duration":   duration.String(),
        "timestamp":  time.Now().Format(time.RFC3339),
        "status":     strconv.Itoa(c.Writer.Status()),
    })
}

func (mal *MonitoringAuditLogger) getAuthMethod(c *gin.Context) string {
    if c.GetHeader("Authorization") != "" {
        return "api_key"
    }
    if isInternalNetwork(c.ClientIP()) {
        return "internal_network"
    }
    return "none"
}
```

### 8. Performance Monitoring for Monitoring System

**Self-Monitoring Implementation**:
```go
type MonitoringSystemMetrics struct {
    metricCollectionDuration *prometheus.HistogramVec
    memoryUsage             prometheus.Gauge
    cardinality             *prometheus.GaugeVec
    errorRate               *prometheus.CounterVec
}

func NewMonitoringSystemMetrics() *MonitoringSystemMetrics {
    return &MonitoringSystemMetrics{
        metricCollectionDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name: "icy_backend_monitoring_collection_duration_seconds",
                Help: "Time spent collecting monitoring metrics",
                Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
            },
            []string{"component"},
        ),
        memoryUsage: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_monitoring_memory_bytes",
                Help: "Memory used by monitoring system",
            },
        ),
        cardinality: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_monitoring_cardinality",
                Help: "Number of metric series per component",
            },
            []string{"metric_name"},
        ),
        errorRate: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_monitoring_errors_total",
                Help: "Errors in monitoring system itself",
            },
            []string{"component", "error_type"},
        ),
    }
}

func (msm *MonitoringSystemMetrics) StartSelfMonitoring(interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        
        for range ticker.C {
            msm.collectSelfMetrics()
        }
    }()
}

func (msm *MonitoringSystemMetrics) collectSelfMetrics() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    msm.memoryUsage.Set(float64(m.Alloc))
    
    // Additional self-monitoring logic would go here
}
```

## Implementation Guidelines

### 1. Security Checklist
- [ ] No private keys or secrets in logs/metrics
- [ ] All user data sanitized before exposure
- [ ] Metrics endpoint properly secured
- [ ] Internal network access configured
- [ ] Audit logging for monitoring access
- [ ] Rate limiting on monitoring endpoints
- [ ] Error message sanitization

### 2. Performance Checklist  
- [ ] Middleware overhead < 1ms per request
- [ ] Memory usage bounded and monitored
- [ ] Cardinality limits enforced
- [ ] Sampling implemented for high-volume metrics
- [ ] Path normalization cached
- [ ] Periodic cleanup of unused metrics
- [ ] Self-monitoring of monitoring system

### 3. Testing Requirements
- [ ] Security penetration testing
- [ ] Performance benchmarking under load
- [ ] Memory leak testing
- [ ] Cardinality explosion testing
- [ ] Data sanitization validation
- [ ] Access control testing

## Consequences

### Positive
- **Security**: Financial data protected with multiple layers of security
- **Performance**: Minimal overhead through optimization and budgeting
- **Auditability**: Complete audit trail for monitoring access
- **Maintainability**: Self-monitoring prevents monitoring system issues
- **Compliance**: Meets financial industry security standards

### Negative
- **Complexity**: Additional security and performance code complexity
- **Development Time**: More time required for secure implementation
- **Testing Overhead**: Extensive security and performance testing required

### Risks and Mitigations
- **Data Breach**: Monitoring could expose sensitive financial data
  - *Mitigation*: Multiple layers of data sanitization
- **Performance Degradation**: Monitoring could slow down critical operations
  - *Mitigation*: Performance budgets and optimization
- **Security Vulnerabilities**: Monitoring endpoints could be attack vectors
  - *Mitigation*: Proper access control and rate limiting

## Compliance Considerations

### Financial Industry Standards
- PCI DSS compliance for payment data protection
- SOX compliance for financial reporting accuracy
- GDPR compliance for user data protection
- Industry-specific cryptocurrency regulations

### Security Standards
- OWASP security guidelines
- CIS security benchmarks
- NIST cybersecurity framework
- ISO 27001 information security standards

## References
- [OWASP Application Security](https://owasp.org/www-project-application-security-verification-standard/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [Go Security Best Practices](https://golang.org/doc/security.html)
- [Prometheus Security Model](https://prometheus.io/docs/operating/security/)