package baserpc

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethmath "github.com/ethereum/go-ethereum/common/math"
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

type endpointStatus struct {
	failedAt   time.Time
	retryAfter time.Duration
}

type BaseRPC struct {
	appConfig       *config.AppConfig
	logger          *logger.Logger
	erc20Service    erc20Service
	wallet          *EthereumWallet
	chainID         *big.Int
	endpoints       []string                   // List of available RPC endpoints
	currentEndpoint int                        // Index of the current active endpoint
	failedEndpoints map[string]*endpointStatus // Map of failed endpoints with their failure time
	mu              sync.RWMutex               // Mutex to protect concurrent access to endpoints
}

// Default retry interval for failed endpoints
const defaultRetryInterval = 5 * time.Minute

func New(appConfig *config.AppConfig, logger *logger.Logger) (IBaseRPC, error) {
	// Get the list of endpoints
	endpoints := appConfig.Blockchain.BaseRPCEndpoints
	if len(endpoints) == 0 {
		// If no endpoints are configured, use the primary endpoint
		if appConfig.Blockchain.BaseRPCEndpoint != "" {
			endpoints = []string{appConfig.Blockchain.BaseRPCEndpoint}
		} else {
			return nil, fmt.Errorf("no RPC endpoints configured")
		}
	}

	// Initialize with the first endpoint
	baseRPC := &BaseRPC{
		appConfig:       appConfig,
		logger:          logger,
		endpoints:       endpoints,
		currentEndpoint: 0,
		failedEndpoints: make(map[string]*endpointStatus),
	}

	// Try to initialize with each endpoint until one succeeds
	var lastErr error
	for i := 0; i < len(endpoints); i++ {
		baseRPC.currentEndpoint = i
		endpoint := endpoints[i]

		// Skip endpoints that are marked as failed and haven't reached their retry time
		if status, exists := baseRPC.failedEndpoints[endpoint]; exists {
			if time.Since(status.failedAt) < status.retryAfter {
				logger.Info("[New] Skipping failed endpoint", map[string]string{
					"endpoint":   endpoint,
					"failedAt":   status.failedAt.String(),
					"retryAfter": status.retryAfter.String(),
				})
				continue
			}
		}

		err := baseRPC.initClient()
		if err == nil {
			// Successfully initialized - mark endpoint as active
			baseRPC.markEndpointActive(endpoint)
			logger.Info("[New] Successfully initialized BaseRPC client", map[string]string{
				"endpoint": endpoint,
			})
			return baseRPC, nil
		}

		// Mark endpoint as failed
		baseRPC.markEndpointFailed(endpoint, defaultRetryInterval)
		lastErr = err
		logger.Error("[New] Failed to initialize with endpoint", map[string]string{
			"endpoint": endpoint,
			"error":    err.Error(),
		})
	}

	// If we get here, all endpoints failed
	return nil, fmt.Errorf("failed to initialize BaseRPC client with any endpoint: %v", lastErr)
}

// markEndpointFailed marks an endpoint as failed with a retry interval
func (b *BaseRPC) markEndpointFailed(endpoint string, retryAfter time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failedEndpoints[endpoint] = &endpointStatus{
		failedAt:   time.Now(),
		retryAfter: retryAfter,
	}

	b.logger.Info("[markEndpointFailed] Marked endpoint as failed", map[string]string{
		"endpoint":   endpoint,
		"retryAfter": retryAfter.String(),
	})
}

// markEndpointActive removes an endpoint from the failed endpoints list
func (b *BaseRPC) markEndpointActive(endpoint string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.failedEndpoints, endpoint)

	b.logger.Debug("[markEndpointActive] Marked endpoint as active", map[string]string{
		"endpoint": endpoint,
	})
}

