package server

import (
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/robfig/cron/v3"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/controller"
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

	s := store.New(db)
	btcRpc := btcrpc.New(appConfig, logger)
	baseRpc, err := baserpc.New(appConfig, logger)
	if err != nil {
		logger.Error("[Init][baserpc.New] failed to init base rpc", map[string]string{
			"error": err.Error(),
		})
		return
	}

	oracle := oracle.New(db, s, appConfig, logger, btcRpc, baseRpc)

	// Initialize telemetry first
	telemetryInstance := telemetry.New(
		db,
		s,
		appConfig,
		logger,
		btcRpc,
		baseRpc,
		oracle,
	)

	// Initialize contract controller
	contractController := controller.New(
		baseRpc,
		btcRpc,
		oracle,
		telemetryInstance,
		logger,
		appConfig,
		s,
		db,
	)

	c := cron.New()

	// Add cron jobs
	indexInterval := "2m"
	if appConfig.IndexInterval != "" {
		indexInterval = appConfig.IndexInterval
	}

	c.AddFunc("@every "+indexInterval, func() {
		go telemetry.IndexBtcTransaction()
		go telemetry.IndexIcyTransaction()
		go telemetry.ProcessPendingBtcTransactions()
	})

	c.Start()

	httpServer := http.NewHttpServer(appConfig, logger, oracle, contractController, db)
	httpServer.Run()
}
