package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/robfig/cron/v3"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store"
	pgstore "github.com/dwarvesf/icy-backend/internal/store/postgres"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	httpTransport "github.com/dwarvesf/icy-backend/internal/transport/http"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// settlementRunner is the narrow slice of telemetry the settlement cron drives.
type settlementRunner interface {
	ProcessPendingBtcTransactions() error
}

// newSettlementJob wraps the settlement run in a robfig/cron SkipIfStillRunning
// chain so two overlapping cron ticks cannot execute ProcessPendingBtcTransactions
// concurrently. A tick that fires while a prior run is still in flight is
// dropped. Extracted from Init so the no-overlap behavior is unit-testable.
func newSettlementJob(t settlementRunner, log *logger.Logger) cron.Job {
	return cron.NewChain(cron.SkipIfStillRunning(cron.DefaultLogger)).Then(cron.FuncJob(func() {
		if err := t.ProcessPendingBtcTransactions(); err != nil {
			log.Error("[settlementJob][ProcessPendingBtcTransactions]", map[string]string{
				"error": err.Error(),
			})
		}
	}))
}

func Init() {
	appConfig := config.New()
	logger := logger.New(appConfig.Environment)

	db := pgstore.New(appConfig, logger)

	// Initialize monitoring components
	externalAPIMetrics := monitoring.NewExternalAPIMetrics()
	backgroundJobMetrics := monitoring.NewBackgroundJobMetrics()
	jobStatusManager := monitoring.NewJobStatusManager(logger, backgroundJobMetrics)

	// Circuit breaker configuration
	circuitBreakerConfig := monitoring.CircuitBreakerConfig{
		MaxRequests:                 10,
		ConsecutiveFailureThreshold: 5,
		Timeout:                     30 * time.Second,
		Interval:                    60 * time.Second,
	}

	// Timeout configuration
	timeoutConfig := monitoring.TimeoutConfig{
		RequestTimeout:     10 * time.Second,
		HealthCheckTimeout: 3 * time.Second,
	}

	s := store.New(db)
	btcRpc := btcrpc.New(appConfig, logger)
	baseRpc, err := baserpc.New(appConfig, logger)
	if err != nil {
		logger.Error("[Init][baserpc.New] failed to init base rpc", map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Wrap external APIs with circuit breakers
	btcRpcWithCB := monitoring.NewCircuitBreakerBtcRPCWithTimeout(
		btcRpc,
		circuitBreakerConfig,
		timeoutConfig,
		externalAPIMetrics,
		logger,
	)
	baseRpcWithCB := monitoring.NewCircuitBreakerBaseRPCWithTimeout(
		baseRpc,
		circuitBreakerConfig,
		timeoutConfig,
		externalAPIMetrics,
		logger,
	)

	// Use circuit breaker wrapped versions for oracle
	oracle := oracle.New(db, s, appConfig, logger, btcRpcWithCB, baseRpcWithCB)

	// Initialize base telemetry
	baseTelemetryInstance := telemetry.New(
		db,
		s,
		appConfig,
		logger,
		btcRpcWithCB,
		baseRpcWithCB,
		oracle,
	)

	// Wrap telemetry with monitoring instrumentation
	instrumentedTelemetry := monitoring.NewInstrumentedTelemetry(
		baseTelemetryInstance,
		jobStatusManager,
		backgroundJobMetrics,
		logger,
		appConfig,
	)

	// The effective drain-prevention limits, logged once at boot. These are the
	// only controls bounding how much BTC can leave, and they were previously
	// unreadable from outside the process: an operator could not tell a
	// configured value from a placeholder default without a redeploy. 0 means
	// the cap is DISABLED, which is worth seeing in a log line.
	logger.Info("[Init] effective payout limits", map[string]string{
		"max_payout_satoshi":       fmt.Sprintf("%d", appConfig.Bitcoin.MaxPayoutSatoshi),
		"max_daily_payout_satoshi": fmt.Sprintf("%d", appConfig.Bitcoin.MaxDailyPayoutSatoshi),
		"min_btc_confirmations":    fmt.Sprintf("%d", appConfig.Bitcoin.MinBtcConfirmations),
		"min_swap_confirmations":   fmt.Sprintf("%d", appConfig.Blockchain.MinSwapConfirmations),
		"service_fee_rate":         fmt.Sprintf("%v", appConfig.Bitcoin.ServiceFeeRate),
		"min_satoshi_fee":          fmt.Sprintf("%d", appConfig.Bitcoin.MinSatshiFee),
	})

	c := cron.New()

	// Add cron jobs with instrumented telemetry
	indexInterval := "2m"
	if appConfig.IndexInterval != "" {
		indexInterval = appConfig.IndexInterval
	}

	// Indexers are fire-and-forget reads; keep them on their own tick.
	// The ICY-transfer ledger indexer (IndexIcyTransaction) was retired
	// 2026-07-21: its table had no consumers and the job had been failing
	// its year-long catch-up for months. Chain history can rebuild it.
	c.AddFunc("@every "+indexInterval, func() {
		go instrumentedTelemetry.IndexBtcTransaction()
		go instrumentedTelemetry.IndexIcySwapTransaction()
	})

	// Settlement is the money path. Wrap it with SkipIfStillRunning so a slow
	// settlement cycle can never overlap itself: a tick that fires while the
	// previous ProcessPendingBtcTransactions is still in flight is dropped, not
	// run concurrently. Combined with the per-row atomic claim in the store,
	// this closes the overlap double-send window.
	if _, err := c.AddJob("@every "+indexInterval, newSettlementJob(instrumentedTelemetry, logger)); err != nil {
		logger.Error("[Init][AddJob] failed to schedule settlement job", map[string]string{
			"error": err.Error(),
		})
		return
	}

	c.Start()

	// Create HTTP server with monitoring components
	httpServer := httpTransport.NewHttpServerWithMonitoring(
		appConfig,
		logger,
		oracle,
		baseRpcWithCB,
		btcRpcWithCB,
		db,
		jobStatusManager,
		externalAPIMetrics,
		backgroundJobMetrics,
	)

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port
	}

	// Setup graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: httpServer,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting server", map[string]string{
			"port": port,
		})
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", map[string]string{
				"error": err.Error(),
			})
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	logger.Info("Shutting down server gracefully...")

	// Setup shutdown timeout - allow 30 seconds for graceful shutdown
	// This gives webhook calls time to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown the HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", map[string]string{
			"error": err.Error(),
		})
	} else {
		logger.Info("Server shutdown complete")
	}

	// Stop the cron scheduler
	c.Stop()
}
