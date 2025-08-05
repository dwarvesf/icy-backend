# Task 1: Health Endpoints Implementation Specification

**Date**: 2025-08-05  
**Task**: Basic Health Endpoints (Synthetic Monitoring)  
**Priority**: High  
**Estimated Effort**: 3-4 days  

## Overview

Implement three health check endpoints to enable external monitoring systems to assess ICY Backend system health, database connectivity, and external API dependencies. These endpoints provide synthetic monitoring capabilities for external monitoring tools.

## Functional Requirements

### 1. Basic Health Endpoint (`/healthz`)

**Purpose**: Lightweight system availability check  
**Method**: `GET /healthz`  
**Authentication**: None required  
**SLA**: < 200ms response time, 99.9% availability  

**Response Format**:
```json
{
  "message": "ok"
}
```

**Implementation**:
```go
func (h *HealthHandler) Basic(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "message": "ok",
    })
}
```

### 2. Database Health Endpoint (`/api/v1/health/db`)

**Purpose**: Database connectivity and performance validation  
**Method**: `GET /api/v1/health/db`  
**Authentication**: None required  
**SLA**: < 500ms response time, 99.5% availability  
**Timeout**: 5 seconds for database operations  

**Response Format**:
```json
{
  "status": "healthy",
  "timestamp": "2025-08-05T14:30:00Z",
  "latency_ms": 15,
  "checks": {
    "database": {
      "status": "healthy",
      "latency_ms": 15,
      "metadata": {
        "driver": "postgres",
        "connection_pool": {
          "open_connections": 5,
          "in_use": 2,
          "idle": 3
        }
      }
    }
  },
  "duration_ms": 16
}
```

**Error Response**:
```json
{
  "status": "unhealthy", 
  "timestamp": "2025-08-05T14:30:00Z",
  "checks": {
    "database": {
      "status": "unhealthy",
      "error": "connection timeout after 5s",
      "latency_ms": 5000
    }
  },
  "duration_ms": 5001
}
```

### 3. External Dependencies Health Endpoint (`/api/v1/health/external`)

**Purpose**: External API connectivity and performance validation  
**Method**: `GET /api/v1/health/external`  
**Authentication**: None required  
**SLA**: < 2000ms response time, 95% availability  
**Timeout**: 10 seconds total (3 seconds per API check)  

**Response Format**:
```json
{
  "status": "healthy",
  "timestamp": "2025-08-05T14:30:00Z",
  "checks": {
    "blockstream_api": {
      "status": "healthy",
      "latency_ms": 85,
      "metadata": {
        "endpoint": "blockstream.info",
        "circuit_breaker_state": "closed",
        "last_success": "2025-08-05T14:29:45Z"
      }
    },
    "base_rpc": {
      "status": "healthy", 
      "latency_ms": 120,
      "metadata": {
        "endpoint": "base-mainnet.g.alchemy.com",
        "circuit_breaker_state": "closed",
        "last_success": "2025-08-05T14:29:50Z"
      }
    }
  },
  "duration_ms": 205
}
```

## Technical Specification

### 1. Health Handler Interface

```go
package health

import (
    "context"
    "time"
    
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
    
    "github.com/dwarvesf/icy-backend/internal/baserpc"
    "github.com/dwarvesf/icy-backend/internal/btcrpc"
    "github.com/dwarvesf/icy-backend/internal/utils/config"
    "github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type IHealthHandler interface {
    Basic(c *gin.Context)
    Database(c *gin.Context)
    External(c *gin.Context)
}

type HealthHandler struct {
    config  *config.AppConfig
    logger  *logger.Logger
    db      *gorm.DB
    btcRPC  btcrpc.IBtcRpc
    baseRPC baserpc.IBaseRPC
}

func New(
    config *config.AppConfig,
    logger *logger.Logger,
    db *gorm.DB,
    btcRPC btcrpc.IBtcRpc,
    baseRPC baserpc.IBaseRPC,
) IHealthHandler {
    return &HealthHandler{
        config:  config,
        logger:  logger,
        db:      db,
        btcRPC:  btcRPC,
        baseRPC: baseRPC,
    }
}
```

