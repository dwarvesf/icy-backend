package btcrpc

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/patrickmn/go-cache"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type endpointStatus struct {
	failedAt   time.Time
	retryAfter time.Duration
}

type BtcRpc struct {
	appConfig         *config.AppConfig
	logger            *logger.Logger
	blockstreamList   []blockstream.IBlockStream // Multiple blockstream instances
	endpoints         []string                    // List of available endpoints
	currentEndpoint   int                         // Index of the current active endpoint
	failedEndpoints   map[string]*endpointStatus  // Map of failed endpoints with their failure time
	mu                sync.RWMutex                // Mutex to protect concurrent access to endpoints
	cch               *cache.Cache
	networkParam      *chaincfg.Params
}

// Default retry interval for failed endpoints
const defaultRetryInterval = 5 * time.Minute

func New(appConfig *config.AppConfig, logger *logger.Logger) IBtcRpc {
	networkParams := &chaincfg.TestNet3Params
	if appConfig.ApiServer.AppEnv == "prod" {
		networkParams = &chaincfg.MainNetParams
	}

	// Get the list of endpoints
	endpoints := appConfig.Bitcoin.BlockstreamAPIURLs
	if len(endpoints) == 0 {
		// If no endpoints are configured, use the primary endpoint
		if appConfig.Bitcoin.BlockstreamAPIURL != "" {
			endpoints = []string{appConfig.Bitcoin.BlockstreamAPIURL}
		} else {
			logger.Error("[New] No BTC endpoints configured", nil)
			return nil
		}
	}

	// Create blockstream instances for each endpoint
	blockstreamList := make([]blockstream.IBlockStream, len(endpoints))
	for i, endpoint := range endpoints {
		blockstreamList[i] = blockstream.NewWithURL(appConfig, logger, endpoint)
	}

	btcRpc := &BtcRpc{
		appConfig:       appConfig,
		logger:          logger,
		blockstreamList: blockstreamList,
		endpoints:       endpoints,
		currentEndpoint: 0,
		failedEndpoints: make(map[string]*endpointStatus),
		cch:             cache.New(1*time.Minute, 2*time.Minute),
		networkParam:    networkParams,
	}

	logger.Info("[New] Successfully initialized BtcRpc with multiple endpoints", map[string]string{
		"endpoints": fmt.Sprintf("%v", endpoints),
	})

	return btcRpc
}

// markEndpointFailed marks an endpoint as failed with a retry interval
func (b *BtcRpc) markEndpointFailed(endpoint string, retryAfter time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failedEndpoints[endpoint] = &endpointStatus{
		failedAt:   time.Now(),
		retryAfter: retryAfter,
	}

	b.logger.Info("[markEndpointFailed] Marked BTC endpoint as failed", map[string]string{
		"endpoint":   endpoint,
		"retryAfter": retryAfter.String(),
	})
}

// markEndpointActive removes an endpoint from the failed endpoints list
func (b *BtcRpc) markEndpointActive(endpoint string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.failedEndpoints, endpoint)

	b.logger.Info("[markEndpointActive] Marked BTC endpoint as active", map[string]string{
		"endpoint": endpoint,
	})
}

// switchEndpoint switches to the next available endpoint that is not marked as failed
// or has reached its retry time
func (b *BtcRpc) switchEndpoint() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.endpoints) <= 1 {
		return fmt.Errorf("no alternative BTC endpoints available")
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
				b.logger.Info("[switchEndpoint] Skipping failed BTC endpoint", map[string]string{
					"endpoint":   endpoint,
					"failedAt":   status.failedAt.String(),
					"retryAfter": status.retryAfter.String(),
				})
				b.currentEndpoint = nextIndex
				continue
			}

			// Endpoint has reached its retry time, we can try it again
			b.logger.Info("[switchEndpoint] Retry time reached for failed BTC endpoint", map[string]string{
				"endpoint":   endpoint,
				"failedAt":   status.failedAt.String(),
				"retryAfter": status.retryAfter.String(),
			})
		}

		// This endpoint is not failed or has reached its retry time
		b.currentEndpoint = nextIndex
		b.logger.Info("[switchEndpoint] Switching to BTC endpoint", map[string]string{
			"endpoint": endpoint,
		})
		return nil
	}

	// If we get here, all endpoints are failed and haven't reached their retry time
	// We'll use the next endpoint anyway and hope for the best
	b.currentEndpoint = (b.currentEndpoint + 1) % len(b.endpoints)
	b.logger.Error("[switchEndpoint] All BTC endpoints are failed, using next endpoint anyway", map[string]string{
		"endpoint": b.endpoints[b.currentEndpoint],
	})

	return nil
}

