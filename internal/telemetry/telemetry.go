package telemetry

import (
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Telemetry struct {
	db        *gorm.DB
	store     *store.Store
	appConfig *config.AppConfig
	logger    *logger.Logger
	btcRpc    btcrpc.IBtcRpc
	baseRpc   baserpc.IBaseRPC
	oracle    oracle.IOracle
}

func New(
	db *gorm.DB,
	store *store.Store,
	appConfig *config.AppConfig,
	logger *logger.Logger,
	btcRpc btcrpc.IBtcRpc,
	baseRpc baserpc.IBaseRPC,
	oracle oracle.IOracle,
) *Telemetry {
	return &Telemetry{
		db:        db,
		store:     store,
		appConfig: appConfig,
		logger:    logger,
		btcRpc:    btcRpc,
		baseRpc:   baseRpc,
		oracle:    oracle,
	}
}
