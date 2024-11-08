package btcrpc

import (
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type BtcRpc struct {
	appConfig *config.AppConfig
	logger    *logger.Logger
}

func New(appConfig *config.AppConfig, logger *logger.Logger) IBtcRpc {
	return &BtcRpc{
		appConfig: appConfig,
		logger:    logger,
	}
}

func (b *BtcRpc) Send(receiverAddress string, amount *model.Web3BigInt) error {
	return nil
}

func (b *BtcRpc) BalanceOf(address string) (*model.Web3BigInt, error) {
	return nil, nil
}