// withRetry executes a function with retry logic, switching endpoints if necessary
func (b *BtcRpc) withRetry(operation func(blockstream.IBlockStream) error) error {
	maxRetries := len(b.endpoints)
	var lastErr error

	// Get the current endpoint
	b.mu.RLock()
	currentEndpoint := b.endpoints[b.currentEndpoint]
	currentBlockstream := b.blockstreamList[b.currentEndpoint]
	b.mu.RUnlock()

	// Try the operation with the current endpoint
	err := operation(currentBlockstream)
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
			return fmt.Errorf("failed to switch BTC endpoint: %v, original error: %v", err, lastErr)
		}

		// Get the new endpoint
		b.mu.RLock()
		currentEndpoint = b.endpoints[b.currentEndpoint]
		currentBlockstream = b.blockstreamList[b.currentEndpoint]
		b.mu.RUnlock()

		// Try the operation with the new endpoint
		err = operation(currentBlockstream)
		if err == nil {
			// Operation succeeded, mark the endpoint as active
			b.markEndpointActive(currentEndpoint)
			return nil
		}

		// Operation failed, mark the endpoint as failed
		b.markEndpointFailed(currentEndpoint, defaultRetryInterval)
		lastErr = err

		b.logger.Error("[withRetry] BTC operation failed with endpoint", map[string]string{
			"endpoint": currentEndpoint,
			"error":    err.Error(),
			"retry":    fmt.Sprintf("%d/%d", retry+1, maxRetries),
		})
	}

	return fmt.Errorf("BTC operation failed after %d retries: %v", maxRetries, lastErr)
}

