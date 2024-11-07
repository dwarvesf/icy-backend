package server

import (
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/transport/http"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func Init() {
	appConfig := config.New()
	logger := logger.New(appConfig.Environment)

	_ = store.NewPostgresStore(appConfig, logger)
	httpServer := http.NewHttpServer(appConfig, logger)

	httpServer.Run()
}
