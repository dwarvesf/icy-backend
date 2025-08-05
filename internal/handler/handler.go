package handler

import (
	"gorm.io/gorm"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/handler/health"
	"github.com/dwarvesf/icy-backend/internal/handler/metrics"
	"github.com/dwarvesf/icy-backend/internal/handler/oracle"
	"github.com/dwarvesf/icy-backend/internal/handler/swap"
	"github.com/dwarvesf/icy-backend/internal/handler/transaction"
	"github.com/dwarvesf/icy-backend/internal/monitoring"
	oracleService "github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store/onchainbtcprocessedtransaction"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Handler struct {
	SwapHandler        swap.IHandler
	OracleHandler      oracle.IHandler
	TransactionHandler transaction.IHandler
	HealthHandler      health.IHealthHandler
	MetricsHandler     *metrics.MetricsHandler
}

func New(appConfig *config.AppConfig, logger *logger.Logger,
	oracleSvc oracleService.IOracle,
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	db *gorm.DB,
	metricsRegistry *prometheus.Registry) *Handler {
	return &Handler{
		OracleHandler:      oracle.New(oracleSvc, logger, appConfig),
		SwapHandler:        swap.New(logger, appConfig, oracleSvc, baseRPC, btcRPC, db),
		TransactionHandler: transaction.NewTransactionHandler(db, onchainbtcprocessedtransaction.New()),
		HealthHandler:      health.New(appConfig, logger, db, btcRPC, baseRPC, nil),
		MetricsHandler:     metrics.NewMetricsHandler(metricsRegistry),
	}
}

func NewWithMonitoring(appConfig *config.AppConfig, logger *logger.Logger,
	oracleSvc oracleService.IOracle,
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	db *gorm.DB,
	metricsRegistry *prometheus.Registry,
	jobStatusManager *monitoring.JobStatusManager) *Handler {
	return &Handler{
		OracleHandler:      oracle.New(oracleSvc, logger, appConfig),
		SwapHandler:        swap.New(logger, appConfig, oracleSvc, baseRPC, btcRPC, db),
		TransactionHandler: transaction.NewTransactionHandler(db, onchainbtcprocessedtransaction.New()),
		HealthHandler:      health.New(appConfig, logger, db, btcRPC, baseRPC, jobStatusManager),
		MetricsHandler:     metrics.NewMetricsHandler(metricsRegistry),
	}
}
