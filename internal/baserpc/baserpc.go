package baserpc

import (
	"github.com/dwarvesf/icy-backend/contracts/icy"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type icyService struct {
	address  common.Address
	instance *icy.Icy
}

type BaseRpc struct {
	appConfig  *config.AppConfig
	logger     *logger.Logger
	icyService icyService
}

func New(appConfig *config.AppConfig, logger *logger.Logger) (IBaseRpc, error) {
	client, err := ethclient.Dial(appConfig.Blockchain.BaseRPCEndpoint)
	if err != nil {
		return nil, err
	}

	icyAddress := common.HexToAddress(appConfig.Blockchain.ICYContractAddr)
	icy, err := icy.NewIcy(icyAddress, client)
	if err != nil {
		return nil, err
	}
	return &BaseRpc{
		icyService: icyService{
			address:  icyAddress,
			instance: icy,
		},
		appConfig: appConfig,
		logger:    logger,
	}, nil
}

func (b *BaseRpc) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
	balance, err := b.icyService.instance.BalanceOf(&bind.CallOpts{}, common.HexToAddress(address))
	if err != nil {
		return nil, err
	}
	return &model.Web3BigInt{
		Value:   balance.String(),
		Decimal: 18,
	}, nil
}

func (b *BaseRpc) ICYTotalSupply() (*model.Web3BigInt, error) {
	totalSupply, err := b.icyService.instance.TotalSupply(&bind.CallOpts{})
	if err != nil {
		return nil, err
	}
	return &model.Web3BigInt{
		Value:   totalSupply.String(),
		Decimal: 18,
	}, nil
}
