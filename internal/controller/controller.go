package controller

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

const (
	// Maximum transaction fee threshold (in USD)
	maxTxFee = 1
)

type Controller struct {
	baseRPC   baserpc.IBaseRPC
	btcRPC    btcrpc.IBtcRpc
	oracle    oracle.IOracle
	telemetry telemetry.ITelemetry
	logger    *logger.Logger
	config    *config.AppConfig
	store     *store.Store
	db        *gorm.DB
}

func New(
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	oracle oracle.IOracle,
	telemetry telemetry.ITelemetry,
	logger *logger.Logger,
	config *config.AppConfig,
	store *store.Store,
	db *gorm.DB,
) IController {
	return &Controller{
		baseRPC:   baseRPC,
		btcRPC:    btcRPC,
		oracle:    oracle,
		telemetry: telemetry,
		logger:    logger,
		config:    config,
		store:     store,
		db:        db,
	}
}

func (c *Controller) GetProcessedTxByIcyTransactionHash(txHash string) (*model.OnchainBtcProcessedTransaction, error) {
	// Retrieve the transaction by hash
	tx, err := c.store.OnchainBtcProcessedTransaction.GetByIcyTransactionHash(txHash)
	if err != nil {
		c.logger.Error("[GetOnchainICYTransaction]", map[string]string{
			"error":   err.Error(),
			"tx_hash": txHash,
		})
		return nil, err
	}

	return tx, nil
}

func (c *Controller) TriggerSwap(icyAmount *model.Web3BigInt, btcAmount *model.Web3BigInt, btcAddress string) (string, error) {
	// Input validation
	if btcAmount == nil {
		return "", errors.New("BTC amount cannot be nil")
	}

	// Check for zero or negative BTC amount
	btcFloat := btcAmount.ToFloat()
	if btcFloat <= 0 {
		return "", errors.New("BTC amount must be greater than zero")
	}

	// Validate BTC address format
	if err := c.validateBTCAddress(btcAddress); err != nil {
		return "", fmt.Errorf("invalid BTC address: %w", err)
	}

	// TODO: Implement proper validation that BTC address belongs to user

	tx, err := c.baseRPC.Swap(icyAmount, btcAddress, btcAmount)
	if err != nil {
		c.logger.Error("[TriggerSwap][Swap]", map[string]string{
			"error":      err.Error(),
			"icy_amount": icyAmount.Value,
			"btc_amount": btcAmount.Value,
			"address":    btcAddress,
		})
		return "", fmt.Errorf("swap transaction failed: %w", err)
	}

	// Log successful swap transaction
	txHash := tx.Hash().Hex()
	c.logger.Info("[TriggerSwap][Swap]", map[string]string{
		"tx_hash":     txHash,
		"icy_amount":  icyAmount.Value,
		"btc_amount":  btcAmount.Value,
		"btc_address": btcAddress,
	})

	// TODO: Implement proper verification of swap transaction success
	// This might involve checking transaction receipt, confirmations, or emitted events

	// Proceed with sending BTC
	btcTxHash, err := c.SendBTC(btcAddress, btcAmount)
	if err != nil {
		c.logger.Error("[TriggerSwap][SendBTC]", map[string]string{
			"error":        err.Error(),
			"swap_tx_hash": txHash,
			"btc_amount":   btcAmount.Value,
			"btc_address":  btcAddress,
		})
		return "", fmt.Errorf("failed to send BTC after swap: %w", err)
	}

	return btcTxHash, nil
}

func (c *Controller) ConfirmLatestPrice() (*model.Web3BigInt, error) {
	// Get realtime price from oracle
	price, err := c.oracle.GetRealtimeICYBTC()
	if err != nil {
		return nil, err
	}

	// Compare with cached price to detect significant changes
	cachedPrice, err := c.oracle.GetCachedRealtimeICYBTC()
	if err != nil {
		return nil, err
	}

	// If price has changed significantly, wait for confirmation
	if c.isPriceChangedSignificantly(price, cachedPrice) {
		return nil, errors.New("price changed significantly, waiting for confirmation")
	}

	return price, nil
}

func (c *Controller) SendBTC(address string, amount *model.Web3BigInt) (string, error) {
	// Get current BTC balance
	balance, err := c.btcRPC.CurrentBalance()
	if err != nil {
		c.logger.Error("[SendBTC][CurrentBalance]", map[string]string{
			"error": err.Error(),
		})
		return "", err
	}

	// Validate sufficient balance
	if !c.hasSufficientBalance(balance, amount) {
		return "", fmt.Errorf("insufficient BTC balance: have %f, need %f",
			balance.ToFloat(), amount.ToFloat())
	}

	// Estimate transaction fees with detailed error handling
	fees, err := c.btcRPC.EstimateFees()
	if err != nil {
		// Log detailed error information
		c.logger.Error("[SendBTC][EstimateFees]", map[string]string{
			"error":        err.Error(),
			"btc_amount":   amount.Value,
			"btc_decimals": fmt.Sprintf("%d", amount.Decimal),
		})

		// Provide a more informative error message
		return "", fmt.Errorf("failed to estimate transaction fees: %w", err)
	}

	// Fallback to default fee rate if no fees are returned
	if len(fees) == 0 {
		c.logger.Error("[SendBTC][EstimateFees]", map[string]string{
			"message": "No fee estimates available, using default fee rate",
		})
		// Use a default fee rate if no estimates are available
		fees = map[string]float64{"6": 10.0} // 10 sat/vB as a conservative default
	}

	// Select fee rate for 6 confirmations (standard)
	feeRate, ok := fees["6"]
	if !ok {
		return "", errors.New("unable to get fee rate for 6 confirmations")
	}

	// Estimate transaction fee in USD
	txFeeUSD, err := c.estimateTxFeeUSD(feeRate, amount)
	if err != nil {
		c.logger.Error("[SendBTC][estimateTxFeeUSD]", map[string]string{
			"error": err.Error(),
		})
		return "", err
	}

	// Check if transaction fee exceeds maximum threshold
	if txFeeUSD > maxTxFee {
		return "", fmt.Errorf("transaction fee ($%.2f) exceeds maximum threshold ($%d)", txFeeUSD, maxTxFee)
	}

	return c.btcRPC.Send(address, amount)
}
