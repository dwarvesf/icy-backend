package handler

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/handler/oracle"
	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	"github.com/dwarvesf/icy-backend/internal/handler/transaction"
	oracleService "github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Handler struct {
	SwapHandler        swap.IHandler
	OracleHandler      oracle.IHandler
	TransactionHandler transaction.IHandler
}

func New(appConfig *config.AppConfig, logger *logger.Logger, oracleSvc oracleService.IOracle, baseRPC baserpc.IBaseRPC, db *gorm.DB) *Handler {
	return &Handler{
		OracleHandler:      oracle.New(oracleSvc, logger, appConfig),
		SwapHandler:        swap.New(logger, appConfig, oracleSvc, baseRPC, db),
		TransactionHandler: transaction.NewTransactionHandler(db, onchainbtcprocessedtransaction.New()),
	}
}