### 2. Response Types

```go
type HealthResponse struct {
    Status    string                    `json:"status"`
    Timestamp time.Time                 `json:"timestamp"`
    Checks    map[string]HealthCheck   `json:"checks,omitempty"`
    Duration  int64                     `json:"duration_ms"`
    Message   string                    `json:"message,omitempty"`
}

type HealthCheck struct {
    Status   string                 `json:"status"`
    Latency  int64                  `json:"latency_ms,omitempty"`
    Error    string                 `json:"error,omitempty"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type HealthStatus string

const (
    HealthStatusHealthy   HealthStatus = "healthy"
    HealthStatusUnhealthy HealthStatus = "unhealthy"
)
```

### 3. Database Health Check Implementation

```go
func (h *HealthHandler) Database(c *gin.Context) {
    start := time.Now()
    
    response := HealthResponse{
        Timestamp: start,
        Checks:    make(map[string]HealthCheck),
    }
    
    // Check database connectivity
    dbCheck := h.checkDatabase(c.Request.Context())
    response.Checks["database"] = dbCheck
    
    // Determine overall status
    response.Status = string(HealthStatusHealthy)
    if dbCheck.Status == string(HealthStatusUnhealthy) {
        response.Status = string(HealthStatusUnhealthy)
    }
    
    response.Duration = time.Since(start).Milliseconds()
    
    statusCode := http.StatusOK
    if response.Status == string(HealthStatusUnhealthy) {
        statusCode = http.StatusServiceUnavailable
    }
    
    // Log health check
    h.logger.Info("Database health check completed", map[string]string{
        "status":   response.Status,
        "duration": fmt.Sprintf("%dms", response.Duration),
        "db_latency": fmt.Sprintf("%dms", dbCheck.Latency),
    })
    
    c.JSON(statusCode, response)
}

func (h *HealthHandler) checkDatabase(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Create timeout context
    dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    // Get underlying SQL DB from GORM
    sqlDB, err := h.db.DB()
    if err != nil {
        return HealthCheck{
            Status: string(HealthStatusUnhealthy),
            Error:  fmt.Sprintf("failed to get SQL DB: %v", err),
            Latency: time.Since(start).Milliseconds(),
        }
    }
    
    // Ping database with timeout
    if err := sqlDB.PingContext(dbCtx); err != nil {
        return HealthCheck{
            Status: string(HealthStatusUnhealthy),
            Error:  fmt.Sprintf("database ping failed: %v", err),
            Latency: time.Since(start).Milliseconds(),
        }
    }
    
    // Get connection pool stats
    stats := sqlDB.Stats()
    
    return HealthCheck{
        Status:  string(HealthStatusHealthy),
        Latency: time.Since(start).Milliseconds(),
        Metadata: map[string]interface{}{
            "driver": "postgres",
            "connection_pool": map[string]interface{}{
                "open_connections": stats.OpenConnections,
                "in_use":          stats.InUse,
                "idle":            stats.Idle,
                "max_open":        stats.MaxOpenConnections,
            },
        },
    }
}
```

### 4. External Dependencies Health Check Implementation

```go
func (h *HealthHandler) External(c *gin.Context) {
    start := time.Now()
    
    response := HealthResponse{
        Timestamp: start,
        Checks:    make(map[string]HealthCheck),
    }
    
    // Create timeout context for overall operation
    ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
    defer cancel()
    
    // Check external APIs in parallel
    checksChan := make(chan map[string]HealthCheck, 2)
    
    go func() {
        checks := make(map[string]HealthCheck)
        checks["blockstream_api"] = h.checkBitcoinAPI(ctx)
        checksChan <- checks
    }()
    
    go func() {
        checks := make(map[string]HealthCheck)
        checks["base_rpc"] = h.checkBaseAPI(ctx)
        checksChan <- checks
    }()
    
    // Collect results
    for i := 0; i < 2; i++ {
        select {
        case checks := <-checksChan:
            for name, check := range checks {
                response.Checks[name] = check
            }
        case <-ctx.Done():
            // Handle timeout
            if len(response.Checks) == 0 {
                response.Checks["timeout"] = HealthCheck{
                    Status: string(HealthStatusUnhealthy),
                    Error:  "external API health check timeout",
                }
            }
            break
        }
    }
    
    // Determine overall status
    response.Status = string(HealthStatusHealthy)
    for _, check := range response.Checks {
        if check.Status == string(HealthStatusUnhealthy) {
            response.Status = string(HealthStatusUnhealthy)
            break
        }
    }
    
    response.Duration = time.Since(start).Milliseconds()
    
    statusCode := http.StatusOK
    if response.Status == string(HealthStatusUnhealthy) {
        statusCode = http.StatusServiceUnavailable
    }
    
    // Log health check
    h.logExternalHealthCheck(response)
    
    c.JSON(statusCode, response)
}

