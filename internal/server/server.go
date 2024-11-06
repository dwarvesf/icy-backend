package server

import (
	"github.com/dwarvesf/icy-backend/internal/transport/http"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func Init() {
	appConfig := config.New()
	logger := logger.New(appConfig.Environment)

	httpServer := http.NewHttpServer(appConfig, logger)

	httpServer.Run()
}
