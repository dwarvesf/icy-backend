package telemetry

import (
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Telemetry struct {
	appConfig *config.AppConfig
	logger    *logger.Logger
}

func New(appConfig *config.AppConfig, logger *logger.Logger) *Telemetry {
	return &Telemetry{
		appConfig: appConfig,
		logger:    logger,
	}
}
