package handler

import (
	"github.com/dwarvesf/icy-backend/internal/controller"
	"github.com/dwarvesf/icy-backend/internal/handler/oracle"
	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	oracleService "github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"gorm.io/gorm"
)

type Handler struct {
	OracleHandler oracle.IHandler
	SwapHandler   swap.IHandler
}

func New(appConfig *config.AppConfig, logger *logger.Logger, oracleSvc oracleService.IOracle, controller controller.IController, db *gorm.DB) *Handler {
	return &Handler{
		OracleHandler: oracle.New(oracleSvc, logger, appConfig),
		SwapHandler:   swap.New(controller, logger, appConfig, oracleSvc, db),
	}
}
