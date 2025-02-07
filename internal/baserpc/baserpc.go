package baserpc

import (
	"context"
	"fmt"
	"math/big"

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
	client   *ethclient.Client
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
			client:   client,
		},
		appConfig: appConfig,
		logger:    logger,
	}, nil
}

func (b *BaseRPC) Client() *ethclient.Client {
	return b.erc20Service.client
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
	// Get the latest block number
	latestBlock, err := b.erc20Service.client.BlockNumber(context.Background())
	if err != nil {
		b.logger.Error("[GetTransactionsByAddress][BlockNumber]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	// Determine start block
	startBlock := uint64(0)
	if fromTxId != "" {
		receipt, err := b.erc20Service.client.TransactionReceipt(context.Background(), common.HexToHash(fromTxId))
		if err != nil {
			b.logger.Error("[GetTransactionsByAddress][TransactionReceipt]", map[string]string{
				"txHash": fromTxId,
				"error":  err.Error(),
			})
			return nil, fmt.Errorf("failed to find transaction %s: %v", fromTxId, err)
		}
		startBlock = receipt.BlockNumber.Uint64()
	}

	// Process transactions in batches to avoid block range limitation
	const maxBlockRange = 10000
	var allTransactions []model.OnchainIcyTransaction

	for currentStart := startBlock; currentStart <= latestBlock; currentStart += maxBlockRange {
		currentEnd := currentStart + maxBlockRange
		if currentEnd > latestBlock {
			currentEnd = latestBlock
		}

		// Prepare filter options for current block range
		opts := &bind.FilterOpts{
			Start: currentStart,
			End:   &currentEnd,
		}

		// Filter Transfer events
		iterator, err := b.erc20Service.instance.FilterTransfer(opts,
			[]common.Address{common.HexToAddress(address)},
			[]common.Address{common.HexToAddress(address)},
		)
		if err != nil {
			b.logger.Error("[GetTransactionsByAddress][FilterTransfer]", map[string]string{
				"error":      err.Error(),
				"startBlock": fmt.Sprintf("%d", currentStart),
				"endBlock":   fmt.Sprintf("%d", currentEnd),
			})
			return nil, err
		}

		// Convert logs to OnchainIcyTransaction
		var transactions []model.OnchainIcyTransaction
		for iterator.Next() {
			event := iterator.Event

			// Determine transaction type
			var txType model.TransactionType
			var otherAddress common.Address
			if event.From.Hex() == address {
				txType = model.Out
				otherAddress = event.To
			} else {
				txType = model.In
				otherAddress = event.From
			}

			// Get block time
			block, err := b.erc20Service.client.BlockByNumber(context.Background(), big.NewInt(int64(event.Raw.BlockNumber)))
			var blockTime int64
			if err == nil {
				blockTime = int64(block.Time())
			}

			transactions = append(transactions, model.OnchainIcyTransaction{
				TransactionHash: event.Raw.TxHash.Hex(),
				Amount:          event.Value.String(),
				Type:            txType,
				OtherAddress:    otherAddress.Hex(),
				BlockTime:       blockTime,
			})
		}

		// Append batch transactions to all transactions
		allTransactions = append(allTransactions, transactions...)
	}

	return allTransactions, nil
}
