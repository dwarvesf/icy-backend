package baserpc

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
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

func (b *BaseRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainIcyTransaction, error) {
	client, err := ethclient.Dial(b.appConfig.Blockchain.BaseRPCEndpoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	contractAddress := common.HexToAddress(address)

	// Determine starting block
	var fromBlock *big.Int = big.NewInt(0)
	if fromTxId != "" {
		// Find the transaction receipt to get its block number
		receipt, err := client.TransactionReceipt(context.Background(), common.HexToHash(fromTxId))
		if err != nil {
			return nil, fmt.Errorf("failed to find transaction %s: %v", fromTxId, err)
		}

		// Get the block number
		fromBlock = receipt.BlockNumber
	}

	// Create filter query for Transfer events
	query := ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   nil, // latest block
		Addresses: []common.Address{contractAddress},
		Topics: [][]common.Hash{
			{crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))},
		},
	}

	// Fetch logs
	logs, err := client.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, err
	}

	// Convert logs to OnchainIcyTransaction
	var transactions []model.OnchainIcyTransaction
	for _, vLog := range logs {
		from := common.HexToAddress(vLog.Topics[1].Hex())
		to := common.HexToAddress(vLog.Topics[2].Hex())

		// Determine transaction type
		var txType model.TransactionType
		var otherAddress common.Address
		if from.Hex() == address {
			txType = model.Out
			otherAddress = to
		} else if to.Hex() == address {
			txType = model.In
			otherAddress = from
		} else {
			continue
		}

		// Get block time
		block, err := client.BlockByNumber(context.Background(), big.NewInt(int64(vLog.BlockNumber)))
		var blockTime int64
		if err == nil {
			blockTime = int64(block.Time())
		}

		// Convert amount
		amount := new(big.Int)
		amount.SetBytes(vLog.Data)

		transactions = append(transactions, model.OnchainIcyTransaction{
			TransactionHash: vLog.TxHash.Hex(),
			Amount:          amount.String(),
			Type:            txType,
			OtherAddress:    otherAddress.Hex(),
			BlockTime:       blockTime,
		})
	}

	return transactions, nil
}
