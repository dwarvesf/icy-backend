package server

import (
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	pgstore "github.com/dwarvesf/icy-backend/internal/store/postgres"
	"github.com/dwarvesf/icy-backend/internal/transport/http"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func Init() {
	appConfig := config.New()
	logger := logger.New(appConfig.Environment)

	_ = pgstore.New(appConfig, logger)
	btcRpc := btcrpc.New(appConfig, logger)
	oracle := oracle.New(appConfig, logger, btcRpc)

	httpServer := http.NewHttpServer(appConfig, logger, oracle)

	httpServer.Run()
}
