# Task 5: Integration and Middleware Implementation Specification

**Date**: 2025-08-05  
**Task**: Integration and Middleware (System Integration)  
**Priority**: High  
**Estimated Effort**: 2-3 days  

## Overview

Integrate all monitoring components (Tasks 1-4) into a cohesive monitoring system with proper middleware, configuration management, and system-wide coordination. This ensures all monitoring components work together seamlessly while maintaining performance and security standards.

## Functional Requirements

### 1. Unified Metrics Registry

**Purpose**: Centralize all Prometheus metrics registration and management  
**Implementation**: Single registry with all metric families  
**Features**: Cardinality monitoring, cleanup, and health reporting  

### 2. Middleware Stack Integration

**Purpose**: Proper ordering and integration of all monitoring middleware  
**Implementation**: Gin middleware chain with optimized performance  
**Components**: HTTP metrics, security, rate limiting, logging  

### 3. Configuration Management

**Purpose**: Centralized configuration for all monitoring components  
**Implementation**: Environment-based configuration with validation  
**Features**: Hot reloading, validation, defaults  

### 4. System Startup Coordination

**Purpose**: Proper initialization order and dependency management  
**Implementation**: Structured startup with error handling  
**Features**: Graceful shutdown, health checks during startup  

## Technical Specification

### 1. Unified Monitoring System

