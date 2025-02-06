package server

import (
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

	s := store.New()
	btcRpc := btcrpc.New(appConfig, logger)
	baseRpc, err := baserpc.New(appConfig, logger)
	if err != nil {
		logger.Error("Failed to init base rpc")
		return
	}

	oracle := oracle.New(db, s, appConfig, logger, btcRpc, baseRpc)
	telemetry := telemetry.New(db, s, appConfig, logger, btcRpc, baseRpc)

	// Initialize contract controller
	contractController := controller.New(
		baseRpc,
		btcRpc,
		oracle,
		telemetry,
		logger,
		appConfig,
	)

	c := cron.New()

	// Add cron jobs
	indexInterval := "2m"
	if appConfig.IndexInterval != "" {
		indexInterval = appConfig.IndexInterval
	}

	c.AddFunc("@every "+indexInterval, func() {
		telemetry.IndexBtcTransaction()
		telemetry.IndexIcyTransaction()
	})

	c.Start()

	httpServer := http.NewHttpServer(appConfig, logger, oracle, contractController)
	httpServer.Run()
}