// initClient initializes the ethclient and contract instances with the current endpoint
func (b *BaseRPC) initClient() error {
	b.mu.RLock()
	endpoint := b.endpoints[b.currentEndpoint]
	b.mu.RUnlock()

	// Create client for read operations
	client, err := ethclient.Dial(endpoint)
	if err != nil {
		b.logger.Error("[initClient][Dial]", map[string]string{
			"endpoint": endpoint,
			"error":    err.Error(),
		})
		return err
	}

	// Create signer client for write operations
	wallet, err := AccountFromPrivateKey(b.appConfig.Blockchain.IcySwapSignerPrivateKey)
	if err != nil {
		return err
	}

	// Set a timeout context for network ID retrieval
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get the current network chain ID with timeout
	chainID, err := client.NetworkID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get network ID: %v", err)
	}

	icyAddress := common.HexToAddress(b.appConfig.Blockchain.ICYContractAddr)
	icy, err := erc20.NewErc20(icyAddress, client)
	if err != nil {
		return err
	}

	icySwapAddress := common.HexToAddress(b.appConfig.Blockchain.ICYSwapContractAddr)
	icySwap, err := icyBtcSwap.NewIcyBtcSwap(icySwapAddress, client)
	if err != nil {
		return err
	}

	// Update the BaseRPC instance with the new client and services
	b.erc20Service = erc20Service{
		address:         icyAddress,
		icyInstance:     icy,
		icySwapInstance: icySwap,
		client:          client,
	}
	b.wallet = wallet
	b.chainID = chainID

	b.logger.Info("[initClient] Successfully initialized client", map[string]string{
		"endpoint": endpoint,
		"chainID":  chainID.String(),
	})

	return nil
}

// switchEndpoint switches to the next available endpoint that is not marked as failed
// or has reached its retry time
func (b *BaseRPC) switchEndpoint() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.endpoints) <= 1 {
		return fmt.Errorf("no alternative endpoints available")
	}

	// Try to find a non-failed endpoint or one that has reached its retry time
	for i := 0; i < len(b.endpoints); i++ {
		// Move to the next endpoint
		nextIndex := (b.currentEndpoint + 1) % len(b.endpoints)
		endpoint := b.endpoints[nextIndex]

		// Check if this endpoint is failed and hasn't reached its retry time
		if status, exists := b.failedEndpoints[endpoint]; exists {
			if time.Since(status.failedAt) < status.retryAfter {
				// Skip this endpoint
				b.logger.Info("[switchEndpoint] Skipping failed endpoint", map[string]string{
					"endpoint":   endpoint,
					"failedAt":   status.failedAt.String(),
					"retryAfter": status.retryAfter.String(),
				})
				b.currentEndpoint = nextIndex
				continue
			}

			// Endpoint has reached its retry time, we can try it again
			b.logger.Info("[switchEndpoint] Retry time reached for failed endpoint", map[string]string{
				"endpoint":   endpoint,
				"failedAt":   status.failedAt.String(),
				"retryAfter": status.retryAfter.String(),
			})
		}

		// This endpoint is not failed or has reached its retry time
		b.currentEndpoint = nextIndex
		b.logger.Info("[switchEndpoint] Switching to endpoint", map[string]string{
			"endpoint": endpoint,
		})
		return nil
	}

	// If we get here, all endpoints are failed and haven't reached their retry time
	// We'll use the next endpoint anyway and hope for the best
	b.currentEndpoint = (b.currentEndpoint + 1) % len(b.endpoints)
	b.logger.Error("[switchEndpoint] All endpoints are failed, using next endpoint anyway", map[string]string{
		"endpoint": b.endpoints[b.currentEndpoint],
	})

	return nil
}

