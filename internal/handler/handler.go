package handler

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/handler/oracle"
	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	oracleService "github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Handler struct {
	OracleHandler oracle.IHandler
	SwapHandler   swap.IHandler
}

func New(appConfig *config.AppConfig, logger *logger.Logger, oracleSvc oracleService.IOracle, baseRPC baserpc.IBaseRPC, db *gorm.DB) *Handler {
	return &Handler{
		OracleHandler: oracle.New(oracleSvc, logger, appConfig),
		SwapHandler:   swap.New(logger, appConfig, oracleSvc, baseRPC, db),
	}
}
