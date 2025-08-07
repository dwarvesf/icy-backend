package server

import (
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/robfig/cron/v3"
	"time"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store"
	pgstore "github.com/dwarvesf/icy-backend/internal/store/postgres"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/transport/http"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

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

	c := cron.New()

	// Add cron jobs with instrumented telemetry
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
	
	// Create HTTP server with monitoring components
	httpServer := http.NewHttpServerWithMonitoring(
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
	httpServer.Run()
}