func (h *HealthHandler) checkBitcoinAPI(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Create API-specific timeout
    apiCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()
    
    // Use a lightweight operation for health check
    done := make(chan error, 1)
    go func() {
        _, err := h.btcRPC.EstimateFees()
        done <- err
    }()
    
    select {
    case err := <-done:
        latency := time.Since(start).Milliseconds()
        
        if err != nil {
            return HealthCheck{
                Status:  string(HealthStatusUnhealthy),
                Error:   fmt.Sprintf("bitcoin API error: %v", err),
                Latency: latency,
                Metadata: map[string]interface{}{
                    "endpoint": "blockstream.info",
                },
            }
        }
        
        return HealthCheck{
            Status:  string(HealthStatusHealthy),
            Latency: latency,
            Metadata: map[string]interface{}{
                "endpoint":    "blockstream.info",
                "last_success": time.Now().Format(time.RFC3339),
            },
        }
        
    case <-apiCtx.Done():
        return HealthCheck{
            Status:  string(HealthStatusUnhealthy),
            Error:   "bitcoin API timeout after 3s",
            Latency: time.Since(start).Milliseconds(),
            Metadata: map[string]interface{}{
                "endpoint": "blockstream.info",
            },
        }
    }
}

func (h *HealthHandler) checkBaseAPI(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Create API-specific timeout
    apiCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()
    
    // Use a lightweight operation for health check
    done := make(chan error, 1)
    go func() {
        _, err := h.baseRPC.ICYTotalSupply()
        done <- err
    }()
    
    select {
    case err := <-done:
        latency := time.Since(start).Milliseconds()
        
        if err != nil {
            return HealthCheck{
                Status:  string(HealthStatusUnhealthy),
                Error:   fmt.Sprintf("base RPC error: %v", err),
                Latency: latency,
                Metadata: map[string]interface{}{
                    "endpoint": "base-mainnet.g.alchemy.com",
                },
            }
        }
        
        return HealthCheck{
            Status:  string(HealthStatusHealthy),
            Latency: latency,
            Metadata: map[string]interface{}{
                "endpoint":     "base-mainnet.g.alchemy.com",
                "last_success": time.Now().Format(time.RFC3339),
            },
        }
        
    case <-apiCtx.Done():
        return HealthCheck{
            Status:  string(HealthStatusUnhealthy),
            Error:   "base RPC timeout after 3s",
            Latency: time.Since(start).Milliseconds(),
            Metadata: map[string]interface{}{
                "endpoint": "base-mainnet.g.alchemy.com",
            },
        }
    }
}

