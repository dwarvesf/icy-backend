package btcrpc

import (
	"errors"
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

// ErrNotBroadcast marks a Send failure where the signed transaction was
// DEFINITELY NOT put on the wire: every error raised before the first broadcast
// POST (WIF decode, address decode, amount parse, UTXO selection, insufficient
// funds), plus the fee-adjustment path where the first POST was cleanly REJECTED
// by the node (min-relay-fee) and re-broadcast never happened. Such a failure is
// safe to retry, no BTC left the treasury. Any Send error that is NOT wrapped
// with ErrNotBroadcast is AMBIGUOUS (a POST was attempted and its outcome is
// unknown); the settlement layer must never auto-resend those. Callers test with
// errors.Is(err, btcrpc.ErrNotBroadcast).
var ErrNotBroadcast = errors.New("btc transaction was not broadcast")

type endpointStatus struct {
	failedAt   time.Time
	retryAfter time.Duration
}

type BtcRpc struct {
	appConfig       *config.AppConfig
	logger          *logger.Logger
	blockstreamList []blockstream.IBlockStream // Multiple blockstream instances
	endpoints       []string                   // List of available endpoints
	currentEndpoint int                        // Index of the current active endpoint
	failedEndpoints map[string]*endpointStatus // Map of failed endpoints with their failure time
	mu              sync.RWMutex               // Mutex to protect concurrent access to endpoints
	cch             *cache.Cache
	networkParam    *chaincfg.Params
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
		// Pre-broadcast: nothing was put on the wire, safe to retry.
		return "", 0, fmt.Errorf("failed to get self private key: %v: %w", err, ErrNotBroadcast)
	}

	// Get receiver's address
	receiverAddress, err := btcutil.DecodeAddress(receiverAddressStr, b.networkParam)
	if err != nil {
		b.logger.Error("[btcrpc.Send][DecodeAddress]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, fmt.Errorf("decode receiver address: %v: %w", err, ErrNotBroadcast)
	}

	amountToSend, ok := amount.Int64()
	if !ok {
		b.logger.Error("[btcrpc.Send][Int64]", map[string]string{
			"value": amount.Value,
		})
		return "", 0, fmt.Errorf("failed to convert amount to int64: %w", ErrNotBroadcast)
	}

	// Per-payout cap backstop (SG-06). The settlement orchestrator is the
	// authoritative enforcer (it also routes an over-cap row to a terminal
	// failed state), and at the SAME threshold it never even reaches this call
	// for an over-cap payout. This check is defence-in-depth for ANY direct
	// caller of Send: a single payout can never exceed the per-payout cap. It is
	// tagged ErrNotBroadcast because nothing has been put on the wire (pre-POST),
	// so it is unambiguously safe. 0 disables the cap.
	if maxPayout := b.appConfig.Bitcoin.MaxPayoutSatoshi; maxPayout > 0 && amountToSend > maxPayout {
		b.logger.Error("[btcrpc.Send][PayoutCap] amount exceeds per-payout cap, refusing (not broadcast)", map[string]string{
			"amount": strconv.FormatInt(amountToSend, 10),
			"cap":    strconv.FormatInt(maxPayout, 10),
		})
		return "", 0, fmt.Errorf("payout %d sat exceeds per-payout cap %d sat: %w", amountToSend, maxPayout, ErrNotBroadcast)
	}

	// Select required UTXOs and calculate change amount
	selectedUTXOs, changeAmount, fee, err := b.selectUTXOs(senderAddress.EncodeAddress(), amountToSend)
	if err != nil {
		b.logger.Error("[btcrpc.Send][selectUTXOs]", map[string]string{
			"error":          err.Error(),
			"sender_address": senderAddress.EncodeAddress(),
		})
		// UTXO selection / fee estimation / insufficient funds: all pre-broadcast.
		return "", 0, fmt.Errorf("select utxos: %v: %w", err, ErrNotBroadcast)
	}

	// Create new tx and prepare inputs/outputs
	tx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		b.logger.Error("[btcrpc.Send][prepareTx]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, fmt.Errorf("prepare tx: %v: %w", err, ErrNotBroadcast)
	}

	// Sign tx
	err = b.sign(tx, privKey, senderAddress, selectedUTXOs)
	if err != nil {
		b.logger.Error("[btcrpc.Send][sign]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, fmt.Errorf("sign tx: %v: %w", err, ErrNotBroadcast)
	}

	// Serialize & broadcast tx with potential fee adjustment. From here on an
	// error may be AMBIGUOUS (a POST was attempted), so it is returned UNWRAPPED
	// unless broadcastWithFeeAdjustment itself proves the tx never went out (it
	// tags those with ErrNotBroadcast).
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

	// A *BroadcastTxError means the node CLEANLY REJECTED the first POST (min
	// relay fee not met): the tx did NOT enter the mempool. So every failure in
	// the fee-adjustment prep below leaves the treasury untouched and is tagged
	// ErrNotBroadcast (safe to retry). Only the re-broadcast POST at the end is
	// ambiguous again.
	//
	// KNOWN LIMITATION (currently unreachable branch): this type assertion never
	// succeeds today. broadcast() -> withRetry() re-wraps every endpoint failure
	// with fmt.Errorf("BTC operation failed after ...: %v", lastErr), and %v
	// erases the concrete type, so err is a plain *errorString, not a
	// *blockstream.BroadcastTxError. Consequence: a min-relay-fee rejection is
	// NOT fee-bumped here; it falls through to the ambiguous return below and the
	// settlement layer routes the row to needs_reconcile for manual handling.
	// This is a liveness regression (an auto-recoverable low-fee tx needs a human
	// nudge), NOT a safety bug: the direction is safe (no double-send, no false
	// completion). Restoring the fee-bump requires preserving the typed error
	// through withRetry (e.g. return the raw BroadcastTx error from broadcast()
	// without the withRetry re-wrap, then errors.As here) plus a test that a
	// min-relay rejection triggers the bump rather than needs_reconcile. Left
	// untouched here to avoid shipping an unverified re-broadcast path in money
	// code: the btcrpc test package is build-broken (out of scope) so that test
	// cannot currently be added, and needs_reconcile is the safe status quo.
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
				return "", fmt.Errorf("failed to get fee rates for adjustment: %v: %w", err, ErrNotBroadcast)
			}

			currentFee, err = b.calculateTxFee(feeRates, len(selectedUTXOs), 2, 6)
			if err != nil {
				return "", fmt.Errorf("failed to calculate current fee: %v: %w", err, ErrNotBroadcast)
			}

			if adjustedFee > int64(float64(currentFee)*1.05) {
				return "", fmt.Errorf("fee too high to adjust, adjusted fee: %d, current fee: %d: %w", adjustedFee, currentFee, ErrNotBroadcast)
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
				return "", fmt.Errorf("failed to get fee rates for adjustment: %v: %w", err, ErrNotBroadcast)
			}

			currentFee, err = b.calculateTxFee(feeRates, len(selectedUTXOs), 2, 6)
			if err != nil {
				return "", fmt.Errorf("failed to calculate current fee: %v: %w", err, ErrNotBroadcast)
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
			return "", fmt.Errorf("insufficient funds to adjust transaction fee: %w", ErrNotBroadcast)
		}

		// Recreate transaction with adjusted fee
		adjustedTx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, adjustedChangeAmount)
		if err != nil {
			return "", fmt.Errorf("failed to prepare adjusted transaction: %v: %w", err, ErrNotBroadcast)
		}

		// Re-sign the transaction
		privKey, _, err := b.getSelfPrivKeyAndAddress(b.appConfig.Bitcoin.WalletWIF)
		if err != nil {
			return "", fmt.Errorf("failed to get private key for re-signing: %v: %w", err, ErrNotBroadcast)
		}

		err = b.sign(adjustedTx, privKey, senderAddress, selectedUTXOs)
		if err != nil {
			return "", fmt.Errorf("failed to sign adjusted transaction: %v: %w", err, ErrNotBroadcast)
		}

		// Attempt to broadcast adjusted transaction. This POST is AMBIGUOUS on
		// error (returned unwrapped): its outcome is unknown.
		return b.broadcast(adjustedTx)
	}

	// The first POST failed with a non-rejection error (lost response, read
	// timeout, 5xx after enqueue): the tx MAY be live. AMBIGUOUS: return
	// unwrapped so the settlement layer declines to auto-resend.
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

// GetTransactionConfirmations returns the number of on-chain confirmations for
// txHash across the endpoint pool (0 if unconfirmed / not yet mined / not found).
// Failover between blockstream endpoints is handled by withRetry, mirroring every
// other read here.
func (b *BtcRpc) GetTransactionConfirmations(txHash string) (int64, error) {
	var confirmations int64

	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		confirmations, err = bs.GetTransactionConfirmations(txHash)
		if err != nil {
			b.logger.Error("[GetTransactionConfirmations][blockstream.GetTransactionConfirmations]", map[string]string{
				"error":   err.Error(),
				"tx_hash": txHash,
			})
		}
		return err
	})

	if err != nil {
		return 0, err
	}

	return confirmations, nil
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