```go
package monitoring

import (
    "context"
    "net/http"
    "sync"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"
    "gorm.io/gorm"
    
    "github.com/dwarvesf/icy-backend/internal/baserpc"
    "github.com/dwarvesf/icy-backend/internal/btcrpc"
    "github.com/dwarvesf/icy-backend/internal/utils/config"
    "github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type MonitoringSystem struct {
    // Core components
    metricsRegistry   *MetricsRegistry
    jobStatusManager  *JobStatusManager
    
    // Metrics
    httpMetrics       *HTTPMetrics
    businessMetrics   *BusinessMetrics
    externalMetrics   *ExternalAPIMetrics
    backgroundMetrics *BackgroundJobMetrics
    systemMetrics     *SystemMetrics
    
    // Configuration
    config           *MonitoringConfig
    logger           *logger.Logger
    
    // State
    mu               sync.RWMutex
    started          bool
    startTime        time.Time
}

type MonitoringConfig struct {
    // HTTP Metrics
    HTTPMetrics HTTPMetricsConfig `json:"http_metrics"`
    
    // Background Jobs
    BackgroundJobs BackgroundJobConfig `json:"background_jobs"`
    
    // External APIs
    ExternalAPIs ExternalAPIConfig `json:"external_apis"`
    
    // Security
    Security SecurityConfig `json:"security"`
    
    // Performance
    Performance PerformanceConfig `json:"performance"`
}

type HTTPMetricsConfig struct {
    Enabled           bool          `json:"enabled"`
    SampleRate        float64       `json:"sample_rate"`
    IncludeRequestSize bool         `json:"include_request_size"`
    IncludeResponseSize bool        `json:"include_response_size"`
    PathNormalization map[string]string `json:"path_normalization"`
}

type BackgroundJobConfig struct {
    Enabled              bool          `json:"enabled"`
    StalledThreshold     time.Duration `json:"stalled_threshold"`
    CleanupInterval      time.Duration `json:"cleanup_interval"`
    RetentionPeriod      time.Duration `json:"retention_period"`
    DefaultJobTimeout    time.Duration `json:"default_job_timeout"`
}

type ExternalAPIConfig struct {
    CircuitBreakers map[string]CircuitBreakerSettings `json:"circuit_breakers"`
    Timeouts        map[string]TimeoutSettings        `json:"timeouts"`
}

type SecurityConfig struct {
    MetricsAuth      MetricsAuthConfig   `json:"metrics_auth"`
    DataSanitization SanitizationConfig  `json:"data_sanitization"`
    RateLimiting     RateLimitingConfig  `json:"rate_limiting"`
}

type PerformanceConfig struct {
    MaxCardinality      int           `json:"max_cardinality"`
    MetricsTimeout      time.Duration `json:"metrics_timeout"`
    MemoryLimit         int64         `json:"memory_limit_bytes"`
    CPULimit            float64       `json:"cpu_limit_percent"`
}

func NewMonitoringSystem(appConfig *config.AppConfig, logger *logger.Logger) (*MonitoringSystem, error) {
    // Load monitoring configuration
    monitoringConfig, err := loadMonitoringConfig(appConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to load monitoring config: %w", err)
    }
    
    // Create metrics components
    metricsRegistry := prometheus.NewRegistry()
    
    httpMetrics := NewHTTPMetrics()
    businessMetrics := NewBusinessMetrics()
    externalMetrics := NewExternalAPIMetrics()
    backgroundMetrics := NewBackgroundJobMetrics()
    systemMetrics := NewSystemMetrics()
    
    // Register all metrics
    metricsRegistry.MustRegister(
        httpMetrics.collectors()...,
    )
    metricsRegistry.MustRegister(
        businessMetrics.collectors()...,
    )
    metricsRegistry.MustRegister(
        externalMetrics.collectors()...,
    )
    metricsRegistry.MustRegister(
        backgroundMetrics.collectors()...,
    )
    metricsRegistry.MustRegister(
        systemMetrics.collectors()...,
    )
    
    // Create unified metrics registry
    unifiedRegistry := &MetricsRegistry{
        registry:        metricsRegistry,
        httpMetrics:     httpMetrics,
        businessMetrics: businessMetrics,
        externalMetrics: externalMetrics,
        backgroundMetrics: backgroundMetrics,
        systemMetrics:   systemMetrics,
    }
    
    // Create job status manager
    jobStatusManager := NewJobStatusManager(logger, backgroundMetrics)
    
    system := &MonitoringSystem{
        metricsRegistry:   unifiedRegistry,
        jobStatusManager:  jobStatusManager,
        httpMetrics:       httpMetrics,
        businessMetrics:   businessMetrics,
        externalMetrics:   externalMetrics,
        backgroundMetrics: backgroundMetrics,
        systemMetrics:     systemMetrics,
        config:           monitoringConfig,
        logger:           logger,
    }
    
    return system, nil
}

func (ms *MonitoringSystem) Start(ctx context.Context) error {
    ms.mu.Lock()
    defer ms.mu.Unlock()
    
    if ms.started {
        return fmt.Errorf("monitoring system already started")
    }
    
    ms.startTime = time.Now()
    
    // Start system metrics collection
    if err := ms.systemMetrics.Start(ctx); err != nil {
        return fmt.Errorf("failed to start system metrics: %w", err)
    }
    
    // Start cardinality monitoring
    go ms.startCardinalityMonitoring(ctx)
    
    // Start performance monitoring
    go ms.startPerformanceMonitoring(ctx)
    
    ms.started = true
    
    ms.logger.Info("Monitoring system started", map[string]string{
        "start_time": ms.startTime.Format(time.RFC3339),
    })
    
    return nil
}

func (ms *MonitoringSystem) Stop(ctx context.Context) error {
    ms.mu.Lock()
    defer ms.mu.Unlock()
    
    if !ms.started {
        return nil
    }
    
    // Stop system metrics
    if err := ms.systemMetrics.Stop(ctx); err != nil {
        ms.logger.Error("Error stopping system metrics", map[string]string{
            "error": err.Error(),
        })
    }
    
    ms.started = false
    
    ms.logger.Info("Monitoring system stopped", map[string]string{
        "uptime": time.Since(ms.startTime).String(),
    })
    
    return nil
}

func (ms *MonitoringSystem) HTTPMiddleware() gin.HandlerFunc {
    return ms.httpMetrics.Middleware()
}

func (ms *MonitoringSystem) MetricsHandler() http.Handler {
    return ms.metricsRegistry.Handler()
}

func (ms *MonitoringSystem) GetJobStatusManager() *JobStatusManager {
    return ms.jobStatusManager
}

func (ms *MonitoringSystem) GetBusinessMetrics() *BusinessMetrics {
    return ms.businessMetrics
}

func (ms *MonitoringSystem) GetExternalMetrics() *ExternalAPIMetrics {
    return ms.externalMetrics
}

func (ms *MonitoringSystem) startCardinalityMonitoring(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            cardinality := ms.metricsRegistry.GetCardinality()
            ms.systemMetrics.cardinality.Set(float64(cardinality))
            
            if cardinality > ms.config.Performance.MaxCardinality {
                ms.logger.Error("High cardinality detected", map[string]string{
                    "current_cardinality": fmt.Sprintf("%d", cardinality),
                    "max_cardinality":     fmt.Sprintf("%d", ms.config.Performance.MaxCardinality),
                })
            }
            
        case <-ctx.Done():
            return
        }
    }
}

func (ms *MonitoringSystem) startPerformanceMonitoring(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // Monitor memory usage
            var m runtime.MemStats
            runtime.ReadMemStats(&m)
            ms.systemMetrics.memoryUsage.Set(float64(m.Alloc))
            
            if m.Alloc > uint64(ms.config.Performance.MemoryLimit) {
                ms.logger.Error("High memory usage detected", map[string]string{
                    "current_memory": fmt.Sprintf("%d", m.Alloc),
                    "memory_limit":   fmt.Sprintf("%d", ms.config.Performance.MemoryLimit),
                })
            }
            
        case <-ctx.Done():
            return
        }
    }
}
```

