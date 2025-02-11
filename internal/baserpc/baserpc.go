package baserpc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/pkg/errors"

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
	wallet       *EthereumWallet
}

func New(appConfig *config.AppConfig, logger *logger.Logger) (IBaseRPC, error) {
	// Create client for read operations
	client, err := ethclient.Dial(appConfig.Blockchain.BaseRPCEndpoint)
	if err != nil {
		return nil, err
	}

	// Create signer client for write operations
	wallet, err := AccountFromPrivateKey(appConfig.Blockchain.IcySwapSignerPrivateKey)
	if err != nil {
		return nil, err
	}
	fmt.Println("wallet", wallet.publicKeyAddr.Hex())

	icyAddress := common.HexToAddress(appConfig.Blockchain.ICYContractAddr)
	icy, err := erc20.NewErc20(icyAddress, client)
	if err != nil {
		return nil, err
	}

	icySwapAddress := common.HexToAddress(appConfig.Blockchain.ICYSwapContractAddr)
	icySwap, err := icyBtcSwap.NewIcyBtcSwap(icySwapAddress, client)
	if err != nil {
		return nil, err
	}

	return &BaseRPC{
		erc20Service: erc20Service{
			address:         icyAddress,
			icyInstance:     icy,
			icySwapInstance: icySwap,
			client:          client,
		},
		appConfig: appConfig,
		logger:    logger,
		wallet:    wallet,
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

type EthereumWallet struct {
	privateKey    *ecdsa.PrivateKey
	publicKey     ecdsa.PublicKey
	publicKeyAddr common.Address
}

func (w *EthereumWallet) GetPrivateKey() *ecdsa.PrivateKey {
	return w.privateKey
}

func (w *EthereumWallet) GetPublicKey() ecdsa.PublicKey {
	return w.publicKey
}

func GenerateSignature(data []byte, wallet *EthereumWallet) ([]byte, error) {
	digestHash := crypto.Keccak256Hash(data).Bytes()
	byteSignature, err := crypto.Sign(digestHash, wallet.GetPrivateKey())
	if err != nil {
		return nil, err
	}

	return byteSignature, nil
}

func VerifySignature(data []byte, signature string, wallet *EthereumWallet) (bool, error) {
	digestHash := crypto.Keccak256Hash(data).Bytes()

	byteSignature, err := hexutil.Decode(signature)
	if err != nil {
		return false, errors.New("Invalid signature format")
	}

	sigPublicKey, err := crypto.Ecrecover(digestHash, byteSignature)
	if err != nil {
		return false, errors.New("Invalid signature format")
	}

	publickey := wallet.GetPublicKey()
	matches := bytes.Equal(sigPublicKey, crypto.FromECDSAPub(&publickey))

	return matches, nil
}

func AccountFromPrivateKey(privateKeyStr string) (*EthereumWallet, error) {
	acc := &EthereumWallet{}

	fmt.Println("privateKeyStr", privateKeyStr)
	blob, err := hexutil.Decode(privateKeyStr)
	if err != nil {
		fmt.Println("Invalid private format ", err)
		return nil, errors.Wrap(err, "Invalid private format ")
	}

	acc.privateKey, err = crypto.ToECDSA(blob)
	if err != nil {
		return nil, err
	}

	acc.publicKey = acc.privateKey.PublicKey
	acc.publicKeyAddr = crypto.PubkeyToAddress(acc.publicKey)

	return acc, nil
}

// Swap initiates a swap transaction in the IcyBtcSwap contract
func (b *BaseRPC) Swap(
	icyAmount *model.Web3BigInt,
	btcAddress string,
	btcAmount *model.Web3BigInt,
) (*types.Transaction, error) {
	// Validate input parameters
	if icyAmount == nil || btcAmount == nil || btcAddress == "" {
		b.logger.Error("[Swap][InputValidation]", map[string]string{
			"icyAmount":  fmt.Sprintf("%v", icyAmount),
			"btcAddress": btcAddress,
			"btcAmount":  fmt.Sprintf("%v", btcAmount),
		})
		return nil, fmt.Errorf("invalid input: missing or invalid required parameters")
	}

	// Validate amounts are positive
	icyAmountBig := new(big.Int)
	btcAmountBig := new(big.Int)

	if _, ok := icyAmountBig.SetString(icyAmount.Value, 10); !ok {
		b.logger.Error("[Swap][ICYAmountParsing]", map[string]string{
			"icyAmount": icyAmount.Value,
		})
		return nil, fmt.Errorf("invalid ICY amount")
	}

	if _, ok := btcAmountBig.SetString(btcAmount.Value, 10); !ok {
		b.logger.Error("[Swap][BTCAmountParsing]", map[string]string{
			"btcAmount": btcAmount.Value,
		})
		return nil, fmt.Errorf("invalid BTC amount")
	}

	if icyAmountBig.Cmp(big.NewInt(0)) <= 0 || btcAmountBig.Cmp(big.NewInt(0)) <= 0 {
		b.logger.Error("[Swap][AmountValidation]", map[string]string{
			"icyAmount": icyAmountBig.String(),
			"btcAmount": btcAmountBig.String(),
		})
		return nil, fmt.Errorf("swap amounts must be positive")
	}

	// Fetch the chain ID
	chainID, err := b.erc20Service.client.NetworkID(context.Background())
	if err != nil {
		b.logger.Error("[Swap][NetworkID]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get network ID: %v", err)
	}
	// chainID = 84532

	// Create transaction options with signer
	opts, err := bind.NewKeyedTransactorWithChainID(b.wallet.GetPrivateKey(), chainID)
	if err != nil {
		b.logger.Error("[Swap][CreateTransactor]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}
	opts.From = b.wallet.publicKeyAddr

	// Approve the ICYSwap contract to spend tokens
	atx, err := b.erc20Service.icyInstance.Approve(opts, common.HexToAddress(b.appConfig.Blockchain.ICYSwapContractAddr), icyAmountBig)
	if err != nil {
		b.logger.Error("[Swap][Approve]", map[string]string{
			"error":        err.Error(),
			"icyAmount":    icyAmountBig.String(),
			"swapContract": b.appConfig.Blockchain.ICYSwapContractAddr,
		})
		return nil, fmt.Errorf("token approval failed: %v", err)
	}
	b.logger.Info("[Swap][Approve]", map[string]string{
		"txHash": atx.Hash().Hex(),
		"amount": icyAmountBig.String(),
	})

	// Generate nonce and deadline
	nonce := big.NewInt(time.Now().UnixNano())
	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	// Construct the type definitions for the EIP‑712 typed data.
	swapTypes := map[string][]apitypes.Type{
		"EIP712Domain": {
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
		"Swap": {
			{Name: "icyAmount", Type: "uint256"},
			{Name: "btcAddress", Type: "string"},
			{Name: "btcAmount", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		},
	}

	chID := big.NewInt(84532)

	// Construct the domain data as TypedDataDomain.
	swapDomain := apitypes.TypedDataDomain{
		Name:              "ICY BTC SWAP", // Updated to match contract's expected name
		Version:           "1",            // Updated to match contract's expected version
		ChainId:           (*math.HexOrDecimal256)(chID),
		VerifyingContract: b.appConfig.Blockchain.ICYSwapContractAddr,
	}

	// Construct the message payload with exact values (not string representations)
	swapMessage := map[string]interface{}{
		"icyAmount":  icyAmountBig,
		"btcAddress": btcAddress,
		"btcAmount":  btcAmountBig,
		"nonce":      nonce,
		"deadline":   deadline,
	}

	typedData := apitypes.TypedData{
		Types:       swapTypes,
		PrimaryType: "Swap",
		Domain:      swapDomain,
		Message:     swapMessage,
	}

	// Generate EIP-712 signature
	messageHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		b.logger.Error("[Swap][HashMessage]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to hash message: %v", err)
	}

	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		b.logger.Error("[Swap][DomainSeparator]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to hash domain separator: %v", err)
	}

	// Combine domain separator and message hash according to EIP-712
	rawDataToSign := crypto.Keccak256([]byte("\x19\x01"), domainSeparator, messageHash)
	signature, err := crypto.Sign(rawDataToSign, b.wallet.GetPrivateKey())
	if err != nil {
		b.logger.Error("[Swap][SignTypedData]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to sign EIP712 data: %v", err)
	}

	// Adjust signature format for Ethereum (v + 27)
	// signature[64] += 27

	// Log the signature for debugging with additional context
	b.logger.Info("Swap signature generated", map[string]string{
		"signature":  hex.EncodeToString(signature),
		"icyAmount":  icyAmountBig.String(),
		"btcAddress": btcAddress,
		"btcAmount":  btcAmountBig.String(),
		"nonce":      nonce.String(),
		"deadline":   deadline.String(),
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
			"error":      err.Error(),
			"icyAmount":  icyAmountBig.String(),
			"btcAddress": btcAddress,
			"btcAmount":  btcAmountBig.String(),
		})
		return nil, fmt.Errorf("swap transaction failed: %v", err)
	}

	return tx, nil
}
