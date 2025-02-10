package baserpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/dwarvesf/icy-backend/contracts/erc20"
	"github.com/dwarvesf/icy-backend/contracts/icyBtcSwap"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type erc20Service struct {
	address         common.Address
	icyInstance     *erc20.Erc20
	icySwapInstance *icyBtcSwap.IcyBtcSwap
	client          *ethclient.Client
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

	// new signer client by using appConfig.Blockchain.IcySwapSignerPrivateKey AI!




	icySwapAddress := common.HexToAddress(appConfig.Blockchain.ICYSwapContractAddr)
	icySwap, err := icyBtcSwap.NewIcyBtcSwap(icySwapAddress, signerWalletAddress)
	if err != nil {
		return nil, err
	}


	return &BaseRPC{
		erc20Service: erc20Service{
			address:         icyAddress,
			icyInstance:     icy,
			icySwapInstance: icySwap,
			client:          client,
			signer: ....,
		},
		appConfig: appConfig,
		logger:    logger,
	}, nil
}

func (b *BaseRPC) Client() *ethclient.Client {
	return b.erc20Service.client
}

func (b *BaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
	balance, err := b.erc20Service.icyInstance.BalanceOf(&bind.CallOpts{}, common.HexToAddress(address))
	if err != nil {
		return nil, err
	}
	return &model.Web3BigInt{
		Value:   balance.String(),
		Decimal: 18,
	}, nil
}

func (b *BaseRPC) ICYTotalSupply() (*model.Web3BigInt, error) {
	totalSupply, err := b.erc20Service.icyInstance.TotalSupply(&bind.CallOpts{})
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

		// Filter Transfer events for all transactions involving the contract address
		iterator, err := b.erc20Service.icyInstance.FilterTransfer(opts,
			nil, // From address (nil means all addresses)
			nil, // To address (nil means all addresses)
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

			// Skip if neither from nor to address is the target address, only interested in transactions related to the contract address
			if event.From.Hex() != address && event.To.Hex() != address {
				continue
			}

			// Determine transaction type
			var txType model.TransactionType
			var otherAddress common.Address
			if event.From.Hex() == address {
				txType = model.Out
				otherAddress = event.To
			} else if event.To.Hex() == address {
				txType = model.In
				otherAddress = event.From
			}

			// Get block time if possible
			block, err := b.erc20Service.client.BlockByNumber(context.Background(), big.NewInt(int64(event.Raw.BlockNumber)))
			var blockTime int64
			if err == nil {
				blockTime = int64(block.Time())
			} else {
				b.logger.Error("[GetTransactionsByAddress][BlockByNumber] cannot get block data", map[string]string{
					"error": err.Error(),
				})
			}

			transactions = append(transactions, model.OnchainIcyTransaction{
				TransactionHash: event.Raw.TxHash.Hex(),
				Amount:          event.Value.String(),
				Type:            txType,
				OtherAddress:    otherAddress.Hex(),
				BlockTime:       blockTime,
				BlockNumber:     event.Raw.BlockNumber,
			})
		}

		// Append batch transactions to all transactions
		allTransactions = append(allTransactions, transactions...)
	}

	return allTransactions, nil
}

// Swap initiates a swap transaction in the IcyBtcSwap contract
func (b *BaseRPC) Swap(
	icyAmount *model.Web3BigInt,
	btcAddress string,
	btcAmount *model.Web3BigInt,
) (*types.Transaction, error) {
	// Prepare transaction options
	opts := &bind.TransactOpts{
		From: b.erc20Service.address,
		// Note: You might want to set gas limit, gas price, etc. based on your requirements
	}

	// Generate nonce if not provided
	nonce := big.NewInt(time.Now().UnixNano())
	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	// Use signature from app configuration if not provided
	// Remove '0x' prefix before decoding
	signatureStr := b.appConfig.Blockchain.IcySwapSignerPrivateKey
	if len(signatureStr) > 2 && signatureStr[:2] == "0x" {
		signatureStr = signatureStr[2:]
	}

	signature, err := hex.DecodeString(signatureStr)
	if err != nil {
		b.logger.Error("[Swap][DecodeSignature]", map[string]string{
			"error":     err.Error(),
			"signature": signatureStr,
		})
		return nil, fmt.Errorf("failed to decode swap signature: %v", err)
	}

	// Convert Web3BigInt to *big.Int
	icyAmountBig := new(big.Int)
	icyAmountBig.SetString(icyAmount.Value, 10)

	btcAmountBig := new(big.Int)
	btcAmountBig.SetString(btcAmount.Value, 10)
	b.logger.Info("[Swap][Swap]", map[string]string{
		"icyAmount":  icyAmount.Value,
		"btcAddress": btcAddress,
		"btcAmount":  btcAmountBig.String(),
		"nonce":      nonce.String(),
		"deadline":   deadline.String(),
		"signature":  fmt.Sprintf("%x", signature),
	})

	// Call the swap method on the IcyBtcSwap contract
	tx, err := b.erc20Service.icySwapInstance.Swap(
		opts,
		icyAmountBig,
		btcAddress,
		btcAmountBig,
		nonce,
		deadline,
		signature,
	)
	if err != nil {
		b.logger.Error("[Swap][IcyBtcSwapInstance]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	return tx, nil
}