### 2. System Metrics

```go
package monitoring

type SystemMetrics struct {
    // System performance
    memoryUsage     prometheus.Gauge
    cpuUsage        prometheus.Gauge
    goroutineCount  prometheus.Gauge
    
    // Monitoring system health
    cardinality     prometheus.Gauge
    metricsErrors   *prometheus.CounterVec
    uptime          prometheus.Gauge
    
    // Component status
    componentStatus *prometheus.GaugeVec
}

func NewSystemMetrics() *SystemMetrics {
    return &SystemMetrics{
        memoryUsage: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_system_memory_bytes",
                Help: "System memory usage in bytes",
            },
        ),
        cpuUsage: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_system_cpu_percent",
                Help: "System CPU usage percentage",
            },
        ),
        goroutineCount: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_system_goroutines",
                Help: "Number of goroutines",
            },
        ),
        cardinality: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_metrics_cardinality",
                Help: "Total number of metric series",
            },
        ),
        metricsErrors: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "icy_backend_metrics_errors_total",
                Help: "Total metrics collection errors",
            },
            []string{"component", "error_type"},
        ),
        uptime: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Name: "icy_backend_uptime_seconds",
                Help: "Application uptime in seconds",
            },
        ),
        componentStatus: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "icy_backend_component_status",
                Help: "Component status (1=healthy, 0=unhealthy)",
            },
            []string{"component"},
        ),
    }
}

func (sm *SystemMetrics) Start(ctx context.Context) error {
    startTime := time.Now()
    
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                // Update uptime
                sm.uptime.Set(time.Since(startTime).Seconds())
                
                // Update goroutine count
                sm.goroutineCount.Set(float64(runtime.NumGoroutine()))
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return nil
}

func (sm *SystemMetrics) Stop(ctx context.Context) error {
    return nil
}

func (sm *SystemMetrics) collectors() []prometheus.Collector {
    return []prometheus.Collector{
        sm.memoryUsage,
        sm.cpuUsage,
        sm.goroutineCount,
        sm.cardinality,
        sm.metricsErrors,
        sm.uptime,
        sm.componentStatus,
    }
}
```

### 3. Enhanced Middleware Stack

