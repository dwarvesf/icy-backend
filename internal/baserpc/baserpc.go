package baserpc

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/dwarvesf/icy-backend/contracts/erc20"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type erc20Service struct {
	address  common.Address
	instance *erc20.Erc20
}

type BaseRPC struct {
	appConfig    *config.AppConfig
	logger       *logger.Logger
	erc20Service erc20Service
}

func New(appConfig *config.AppConfig, logger *logger.Logger) (IBaseRPC, error) {
	client, err := ethclient.Dial(appConfig.Blockchain.BaseRPCEndpoint)
	if err != nil {
		return nil, err
	}

	icyAddress := common.HexToAddress(appConfig.Blockchain.ICYContractAddr)
	icy, err := erc20.NewErc20(icyAddress, client)
	if err != nil {
		return nil, err
	}
	return &BaseRPC{
		erc20Service: erc20Service{
			address:  icyAddress,
			instance: icy,
		},
		appConfig: appConfig,
		logger:    logger,
	}, nil
}

func (b *BaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
	balance, err := b.erc20Service.instance.BalanceOf(&bind.CallOpts{}, common.HexToAddress(address))
	if err != nil {
		return nil, err
	}
	return &model.Web3BigInt{
		Value:   balance.String(),
		Decimal: 18,
	}, nil
}

func (b *BaseRPC) ICYTotalSupply() (*model.Web3BigInt, error) {
	totalSupply, err := b.erc20Service.instance.TotalSupply(&bind.CallOpts{})
	if err != nil {
		return nil, err
	}
	return &model.Web3BigInt{
		Value:   totalSupply.String(),
		Decimal: 18,
	}, nil
}
