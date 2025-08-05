# ADR-001: Health Check Architecture

**Date**: 2025-08-05  
**Status**: Proposed  
**Deciders**: Project Team  
**Context**: Phase 1 Monitoring Implementation  

## Context

The ICY Backend cryptocurrency swap system requires comprehensive health monitoring to ensure high availability and early detection of issues. We need to implement health check endpoints that validate system availability, database connectivity, and external API dependencies while maintaining minimal performance overhead.

## Decision

### 1. Health Check Library Selection

**Decision**: Use native Go implementation instead of third-party libraries like `tavsec/gin-healthcheck`.

**Rationale**:
- **Simplicity**: Current requirements are straightforward and don't require complex health check frameworks
- **Control**: Full control over response formats and check logic
- **Performance**: Zero dependency overhead, minimal memory footprint 
- **Flexibility**: Easy to customize for cryptocurrency-specific requirements
- **Maintenance**: No external dependency management or version conflicts

### 2. Health Check Endpoints Architecture

**Endpoints Structure**:
```
GET /healthz                    - Basic liveness check (no dependencies)
GET /api/v1/health/db          - Database connectivity check
GET /api/v1/health/external    - External API dependencies check
```

**Response Format Standardization**:
```go
// Basic health response
type HealthResponse struct {
    Status  string `json:"status"`         // "healthy" | "unhealthy"
    Message string `json:"message"`        // Human-readable message
}

// Detailed health response  
type DetailedHealthResponse struct {
    Status    string                    `json:"status"`
    Timestamp time.Time                 `json:"timestamp"`
    Checks    map[string]HealthCheck   `json:"checks"`
    Duration  string                    `json:"duration_ms"`
}

type HealthCheck struct {
    Status   string `json:"status"`
    Latency  int64  `json:"latency_ms,omitempty"`
    Error    string `json:"error,omitempty"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}
```

### 3. Integration with Existing Architecture

**Handler Integration**:
- Create new `internal/handler/health/` package
- Follow existing handler pattern with interface-based design
- Integrate with existing `handler.Handler` struct
- Use existing logger and configuration infrastructure

**Route Integration**:
- Basic `/healthz` route at root level (bypassing API key middleware)
- Detailed health routes under `/api/v1/health/*` (no API key required for monitoring)
- Modify existing middleware to exempt health endpoints from authentication

### 4. Database Health Check Strategy

**GORM Integration**:
```go
func (h *HealthHandler) checkDatabase(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Get underlying sql.DB from GORM
    sqlDB, err := h.db.DB()
    if err != nil {
        return HealthCheck{Status: "unhealthy", Error: err.Error()}
    }
    
    // Ping with context timeout
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    if err := sqlDB.PingContext(ctx); err != nil {
        return HealthCheck{Status: "unhealthy", Error: err.Error()}
    }
    
    return HealthCheck{
        Status:  "healthy", 
        Latency: time.Since(start).Milliseconds(),
    }
}
```

### 5. External API Health Check Strategy

**Circuit Breaker Integration**:
- Leverage existing circuit breaker implementation (if any) or implement basic timeout logic
- Check both Blockstream API and Base Chain RPC
- Use existing `btcrpc.IBtcRpc` and `baserpc.IBaseRPC` interfaces
- Implement parallel checks with appropriate timeouts

**Timeout Strategy**:
- Individual API check timeout: 3 seconds
- Overall external health check timeout: 10 seconds
- Use Go context cancellation for proper timeout handling

## Implementation Details

### 1. Health Handler Interface

```go
package health

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
```

### 2. Dependency Health Checking

```go
func (h *HealthHandler) checkBitcoinAPI(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Use a lightweight operation for health check
    _, err := h.btcRPC.EstimateFees()
    if err != nil {
        return HealthCheck{Status: "unhealthy", Error: err.Error()}
    }
    
    return HealthCheck{
        Status:  "healthy",
        Latency: time.Since(start).Milliseconds(),
    }
}

func (h *HealthHandler) checkBaseAPI(ctx context.Context) HealthCheck {
    start := time.Now()
    
    // Use ICY total supply as a lightweight check
    _, err := h.baseRPC.ICYTotalSupply()
    if err != nil {
        return HealthCheck{Status: "unhealthy", Error: err.Error()}
    }
    
    return HealthCheck{
        Status:  "healthy", 
        Latency: time.Since(start).Milliseconds(),
    }
}
```

## Consequences

### Positive
- **Low Overhead**: Native implementation with minimal performance impact
- **Full Control**: Complete control over health check logic and responses
- **Integration**: Seamless integration with existing Gin/GORM architecture
- **Monitoring**: Enables external monitoring systems to assess system health
- **Security**: Health endpoints accessible without compromising API security

### Negative
- **Custom Implementation**: Need to implement all health check logic from scratch
- **Testing Complexity**: More extensive testing required for custom implementation
- **Feature Scope**: Limited to current requirements, may need expansion later

### Risks and Mitigations
- **Performance Risk**: Health checks could impact application performance
  - *Mitigation*: Use lightweight operations and aggressive timeouts
- **Security Risk**: Health endpoints could expose system information
  - *Mitigation*: Limit information exposure, sanitize error messages
- **Reliability Risk**: Health checks themselves could fail
  - *Mitigation*: Implement graceful degradation and fallback responses

## Alternatives Considered

1. **tavsec/gin-healthcheck Library**
   - Pros: Quick implementation, established patterns
   - Cons: External dependency, less control, potential bloat

2. **Kubernetes Native Health Checks**
   - Pros: Industry standard endpoints
   - Cons: More complex, overkill for current requirements

3. **No Health Checks**
   - Pros: Zero implementation cost
   - Cons: No monitoring capability, higher operational risk

## References
- [Go Database Health Check Patterns](https://github.com/golang-standards/project-layout)
- [Gin Framework Middleware Best Practices](https://gin-gonic.com/docs/examples/custom-middleware/)
- [Health Check API RFC](https://tools.ietf.org/id/draft-inadarei-api-health-check-06.html)