```go
package http

import (
    "net"
    "time"
    
    "github.com/gin-contrib/cors"
    "github.com/gin-gonic/gin" 
    
    "github.com/dwarvesf/icy-backend/internal/monitoring"
    "github.com/dwarvesf/icy-backend/internal/utils/config"
    "github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func setupMiddleware(
    r *gin.Engine,
    appConfig *config.AppConfig,
    logger *logger.Logger,
    monitoringSystem *monitoring.MonitoringSystem,
) {
    // Base middleware (always first)
    r.Use(gin.Recovery())
    
    // Monitoring middleware (early for accurate metrics)
    if monitoringSystem != nil {
        r.Use(monitoringSystem.HTTPMiddleware())
    }
    
    // Logging middleware (with monitoring endpoints excluded)
    r.Use(gin.LoggerWithWriter(
        gin.DefaultWriter,
        "/healthz",
        "/metrics",
        "/api/v1/health/*",
    ))
    
    // CORS middleware
    setupCORS(r, appConfig)
    
    // Rate limiting middleware
    r.Use(rateLimitMiddleware(appConfig))
    
    // Security headers middleware
    r.Use(securityHeadersMiddleware())
    
    // API key middleware (applied selectively)
    r.Use(apiKeyMiddleware(appConfig))
}

func rateLimitMiddleware(appConfig *config.AppConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Skip rate limiting for health endpoints and metrics
        if isMonitoringEndpoint(c.Request.URL.Path) {
            c.Next()
            return
        }
        
        // Implement rate limiting logic based on client IP
        clientIP := c.ClientIP()
        
        // Different limits for different endpoint types
        var limit int
        switch {
        case strings.HasPrefix(c.Request.URL.Path, "/api/v1/oracle"):
            limit = 60 // 60 requests per minute for oracle
        case strings.HasPrefix(c.Request.URL.Path, "/api/v1/swap"):
            limit = 30 // 30 requests per minute for swap
        default:
            limit = 100 // Default limit
        }
        
        // Simplified rate limiting (production should use Redis)
        if !checkRateLimit(clientIP, limit) {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded",
            })
            c.Abort()
            return
        }
        
        c.Next()
    }
}

func securityHeadersMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Security headers
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        
        // Don't cache sensitive endpoints
        if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
            c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
            c.Header("Pragma", "no-cache")
        }
        
        c.Next()
    }
}

func setupMetricsEndpoint(
    r *gin.Engine,
    monitoringSystem *monitoring.MonitoringSystem,
    appConfig *config.AppConfig,
) {
    // Metrics endpoint with authentication
    metrics := r.Group("/metrics")
    metrics.Use(metricsAuthMiddleware(appConfig))
    metrics.Use(rateLimitMiddleware(appConfig)) // Specific rate limiting for metrics
    
    metrics.GET("", gin.WrapH(monitoringSystem.MetricsHandler()))
}

func metricsAuthMiddleware(appConfig *config.AppConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Allow internal network access without auth
        clientIP := c.ClientIP()
        if isInternalNetwork(clientIP) {
            c.Next()
            return
        }
        
        // Require authentication for external access
        apiKey := c.GetHeader("Authorization")
        if apiKey == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Metrics access requires authentication"})
            c.Abort()
            return
        }
        
        // Validate metrics API key (separate from main API key)
        if !validateMetricsAPIKey(apiKey, appConfig) {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid metrics API key"})
            c.Abort()
            return
        }
        
        c.Next()
    }
}

func isMonitoringEndpoint(path string) bool {
    monitoringPaths := []string{
        "/healthz",
        "/metrics",
        "/api/v1/health/",
    }
    
    for _, monitoringPath := range monitoringPaths {
        if strings.HasPrefix(path, monitoringPath) {
            return true
        }
    }
    
    return false
}

func isInternalNetwork(ip string) bool {
    clientIP := net.ParseIP(ip)
    if clientIP == nil {
        return false
    }
    
    internalRanges := []string{
        "10.0.0.0/8",
        "172.16.0.0/12",
        "192.168.0.0/16",
        "127.0.0.0/8",
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
```

### 4. Updated Server Initialization