// withRetry executes a function with retry logic, switching endpoints if necessary
func (b *BaseRPC) withRetry(operation func() error) error {
	maxRetries := len(b.endpoints)
	var lastErr error

	// Get the current endpoint
	b.mu.RLock()
	currentEndpoint := b.endpoints[b.currentEndpoint]
	b.mu.RUnlock()

	// Try the operation with the current endpoint
	err := operation()
	if err == nil {
		// Operation succeeded, mark the endpoint as active
		b.markEndpointActive(currentEndpoint)
		return nil
	}

	// Operation failed, mark the endpoint as failed
	b.markEndpointFailed(currentEndpoint, defaultRetryInterval)
	lastErr = err

	// Try with other endpoints
	for retry := 1; retry < maxRetries; retry++ {
		// Switch to the next endpoint
		if err := b.switchEndpoint(); err != nil {
			return fmt.Errorf("failed to switch endpoint: %v, original error: %v", err, lastErr)
		}

		// Get the new endpoint
		b.mu.RLock()
		currentEndpoint = b.endpoints[b.currentEndpoint]
		b.mu.RUnlock()

		// Initialize the client with the new endpoint
		if err := b.initClient(); err != nil {
			b.logger.Error("[withRetry] Failed to initialize client with new endpoint", map[string]string{
				"endpoint": currentEndpoint,
				"error":    err.Error(),
				"retry":    fmt.Sprintf("%d/%d", retry+1, maxRetries),
			})

			// Mark this endpoint as failed too
			b.markEndpointFailed(currentEndpoint, defaultRetryInterval)
			continue
		}

		// Try the operation with the new endpoint
		err = operation()
		if err == nil {
			// Operation succeeded, mark the endpoint as active
			b.markEndpointActive(currentEndpoint)
			return nil
		}

		// Operation failed, mark the endpoint as failed
		b.markEndpointFailed(currentEndpoint, defaultRetryInterval)
		lastErr = err

		b.logger.Error("[withRetry] Operation failed with endpoint", map[string]string{
			"endpoint": currentEndpoint,
			"error":    err.Error(),
			"retry":    fmt.Sprintf("%d/%d", retry+1, maxRetries),
		})
	}

	return fmt.Errorf("operation failed after %d retries: %v", maxRetries, lastErr)
}

func (b *BaseRPC) Client() *ethclient.Client {
	return b.erc20Service.client
}

func (b *BaseRPC) GetContractAddress() common.Address {
	return common.HexToAddress(b.appConfig.Blockchain.ICYSwapContractAddr)
}

func (b *BaseRPC) ICYBalanceOf(address string) (*model.Web3BigInt, error) {
	var balance *big.Int

	err := b.withRetry(func() error {
		var err error
		balance, err = b.erc20Service.icyInstance.BalanceOf(&bind.CallOpts{}, common.HexToAddress(address))
		return err
	})

	if err != nil {
		return nil, err
	}

	return &model.Web3BigInt{
		Value:   balance.String(),
		Decimal: 18,
	}, nil
}

func (b *BaseRPC) ICYTotalSupply() (*model.Web3BigInt, error) {
	var totalSupply *big.Int

	err := b.withRetry(func() error {
		var err error
		totalSupply, err = b.erc20Service.icyInstance.TotalSupply(&bind.CallOpts{})
		return err
	})

	if err != nil {
		return nil, err
	}

	return &model.Web3BigInt{
		Value:   totalSupply.String(),
		Decimal: 18,
	}, nil
}

