package main

import (
	"github.com/dwarvesf/icy-backend/internal/server"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

func main() {
	appConfig := config.New()
	logger := logger.New(appConfig.Environment)
	_ = store.NewPostgresStore(appConfig, logger)

	server.Init()
}
