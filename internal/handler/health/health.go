package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// HealthHandler implements IHealthHandler interface
type HealthHandler struct {
	config           *config.AppConfig
	logger           *logger.Logger
	db               *gorm.DB
	btcRPC           btcrpc.IBtcRpc
	baseRPC          baserpc.IBaseRPC
	oracle           oracle.IOracle
	jobStatusManager *monitoring.JobStatusManager
}

// New creates a new health handler instance
func New(config *config.AppConfig, logger *logger.Logger, db *gorm.DB, btcRPC btcrpc.IBtcRpc, baseRPC baserpc.IBaseRPC, oracleSvc oracle.IOracle, jobStatusManager *monitoring.JobStatusManager) IHealthHandler {
	return &HealthHandler{
		config:           config,
		logger:           logger,
		db:               db,
		btcRPC:           btcRPC,
		baseRPC:          baseRPC,
		oracle:           oracleSvc,
		jobStatusManager: jobStatusManager,
	}
}

// Basic handles the basic health check endpoint (/healthz)
// @Summary Basic health check
// @Description Returns basic system availability status
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} BasicHealthResponse
// @Router /healthz [get]
func (h *HealthHandler) Basic(c *gin.Context) {
	response := BasicHealthResponse{
		Message: "ok",
	}
	c.JSON(http.StatusOK, response)
}

// Database handles the database health check endpoint
// @Summary Database health check
// @Description Validates database connectivity and performance
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse
// @Failure 503 {object} HealthResponse
// @Router /api/v1/health/db [get]
func (h *HealthHandler) Database(c *gin.Context) {
	start := time.Now()
	
	response := HealthResponse{
		Timestamp: start,
		Checks:    make(map[string]HealthCheck),
	}

	// Get context safely
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	// Check database health
	dbCheck := h.checkDatabase(ctx)
	response.Checks["database"] = dbCheck
	response.DurationMs = time.Since(start).Milliseconds()

	// Determine overall status
	if dbCheck.Status == "healthy" {
		response.Status = "healthy"
		c.JSON(http.StatusOK, response)
	} else {
		response.Status = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, response)
	}
}

// External handles the external API dependencies health check endpoint
// @Summary External dependencies health check
// @Description Validates external API connectivity and performance
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse
// @Failure 503 {object} HealthResponse
// @Router /api/v1/health/external [get]
func (h *HealthHandler) External(c *gin.Context) {
	start := time.Now()
	
	response := HealthResponse{
		Timestamp: start,
		Checks:    make(map[string]HealthCheck),
	}

	// Get context safely and create overall context with timeout
	baseCtx := context.Background()
	if c.Request != nil {
		baseCtx = c.Request.Context()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
	defer cancel()

	// Check external APIs in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Check Bitcoin API
	wg.Add(1)
	go func() {
		defer wg.Done()
		btcCheck := h.checkBitcoinAPI(ctx)
		mu.Lock()
		response.Checks["blockstream_api"] = btcCheck
		mu.Unlock()
	}()

	// Check Base RPC
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseCheck := h.checkBaseAPI(ctx)
		mu.Lock()
		response.Checks["base_rpc"] = baseCheck
		mu.Unlock()
	}()

	// Check the capability that actually matters: can we quote a rate?
	wg.Add(1)
	go func() {
		defer wg.Done()
		rateCheck := h.checkSwapRate()
		mu.Lock()
		response.Checks["swap_rate"] = rateCheck
		mu.Unlock()
	}()

	wg.Wait()
	response.DurationMs = time.Since(start).Milliseconds()

	// Determine overall status
	allHealthy := true
	for _, check := range response.Checks {
		if check.Status != "healthy" {
			allHealthy = false
			break
		}
	}

	if allHealthy {
		response.Status = "healthy"
		c.JSON(http.StatusOK, response)
	} else {
		response.Status = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, response)
	}
}