func (b *BaseRPC) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainIcyTransaction, error) {
	// Set a longer timeout context for blockchain scanning operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var allTransactions []model.OnchainIcyTransaction
	address = strings.ToLower(address)

	// Get the latest block number with retry
	var latestBlock uint64
	err := b.withRetry(func() error {
		var err error
		latestBlock, err = b.erc20Service.client.BlockNumber(ctx)
		if err != nil {
			b.logger.Error("[GetTransactionsByAddress][BlockNumber]", map[string]string{
				"error": err.Error(),
			})
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	// Determine start block with retry
	startBlock := uint64(0)
	if fromTxId != "" {
		err := b.withRetry(func() error {
			receipt, err := b.erc20Service.client.TransactionReceipt(ctx, common.HexToHash(fromTxId))
			if err != nil {
				b.logger.Error("[GetTransactionsByAddress][TransactionReceipt]", map[string]string{
					"txHash": fromTxId,
					"error":  err.Error(),
				})
				return fmt.Errorf("failed to find transaction %s: %v", fromTxId, err)
			}
			startBlock = receipt.BlockNumber.Uint64()
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Process transactions in batches to avoid block range limitation
	const maxBlockRange = 10000
	for currentStart := startBlock; currentStart <= latestBlock; currentStart += maxBlockRange {
		currentEnd := currentStart + maxBlockRange
		if currentEnd > latestBlock {
			currentEnd = latestBlock
		}
		// Only log block range every 100K blocks to reduce noise
		if currentStart%100000 == 0 {
			b.logger.Info("[GetTransactionsByAddress] block range", map[string]string{
				"startBlock": fmt.Sprintf("%d", currentStart),
				"endBlock":   fmt.Sprintf("%d", currentEnd),
			})
		}

		// Prepare filter options for current block range
		opts := &bind.FilterOpts{
			Start: currentStart,
			End:   &currentEnd,
		}

		// Filter Transfer events with retry
		var iterator *erc20.Erc20TransferIterator
		err := b.withRetry(func() error {
			var err error
			iterator, err = b.erc20Service.icyInstance.FilterTransfer(opts,
				nil, // From address (nil means all addresses)
				nil, // To address (nil means all addresses)
			)
			if err != nil {
				b.logger.Error("[GetTransactionsByAddress][FilterTransfer]", map[string]string{
					"error":      err.Error(),
					"startBlock": fmt.Sprintf("%d", currentStart),
					"endBlock":   fmt.Sprintf("%d", currentEnd),
				})
			}
			return err
		})
		if err != nil {
			return nil, err
		}

		// Convert logs to OnchainIcyTransaction
		var transactions []model.OnchainIcyTransaction
		for iterator.Next() {
			event := iterator.Event
			from := strings.ToLower(event.From.Hex())
			to := strings.ToLower(event.To.Hex())

			transaction := model.OnchainIcyTransaction{
				TransactionHash: event.Raw.TxHash.Hex(),
				Amount:          event.Value.String(),
				BlockNumber:     event.Raw.BlockNumber,
			}
			// Determine transaction type
			if from == address {
				transaction.Type = model.Out
				transaction.ToAddress = event.To.Hex()
			} else if to == address {
				transaction.Type = model.In
				transaction.FromAddress = event.From.Hex()
			} else {
				transaction.Type = model.Transfer
				transaction.FromAddress = event.From.Hex()
				transaction.ToAddress = event.To.Hex()
			}

			// Get block time if possible with retry
			var blockTime int64
			_ = b.withRetry(func() error {
				block, err := b.erc20Service.client.BlockByNumber(ctx, big.NewInt(int64(event.Raw.BlockNumber)))
				if err == nil {
					blockTime = int64(block.Time())
					transaction.BlockTime = blockTime
				}
				return nil // Don't fail if we can't get block time
			})

			transactions = append(transactions, transaction)
		}

		// Log transaction discoveries at debug level to reduce noise
		if len(transactions) > 0 {
			b.logger.Debug("[GetTransactionsByAddress] found transactions", map[string]string{
				"len":        fmt.Sprintf("%d", len(transactions)),
				"startBlock": fmt.Sprintf("%d", currentStart),
				"endBlock":   fmt.Sprintf("%d", currentEnd),
			})
		} else if currentStart%100000 == 0 { // Log progress every 100K blocks
			b.logger.Debug("[GetTransactionsByAddress] scanning progress", map[string]string{
				"startBlock": fmt.Sprintf("%d", currentStart),
				"endBlock":   fmt.Sprintf("%d", currentEnd),
			})
		}

		// Append batch transactions to all transactions
		allTransactions = append(allTransactions, transactions...)

		// limit the number of transactions to fetch
		if len(allTransactions) >= 100 {
			break
		}
	}

	// Log summary of scan results at Info level for operational visibility
	if len(allTransactions) > 0 {
		b.logger.Info("[GetTransactionsByAddress] scan completed", map[string]string{
			"totalTransactions": fmt.Sprintf("%d", len(allTransactions)),
			"blocksScanned":     fmt.Sprintf("%d", latestBlock-startBlock),
		})
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

func AccountFromPrivateKey(privateKeyStr string) (*EthereumWallet, error) {
	acc := &EthereumWallet{}

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

func (b *BaseRPC) GenerateSignature(
	icyAmount *model.Web3BigInt,
	btcAddress string,
	btcAmount *model.Web3BigInt,
	nonce *big.Int,
	deadline *big.Int,
) (string, error) {
	// 1. Validate input parameters.
	if icyAmount == nil || btcAmount == nil || btcAddress == "" {
		b.logger.Error("[Swap][InputValidation]", map[string]string{
			"icyAmount":  fmt.Sprintf("%v", icyAmount),
			"btcAddress": btcAddress,
			"btcAmount":  fmt.Sprintf("%v", btcAmount),
		})
		return "", fmt.Errorf("invalid input: missing or invalid required parameters")
	}

	// 2. Convert amounts from string to *big.Int.
	icyAmountBig := new(big.Int)
	btcAmountBig := new(big.Int)

	if _, ok := icyAmountBig.SetString(icyAmount.Value, 10); !ok {
		b.logger.Error("[Swap][ICYAmountParsing]", map[string]string{
			"icyAmount": icyAmount.Value,
		})
		return "", fmt.Errorf("invalid ICY amount")
	}

	if _, ok := btcAmountBig.SetString(btcAmount.Value, 10); !ok {
		b.logger.Error("[Swap][BTCAmountParsing]", map[string]string{
			"btcAmount": btcAmount.Value,
		})
		return "", fmt.Errorf("invalid BTC amount")
	}

	if icyAmountBig.Cmp(big.NewInt(0)) <= 0 || btcAmountBig.Cmp(big.NewInt(0)) <= 0 {
		b.logger.Error("[Swap][AmountValidation]", map[string]string{
			"icyAmount": icyAmountBig.String(),
			"btcAmount": btcAmountBig.String(),
		})
		return "", fmt.Errorf("swap amounts must be positive")
	}

	// 4. Create transaction options (transactor) with the fetched chainID.
	opts, err := bind.NewKeyedTransactorWithChainID(b.wallet.GetPrivateKey(), b.chainID)
	if err != nil {
		b.logger.Error("[Swap][CreateTransactor]", map[string]string{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to create transactor: %v", err)
	}
	opts.From = b.wallet.publicKeyAddr

	// 5. Approve the ICYSwap contract to spend the tokens with retry.
	var atx *types.Transaction
	swapContractAddr := common.HexToAddress(b.appConfig.Blockchain.ICYSwapContractAddr)

	err = b.withRetry(func() error {
		var err error
		atx, err = b.erc20Service.icyInstance.Approve(opts, swapContractAddr, icyAmountBig)
		if err != nil {
			b.logger.Error("[Swap][Approve]", map[string]string{
				"error":        err.Error(),
				"icyAmount":    icyAmountBig.String(),
				"swapContract": b.appConfig.Blockchain.ICYSwapContractAddr,
			})
			return fmt.Errorf("token approval failed: %v", err)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	b.logger.Info("[Swap][Approve]", map[string]string{
		"txHash": atx.Hash().Hex(),
		"amount": icyAmountBig.String(),
	})

	// 6. Generate a nonce and a deadline if not provided.
	if nonce == nil {
		nonce = big.NewInt(time.Now().UnixNano())
	}
	if deadline == nil {
		deadline = big.NewInt(time.Now().Add(10 * time.Minute).Unix())
	}

	// 7. Construct the EIP-712 typed data.
	// Define the types.
	swapTypes := apitypes.Types{
		"EIP712Domain": []apitypes.Type{
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
		"Swap": []apitypes.Type{
			{Name: "icyAmount", Type: "uint256"},
			{Name: "btcAddress", Type: "string"},
			{Name: "btcAmount", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		},
	}
	// Use the fetched chainID in the domain. Convert chainID to *math.HexOrDecimal256.
	domainChainID := ethmath.NewHexOrDecimal256(b.chainID.Int64())
	swapDomain := apitypes.TypedDataDomain{
		Name:              "ICY BTC SWAP",
		Version:           "1",
		ChainId:           domainChainID,
		VerifyingContract: b.appConfig.Blockchain.ICYSwapContractAddr,
	}

	// Build the message payload.
	// Note: We convert big.Int values to strings so that they are JSON‑marshalable.
	swapMessage := map[string]interface{}{
		"icyAmount":  icyAmountBig.String(),
		"btcAddress": btcAddress,
		"btcAmount":  btcAmountBig.String(),
		"nonce":      nonce.String(),
		"deadline":   deadline.String(),
	}

	typedData := apitypes.TypedData{
		Types:       swapTypes,
		PrimaryType: "Swap",
		Domain:      swapDomain,
		Message:     swapMessage,
	}

	var domainSeparator []byte
	err = b.withRetry(func() error {
		var err error
		domainSeparator, err = typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
		if err != nil {
			b.logger.Error("[Swap][DomainSeparator]", map[string]string{
				"error": err.Error(),
			})
			return fmt.Errorf("failed to hash domain separator: %v", err)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	var messageHash []byte
	err = b.withRetry(func() error {
		var err error
		messageHash, err = typedData.HashStruct(typedData.PrimaryType, typedData.Message)
		if err != nil {
			b.logger.Error("[Swap][HashMessage]", map[string]string{
				"error": err.Error(),
			})
			return fmt.Errorf("failed to hash message: %v", err)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	// EIP-712 requires the data to be hashed as keccak256("\x19\x01" || domainSeparator || messageHash)
	digestBytes := crypto.Keccak256(
		[]byte("\x19\x01"),
		domainSeparator,
		messageHash,
	)
	digest := common.BytesToHash(digestBytes)

	// 9. Sign the digest using the private key.
	var signature []byte
	err = b.withRetry(func() error {
		var err error
		signature, err = crypto.Sign(digest.Bytes(), b.wallet.GetPrivateKey())
		if err != nil {
			b.logger.Error("[Swap][SignTypedData]", map[string]string{
				"error": err.Error(),
			})
			return fmt.Errorf("failed to sign EIP712 data: %v", err)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if signature[64] < 27 {
		signature[64] += 27
	}

	b.logger.Info("Swap signature generated", map[string]string{
		"signature":  hex.EncodeToString(signature),
		"icyAmount":  icyAmountBig.String(),
		"btcAddress": btcAddress,
		"btcAmount":  btcAmountBig.String(),
		"nonce":      nonce.String(),
		"deadline":   deadline.String(),
	})

	return hex.EncodeToString(signature), nil
}

func (b *BaseRPC) Swap(
	icyAmount *model.Web3BigInt,
	btcAddress string,
	btcAmount *model.Web3BigInt,
) (*types.Transaction, error) {
	// Generate a nonce and a deadline
	nonce := big.NewInt(time.Now().UnixNano())
	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	signature, err := b.GenerateSignature(icyAmount, btcAddress, btcAmount, nonce, deadline)
	if err != nil {
		b.logger.Error("[Swap][GenerateSignature]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("signature generation failed: %v", err)
	}

	// Convert amounts from string to *big.Int
	icyAmountBig := new(big.Int)
	btcAmountBig := new(big.Int)

	if _, ok := icyAmountBig.SetString(icyAmount.Value, 10); !ok {
		return nil, fmt.Errorf("invalid ICY amount: %s", icyAmount.Value)
	}

	if _, ok := btcAmountBig.SetString(btcAmount.Value, 10); !ok {
		return nil, fmt.Errorf("invalid BTC amount: %s", btcAmount.Value)
	}

	// Create transaction options (transactor) with the fetched chainID
	opts, err := bind.NewKeyedTransactorWithChainID(b.wallet.GetPrivateKey(), b.chainID)
	if err != nil {
		b.logger.Error("[Swap][CreateTransactor]", map[string]string{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}
	opts.From = b.wallet.publicKeyAddr

	// Convert signature from hex string to byte slice
	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		b.logger.Error("[Swap][SignatureDecode]", map[string]string{
			"error":     err.Error(),
			"signature": signature,
		})
		return nil, fmt.Errorf("failed to decode signature: %v", err)
	}

	// Call the swap method on the ICYSwap contract with retry
	var tx *types.Transaction
	err = b.withRetry(func() error {
		var err error
		tx, err = b.erc20Service.icySwapInstance.Swap(
			opts,
			icyAmountBig,
			btcAddress,
			btcAmountBig,
			nonce,
			deadline,
			signatureBytes,
		)
		if err != nil {
			b.logger.Error("[Swap][IcyBtcSwapInstance]", map[string]string{
				"error":      err.Error(),
				"icyAmount":  icyAmountBig.String(),
				"btcAddress": btcAddress,
				"btcAmount":  btcAmountBig.String(),
			})
			return fmt.Errorf("swap transaction failed: %v", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return tx, nil
}