```go
package server

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/dwarvesf/icy-backend/internal/monitoring"
)

func Init() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Setup signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    // Load configuration
    appConfig := config.New()
    logger := logger.New(appConfig.Environment)
    
    // Initialize database
    db := pgstore.New(appConfig, logger)
    s := store.New(db)
    
    // Initialize monitoring system
    monitoringSystem, err := monitoring.NewMonitoringSystem(appConfig, logger)
    if err != nil {
        logger.Error("Failed to initialize monitoring system", map[string]string{
            "error": err.Error(),
        })
        return
    }
    
    // Start monitoring system
    if err := monitoringSystem.Start(ctx); err != nil {
        logger.Error("Failed to start monitoring system", map[string]string{
            "error": err.Error(),
        })
        return
    }
    
    // Initialize RPC clients
    btcRpc := btcrpc.New(appConfig, logger)
    baseRpc, err := baserpc.New(appConfig, logger)
    if err != nil {
        logger.Error("Failed to initialize base RPC", map[string]string{
            "error": err.Error(),
        })
        return
    }
    
    // Wrap RPC clients with circuit breakers
    btcRpcWithCB := monitoring.NewCircuitBreakerBtcRPC(
        btcRpc,
        monitoring.DefaultCircuitBreakerConfigs["blockstream_api"],
        logger,
        monitoringSystem.GetExternalMetrics(),
    )
    
    baseRpcWithCB := monitoring.NewCircuitBreakerBaseRPC(
        baseRpc,
        monitoring.DefaultCircuitBreakerConfigs["base_rpc"],
        logger,
        monitoringSystem.GetExternalMetrics(),
    )
    
    // Initialize business logic
    oracle := oracle.New(db, s, appConfig, logger, btcRpcWithCB, baseRpcWithCB)
    
    // Initialize telemetry with monitoring
    baseTelemetry := telemetry.New(
        db,
        s,
        appConfig,
        logger,
        btcRpcWithCB,
        baseRpcWithCB,
        oracle,
    )
    
    instrumentedTelemetry := telemetry.NewInstrumentedTelemetry(
        baseTelemetry,
        monitoringSystem.GetJobStatusManager(),
        monitoringSystem.GetBackgroundJobMetrics(),
        logger,
    )
    
    // Setup cron jobs
    c := cron.New(cron.WithLogger(cron.VerbosePrintfLogger(logger)))
    
    indexInterval := "2m"
    if appConfig.IndexInterval != "" {
        indexInterval = appConfig.IndexInterval
    }
    
    c.AddFunc("@every "+indexInterval, func() {
        go instrumentedTelemetry.IndexBtcTransaction()
        go instrumentedTelemetry.IndexIcyTransaction() 
        go instrumentedTelemetry.IndexIcySwapTransaction()
        instrumentedTelemetry.ProcessSwapRequests()
        instrumentedTelemetry.ProcessPendingBtcTransactions()
    })
    
    c.Start()
    defer c.Stop()
    
    // Initialize HTTP server
    httpServer := http.NewHttpServer(
        appConfig,
        logger,
        oracle,
        baseRpcWithCB, 
        btcRpcWithCB,
        db,
        monitoringSystem,
    )
    
    // Start HTTP server in goroutine
    go func() {
        if err := httpServer.Run(); err != nil {
            logger.Error("HTTP server error", map[string]string{
                "error": err.Error(),
            })
            cancel()
        }
    }()
    
    // Wait for shutdown signal
    select {
    case sig := <-sigChan:
        logger.Info("Received shutdown signal", map[string]string{
            "signal": sig.String(),
        })
    case <-ctx.Done():
        logger.Info("Context cancelled, shutting down")
    }
    
    // Graceful shutdown
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()
    
    logger.Info("Starting graceful shutdown")
    
    // Stop monitoring system
    if err := monitoringSystem.Stop(shutdownCtx); err != nil {
        logger.Error("Error stopping monitoring system", map[string]string{
            "error": err.Error(),
        })
    }
    
    logger.Info("Graceful shutdown completed")
}
```