func (b *BtcRpc) Send(receiverAddressStr string, amount *model.Web3BigInt) (string, int64, error) {
	// Get sender's priv key and address
	privKey, senderAddress, err := b.getSelfPrivKeyAndAddress(b.appConfig.Bitcoin.WalletWIF)
	if err != nil {
		b.logger.Error("[btcrpc.Send][getSelfPrivKeyAndAddress]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, fmt.Errorf("failed to get self private key: %v", err)
	}

	// Get receiver's address
	receiverAddress, err := btcutil.DecodeAddress(receiverAddressStr, b.networkParam)
	if err != nil {
		b.logger.Error("[btcrpc.Send][DecodeAddress]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	amountToSend, ok := amount.Int64()
	if !ok {
		b.logger.Error("[btcrpc.Send][Int64]", map[string]string{
			"value": amount.Value,
		})
		return "", 0, fmt.Errorf("failed to convert amount to int64")
	}

	// Select required UTXOs and calculate change amount
	selectedUTXOs, changeAmount, fee, err := b.selectUTXOs(senderAddress.EncodeAddress(), amountToSend)
	if err != nil {
		b.logger.Error("[btcrpc.Send][selectUTXOs]", map[string]string{
			"error":          err.Error(),
			"sender_address": senderAddress.EncodeAddress(),
		})
		return "", 0, err
	}

	// Create new tx and prepare inputs/outputs
	tx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		b.logger.Error("[btcrpc.Send][prepareTx]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	// Sign tx
	err = b.sign(tx, privKey, senderAddress, selectedUTXOs)
	if err != nil {
		b.logger.Error("[btcrpc.Send][sign]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	// Serialize & broadcast tx with potential fee adjustment
	txID, err := b.broadcastWithFeeAdjustment(tx, selectedUTXOs, receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		b.logger.Error("[btcrpc.Send][broadcast]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	return txID, fee, nil
}

// broadcastWithFeeAdjustment attempts to broadcast the transaction,
// and if it fails due to minimum relay fee, attempts to increase the fee by 5%
func (b *BtcRpc) broadcastWithFeeAdjustment(
	tx *wire.MsgTx,
	selectedUTXOs []blockstream.UTXO,
	receiverAddress btcutil.Address,
	senderAddress *btcutil.AddressWitnessPubKeyHash,
	amountToSend, changeAmount int64,
) (string, error) {
	// First attempt to broadcast
	txID, err := b.broadcast(tx)
	if err == nil {
		return txID, nil
	}

	// Check if the error is specifically about minimum relay fee
	broadcastErr, ok := err.(*blockstream.BroadcastTxError)
	if ok {
		b.logger.Info("[btcrpc.Send][FeeAdjustment]", map[string]string{
			"message": "Attempting to adjust transaction fee",
		})

		// Use the minimum fee from the error if available
		var adjustedFee, currentFee int64
		if broadcastErr.MinFee > 0 {
			// Use the minimum fee from the error
			adjustedFee = broadcastErr.MinFee

			// Fallback to calculating current fee if no minimum fee in error
			var feeRates map[string]float64
			err = b.withRetry(func(bs blockstream.IBlockStream) error {
				var err error
				feeRates, err = bs.EstimateFees()
				return err
			})
			if err != nil {
				return "", fmt.Errorf("failed to get fee rates for adjustment: %v", err)
			}

			currentFee, err = b.calculateTxFee(feeRates, len(selectedUTXOs), 2, 6)
			if err != nil {
				return "", fmt.Errorf("failed to calculate current fee: %v", err)
			}

			if adjustedFee > int64(float64(currentFee)*1.05) {
				return "", fmt.Errorf("fee too high to adjust, adjusted fee: %d, current fee: %d", adjustedFee, currentFee)
			}
		} else {
			// Fallback to calculating fee if no minimum fee in error
			var feeRates map[string]float64
			err = b.withRetry(func(bs blockstream.IBlockStream) error {
				var err error
				feeRates, err = bs.EstimateFees()
				return err
			})
			if err != nil {
				return "", fmt.Errorf("failed to get fee rates for adjustment: %v", err)
			}

			currentFee, err = b.calculateTxFee(feeRates, len(selectedUTXOs), 2, 6)
			if err != nil {
				return "", fmt.Errorf("failed to calculate current fee: %v", err)
			}

			// Adjust fee to be 5% higher
			adjustedFee = int64(float64(currentFee) * 1.05)
		}

		b.logger.Info("[btcrpc.Send][FeeAdjustment]", map[string]string{
			"currentFee":   strconv.FormatInt(currentFee, 10),
			"adjustedFee":  strconv.FormatInt(adjustedFee, 10),
			"changeAmount": strconv.FormatInt(changeAmount, 10),
			"amountToSend": strconv.FormatInt(amountToSend, 10),
		})

		// Calculate adjusted change amount
		adjustedChangeAmount := changeAmount - (adjustedFee - currentFee)

		// If adjusted change amount becomes negative, we can't proceed
		if adjustedChangeAmount < 0 {
			return "", fmt.Errorf("insufficient funds to adjust transaction fee")
		}

		// Recreate transaction with adjusted fee
		adjustedTx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, adjustedChangeAmount)
		if err != nil {
			return "", fmt.Errorf("failed to prepare adjusted transaction: %v", err)
		}

		// Re-sign the transaction
		privKey, _, err := b.getSelfPrivKeyAndAddress(b.appConfig.Bitcoin.WalletWIF)
		if err != nil {
			return "", fmt.Errorf("failed to get private key for re-signing: %v", err)
		}

		err = b.sign(adjustedTx, privKey, senderAddress, selectedUTXOs)
		if err != nil {
			return "", fmt.Errorf("failed to sign adjusted transaction: %v", err)
		}

		// Attempt to broadcast adjusted transaction
		return b.broadcast(adjustedTx)
	}

	// If it's a different error, return the original error
	return "", err
}

func (b *BtcRpc) CurrentBalance() (*model.Web3BigInt, error) {
	var balance *model.Web3BigInt

	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		balance, err = bs.GetBTCBalance(b.appConfig.Blockchain.BTCTreasuryAddress)
		if err != nil {
			b.logger.Error("[CurrentBalance][GetBTCBalance]", map[string]string{
				"error": err.Error(),
			})
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	return balance, nil
}

func (b *BtcRpc) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error) {
	var rawTx []blockstream.Transaction

	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		rawTx, err = bs.GetTransactionsByAddress(address, fromTxId)
		if err != nil {
			b.logger.Error("[GetTransactionsByAddress][GetTransactionsByAddress]", map[string]string{
				"error": err.Error(),
			})
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	// Filter out unconfirmed transactions
	confirmedTx := make([]blockstream.Transaction, 0)
	for _, tx := range rawTx {
		if tx.TxID == fromTxId {
			break
		}
		if tx.Status.Confirmed {
			confirmedTx = append(confirmedTx, tx)
		}
	}

	transactions := make([]model.OnchainBtcTransaction, 0)
	for _, tx := range confirmedTx {
		var isOutgoing bool
		var senderAddress string
		for _, input := range tx.Vin {
			prevOut := input.Prevout
			if prevOut != nil {
				if prevOut.ScriptPubKeyAddress == address {
					isOutgoing = true
				} else {
					senderAddress = prevOut.ScriptPubKeyAddress
				}
			}
		}

		if isOutgoing {
			for _, output := range tx.Vout {
				if output.ScriptPubKeyAddress != address {
					transactions = append(transactions, model.OnchainBtcTransaction{
						TransactionHash: tx.TxID,
						Amount:          strconv.FormatInt(output.Value, 10),
						Type:            model.Out,
						OtherAddress:    output.ScriptPubKeyAddress,
						BlockTime:       tx.Status.BlockTime,
						InternalID:      tx.TxID,
						Fee:             strconv.FormatInt(tx.Fee, 10),
					})
				}
			}
		} else {
			for _, output := range tx.Vout {
				if output.ScriptPubKeyAddress == address {
					transactions = append(transactions, model.OnchainBtcTransaction{
						TransactionHash: tx.TxID,
						Amount:          strconv.FormatInt(output.Value, 10),
						Type:            model.In,
						OtherAddress:    senderAddress,
						BlockTime:       tx.Status.BlockTime,
						InternalID:      tx.TxID,
					})
				}
			}
		}
	}
	return transactions, nil
}

// EstimateFees retrieves current Bitcoin transaction fee estimates
func (b *BtcRpc) EstimateFees() (map[string]float64, error) {
	var fees map[string]float64

	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		fees, err = bs.EstimateFees()
		if err != nil {
			b.logger.Error("[EstimateFees][blockstream.EstimateFees]", map[string]string{
				"error": err.Error(),
			})
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	return fees, nil
}
