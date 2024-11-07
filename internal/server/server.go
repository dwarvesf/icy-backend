package server

import (
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
	_ = oracle.New(appConfig, logger)

	httpServer := http.NewHttpServer(appConfig, logger)

	httpServer.Run()
}