### 5. Configuration Loading

```go
package config

import (
    "encoding/json"
    "fmt"
    "os"
    "time"
    
    "github.com/dwarvesf/icy-backend/internal/monitoring"
)

func loadMonitoringConfig(appConfig *AppConfig) (*monitoring.MonitoringConfig, error) {
    // Default configuration
    config := &monitoring.MonitoringConfig{
        HTTPMetrics: monitoring.HTTPMetricsConfig{
            Enabled:             true,
            SampleRate:          1.0,
            IncludeRequestSize:  true,
            IncludeResponseSize: true,
            PathNormalization: map[string]string{
                "/api/v1/oracle/*":      "/api/v1/oracle/*",
                "/api/v1/swap/*":        "/api/v1/swap/*",
                "/api/v1/transactions":  "/api/v1/transactions",
                "/api/v1/health/*":      "/api/v1/health/*",
            },
        },
        BackgroundJobs: monitoring.BackgroundJobConfig{
            Enabled:           true,
            StalledThreshold:  5 * time.Minute,
            CleanupInterval:   1 * time.Hour,
            RetentionPeriod:   24 * time.Hour,
            DefaultJobTimeout: 10 * time.Minute,
        },
        ExternalAPIs: monitoring.ExternalAPIConfig{
            CircuitBreakers: map[string]monitoring.CircuitBreakerSettings{
                "blockstream_api": {
                    MaxRequests:                 5,
                    IntervalSeconds:             30,
                    TimeoutSeconds:              60,
                    ConsecutiveFailureThreshold: 3,
                },
                "base_rpc": {
                    MaxRequests:                 3,
                    IntervalSeconds:             45,
                    TimeoutSeconds:              120,
                    ConsecutiveFailureThreshold: 5,
                },
            },
            Timeouts: map[string]monitoring.TimeoutSettings{
                "blockstream_api": {
                    ConnectionTimeout:  2 * time.Second,
                    RequestTimeout:     5 * time.Second,
                    HealthCheckTimeout: 3 * time.Second,
                },
                "base_rpc": {
                    ConnectionTimeout:  3 * time.Second,
                    RequestTimeout:     10 * time.Second,
                    HealthCheckTimeout: 5 * time.Second,
                },
            },
        },
        Security: monitoring.SecurityConfig{
            MetricsAuth: monitoring.MetricsAuthConfig{
                RequireAuth:     appConfig.Environment == "prod",
                InternalNetworks: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
            },
            DataSanitization: monitoring.SanitizationConfig{
                SanitizeAddresses: true,
                SanitizeAmounts:   true,
                SanitizeErrors:    true,
            },
            RateLimiting: monitoring.RateLimitingConfig{
                Enabled:     true,
                DefaultRate: 100,
                MetricsRate: 10,
            },
        },
        Performance: monitoring.PerformanceConfig{
            MaxCardinality: 1000,
            MetricsTimeout: 5 * time.Second,
            MemoryLimit:    50 * 1024 * 1024, // 50MB
            CPULimit:       1.0,               // 1%
        },
    }
    
    // Override with environment variables
    if configPath := os.Getenv("MONITORING_CONFIG_PATH"); configPath != "" {
        if data, err := os.ReadFile(configPath); err == nil {
            if err := json.Unmarshal(data, config); err != nil {
                return nil, fmt.Errorf("failed to parse monitoring config: %w", err)
            }
        }
    }
    
    // Validate configuration
    if err := validateMonitoringConfig(config); err != nil {
        return nil, fmt.Errorf("invalid monitoring config: %w", err)
    }
    
    return config, nil
}

func validateMonitoringConfig(config *monitoring.MonitoringConfig) error {
    if config.HTTPMetrics.SampleRate < 0 || config.HTTPMetrics.SampleRate > 1 {
        return fmt.Errorf("invalid HTTP metrics sample rate: %f", config.HTTPMetrics.SampleRate)
    }
    
    if config.BackgroundJobs.StalledThreshold < time.Minute {
        return fmt.Errorf("stalled threshold too low: %v", config.BackgroundJobs.StalledThreshold)
    }
    
    if config.Performance.MaxCardinality < 100 {
        return fmt.Errorf("max cardinality too low: %d", config.Performance.MaxCardinality)
    }
    
    return nil
}
```