// checkDatabase performs database health validation
func (h *HealthHandler) checkDatabase(ctx context.Context) HealthCheck {
	start := time.Now()
	
	check := HealthCheck{
		Metadata: make(map[string]interface{}),
	}

	// Handle nil database
	if h.db == nil {
		check.Status = "unhealthy"
		check.Error = "database connection not available"
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	// Get underlying SQL DB
	sqlDB, err := h.db.DB()
	if err != nil {
		check.Status = "unhealthy"
		check.Error = fmt.Sprintf("failed to get underlying database: %v", err)
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	// Create context with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Ping database
	if err := sqlDB.PingContext(pingCtx); err != nil {
		check.Status = "unhealthy"
		if pingCtx.Err() == context.DeadlineExceeded {
			check.Error = "timeout"
		} else {
			check.Error = err.Error()
		}
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	// Get connection pool stats
	stats := sqlDB.Stats()
	
	check.Status = "healthy"
	check.Latency = time.Since(start).Milliseconds()
	check.Metadata["driver"] = "postgres"
	check.Metadata["connection_pool"] = map[string]interface{}{
		"open_connections": stats.OpenConnections,
		"in_use":          stats.InUse,
		"idle":            stats.Idle,
		"max_open":        stats.MaxOpenConnections,
	}

	return check
}

// checkBitcoinAPI performs Bitcoin API health validation
func (h *HealthHandler) checkBitcoinAPI(ctx context.Context) HealthCheck {
	start := time.Now()
	
	check := HealthCheck{
		Metadata: make(map[string]interface{}),
	}

	// Handle nil Bitcoin RPC
	if h.btcRPC == nil {
		check.Status = "unhealthy"
		check.Error = "bitcoin rpc not available"
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	// Create context with timeout for individual check
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Use a lightweight operation for health check
	done := make(chan error, 1)
	go func() {
		_, err := h.btcRPC.EstimateFees()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			check.Status = "unhealthy"
			check.Error = err.Error()
		} else {
			check.Status = "healthy"
			check.Metadata["endpoint"] = "blockstream.info"
		}
	case <-checkCtx.Done():
		check.Status = "unhealthy"
		if checkCtx.Err() == context.DeadlineExceeded {
			check.Error = "timeout"
		} else {
			check.Error = checkCtx.Err().Error()
		}
	}

	check.Latency = time.Since(start).Milliseconds()
	return check
}

// checkBaseAPI performs Base RPC health validation
func (h *HealthHandler) checkBaseAPI(ctx context.Context) HealthCheck {
	start := time.Now()
	
	check := HealthCheck{
		Metadata: make(map[string]interface{}),
	}

	// Handle nil Base RPC
	if h.baseRPC == nil {
		check.Status = "unhealthy"
		check.Error = "base rpc not available"
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	// Create context with timeout for individual check
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Use ICY total supply as a lightweight check
	done := make(chan error, 1)
	go func() {
		_, err := h.baseRPC.ICYTotalSupply()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			check.Status = "unhealthy"
			check.Error = err.Error()
		} else {
			check.Status = "healthy"
			check.Metadata["endpoint"] = "base_rpc"
		}
	case <-checkCtx.Done():
		check.Status = "unhealthy"
		if checkCtx.Err() == context.DeadlineExceeded {
			check.Error = "timeout"
		} else {
			check.Error = checkCtx.Err().Error()
		}
	}

	check.Latency = time.Since(start).Milliseconds()
	return check
}
// checkSwapRate reports whether the service can actually quote a swap rate.
//
// The existing base_rpc check calls ICYTotalSupply once and is explicitly a
// "lightweight check". GetCirculatedICY needs ELEVEN calls (one per locked
// treasury plus total supply), so under RPC rate limiting the light probe
// succeeds while the real work fails. That is exactly what production did on
// 2026-07-20: /health/external reported healthy for the whole time
// /swap/info was returning partial_data with no rate and swapping was paused.
//
// A health check that passes while the product is down is worse than no check,
// because it actively suppresses the alarm.
//
// This uses the CACHED accessor deliberately: on a warm cache it costs
// nothing, and when the circuit breaker is open it fails immediately without
// issuing any RPC call, so making the check honest adds no load to the
// dependency that is already struggling.
func (h *HealthHandler) checkSwapRate() HealthCheck {
	start := time.Now()
	check := HealthCheck{Metadata: make(map[string]interface{})}

	if h.oracle == nil {
		// Not wired (some constructions pass nil). Report unknown rather than
		// claiming health we cannot observe.
		check.Status = "unknown"
		check.Error = "oracle not available to health handler"
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	if _, err := h.oracle.GetCachedCirculatedICY(); err != nil {
		check.Status = "unhealthy"
		check.Error = err.Error()
		check.Metadata["impact"] = "cannot quote a swap rate, swapping is paused"
		check.Latency = time.Since(start).Milliseconds()
		return check
	}

	check.Status = "healthy"
	check.Latency = time.Since(start).Milliseconds()
	return check
}
