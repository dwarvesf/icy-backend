package server

import (
	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store"
	pgstore "github.com/dwarvesf/icy-backend/internal/store/postgres"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/transport/http"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/robfig/cron/v3"
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

	_ = telemetry.New(appConfig, logger)

	c := cron.New()

	c.AddFunc("@every 2m", func() {
		//TODO:
		// telemetry.UpdateData()
	})

	httpServer := http.NewHttpServer(appConfig, logger, oracle)

	httpServer.Run()
}