## Integration Requirements

### 1. Updated HTTP Server

```go
// In internal/transport/http/http.go
func NewHttpServer(
    appConfig *config.AppConfig,
    logger *logger.Logger,
    oracle oracle.IOracle,
    baseRPC baserpc.IBaseRPC,
    btcRPC btcrpc.IBtcRpc,
    db *gorm.DB,
    monitoringSystem *monitoring.MonitoringSystem,
) *gin.Engine {
    r := gin.New()
    
    // Setup middleware stack
    setupMiddleware(r, appConfig, logger, monitoringSystem)
    
    // Create handlers with monitoring integration
    h := handler.New(
        appConfig,
        logger,
        oracle,
        baseRPC,
        btcRPC,
        db,
        monitoringSystem,
    )
    
    // Setup metrics endpoint
    setupMetricsEndpoint(r, monitoringSystem, appConfig)
    
    // Setup Swagger
    r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
    
    // Load routes
    loadV1Routes(r, h)
    
    return r
}
```

### 2. Updated Handler Factory

```go
// In internal/handler/handler.go
func New(
    appConfig *config.AppConfig,
    logger *logger.Logger,
    oracle oracle.IOracle,
    baseRPC baserpc.IBaseRPC,
    btcRPC btcrpc.IBtcRpc,
    db *gorm.DB,
    monitoringSystem *monitoring.MonitoringSystem,
) *Handler {
    
    businessMetrics := monitoringSystem.GetBusinessMetrics()
    jobStatusManager := monitoringSystem.GetJobStatusManager()
    
    return &Handler{
        OracleHandler: oracle.NewInstrumentedOracleHandler(
            oracle.New(appConfig, logger, oracle),
            businessMetrics,
            logger,
        ),
        SwapHandler: swap.NewInstrumentedSwapHandler(
            swap.New(appConfig, logger, oracle, baseRPC, btcRPC, db),
            businessMetrics,
            logger,
        ),
        TransactionHandler: transaction.NewInstrumentedTransactionHandler(
            transaction.New(appConfig, logger, db),
            businessMetrics,
            logger,
        ),
        HealthHandler: health.New(
            appConfig,
            logger,
            db,
            btcRPC,
            baseRPC,
            jobStatusManager,
        ),
    }
}
```

## Testing Requirements

### 1. Integration Tests

**File**: `internal/monitoring/system_integration_test.go`
```go
func TestMonitoringSystem_FullIntegration(t *testing.T) {
    // Test complete monitoring system integration
}

func TestMiddlewareStack_Performance(t *testing.T) {
    // Test middleware performance impact
}

func TestConfiguration_Validation(t *testing.T) {
    // Test configuration loading and validation
}
```

### 2. End-to-End Tests

Test complete request flow with all monitoring components active.

## Performance Requirements

- Complete middleware stack overhead: < 2ms per request
- System startup time increase: < 5 seconds
- Memory usage for monitoring system: < 100MB
- Configuration loading time: < 1 second

## Documentation Requirements

### 1. Integration Guide

- Complete setup and configuration guide
- Troubleshooting common integration issues
- Performance tuning recommendations

### 2. Operations Guide

- Monitoring system health checks
- Configuration management
- Upgrade and maintenance procedures

## Acceptance Criteria

- [ ] All monitoring components integrated into unified system
- [ ] Proper middleware ordering with minimal performance impact
- [ ] Configuration management working with validation
- [ ] Graceful startup and shutdown with proper cleanup
- [ ] Metrics endpoint secured and properly authenticated
- [ ] System performance requirements met
- [ ] Comprehensive error handling and logging
- [ ] Integration tests validate complete system functionality
- [ ] Documentation complete for operations and development teams