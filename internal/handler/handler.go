package handler

import (
	"github.com/dwarvesf/icy-backend/internal/handler/oracle"
	oracleService "github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Handler struct {
	OracleHandler oracle.IHandler
}

func New(appConfig *config.AppConfig, logger *logger.Logger, oracleSvc oracleService.IOracle) *Handler {
	return &Handler{
		OracleHandler: oracle.New(oracleSvc, logger, appConfig),
	}
}