func (h *HealthHandler) logExternalHealthCheck(response HealthResponse) {
    logFields := map[string]string{
        "overall_status": response.Status,
        "duration":       fmt.Sprintf("%dms", response.Duration),
    }
    
    for name, check := range response.Checks {
        logFields[name+"_status"] = check.Status
        logFields[name+"_latency"] = fmt.Sprintf("%dms", check.Latency)
        if check.Error != "" {
            logFields[name+"_error"] = check.Error
        }
    }
    
    if response.Status == string(HealthStatusHealthy) {
        h.logger.Info("External APIs health check completed", logFields)
    } else {
        h.logger.Error("External APIs health check failed", logFields)
    }
}
```

## Integration Requirements

### 1. Handler Registration

Integrate health handlers into existing handler structure:

```go
// In internal/handler/handler.go
type Handler struct {
    // ... existing fields
    HealthHandler health.IHealthHandler
}

func New(
    appConfig *config.AppConfig,
    logger *logger.Logger,
    oracle oracle.IOracle,
    baseRPC baserpc.IBaseRPC,
    btcRPC btcrpc.IBtcRpc,
    db *gorm.DB,
) *Handler {
    return &Handler{
        // ... existing initialization
        HealthHandler: health.New(appConfig, logger, db, btcRPC, baseRPC),
    }
}
```

### 2. Route Registration

Update route registration in `internal/transport/http/v1.go`:

```go
func loadV1Routes(r *gin.Engine, h *handler.Handler) {
    // ... existing routes
    
    // Health endpoints
    health := v1.Group("/health")
    {
        health.GET("/db", h.HealthHandler.Database)
        health.GET("/external", h.HealthHandler.External)  
    }
    
    // Basic health check (at root level)
    r.GET("/healthz", h.HealthHandler.Basic)
}
```

### 3. Middleware Configuration

Ensure health endpoints bypass API key authentication in `internal/transport/http/http.go`:

```go
func apiKeyMiddleware(appConfig *config.AppConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Skip API key check for health endpoints
        if c.Request.URL.Path == "/healthz" ||
           strings.HasPrefix(c.Request.URL.Path, "/api/v1/health/") ||
           // ... other existing exemptions
        {
            c.Next()
            return
        }
        
        // ... rest of middleware logic
    }
}
```

## Testing Requirements

### 1. Unit Tests

**File**: `internal/handler/health/health_test.go`

```go
func TestHealthHandler_Basic(t *testing.T) {
    // Test basic health endpoint
}

func TestHealthHandler_Database_Healthy(t *testing.T) {
    // Test database health check with healthy DB
}

func TestHealthHandler_Database_Unhealthy(t *testing.T) {
    // Test database health check with unhealthy DB
}

func TestHealthHandler_External_AllHealthy(t *testing.T) {
    // Test external APIs health check with all APIs healthy
}

func TestHealthHandler_External_PartiallyUnhealthy(t *testing.T) {
    // Test external APIs health check with some APIs unhealthy
}

func TestHealthHandler_External_Timeout(t *testing.T) {
    // Test external APIs health check with timeout
}
```

### 2. Integration Tests

**File**: `internal/handler/health/health_integration_test.go`

Test health endpoints with real database and mock external APIs.

### 3. Performance Tests

Validate SLA requirements:
- `/healthz` responds in < 200ms
- `/api/v1/health/db` responds in < 500ms  
- `/api/v1/health/external` responds in < 2000ms

## Documentation Requirements

### 1. API Documentation

Update Swagger documentation with health endpoint definitions.

### 2. Operational Documentation

Create runbook entries for:
- Health endpoint response interpretation
- Troubleshooting unhealthy status
- SLA monitoring and alerting

## Acceptance Criteria

- [ ] `/healthz` endpoint returns 200 OK with simple message
- [ ] `/api/v1/health/db` validates database connectivity with timeout
- [ ] `/api/v1/health/external` checks both external APIs with parallel execution
- [ ] All endpoints return proper HTTP status codes (200 for healthy, 503 for unhealthy)
- [ ] Response formats match specification exactly
- [ ] Endpoints bypass API key authentication
- [ ] All SLA requirements met under normal conditions
- [ ] Comprehensive error handling and logging
- [ ] Unit test coverage > 90%
- [ ] Integration tests pass
- [ ] Performance tests validate SLA requirements