package controller

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
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
}

func New(
	baseRPC baserpc.IBaseRPC,
	btcRPC btcrpc.IBtcRpc,
	oracle oracle.IOracle,
	telemetry telemetry.ITelemetry,
	logger *logger.Logger,
	config *config.AppConfig,
) IController {
	return &Controller{
		baseRPC:   baseRPC,
		btcRPC:    btcRPC,
		oracle:    oracle,
		telemetry: telemetry,
		logger:    logger,
		config:    config,
	}
}

func (c *Controller) TriggerSwap(icyTx string, btcAmount *model.Web3BigInt, btcAddress string) error {
	// Validate ICY transaction hash
	if icyTx == "" {
		return errors.New("ICY transaction hash cannot be empty")
	}

	// Validate BTC amount
	if btcAmount == nil {
		return errors.New("BTC amount cannot be nil")
	}

	// Check for zero or negative BTC amount
	btcFloat := btcAmount.ToFloat()
	if btcFloat <= 0 {
		return errors.New("BTC amount must be greater than zero")
	}

	// Validate BTC address format
	if err := c.validateBTCAddress(btcAddress); err != nil {
		return err
	}

	// Verify ICY transaction exists in the database
	_, err := c.telemetry.GetIcyTransactionByHash(icyTx)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.logger.Error("[TriggerSwap][GetIcyTransactionByHash]", map[string]string{
				"error":  err.Error(),
				"txHash": icyTx,
			})
		}

		// If transaction not found, attempt to index and retry
		if err := c.telemetry.IndexIcyTransaction(); err != nil {
			c.logger.Error("[TriggerSwap][IndexIcyTransaction]", map[string]string{
				"error": err.Error(),
			})
			return err
		}

		// Retry fetching the transaction after indexing
		_, err = c.telemetry.GetIcyTransactionByHash(icyTx)
		if err != nil {
			c.logger.Error("[TriggerSwap][GetIcyTransactionByHash]", map[string]string{
				"error":  err.Error(),
				"txHash": icyTx,
			})
			return errors.New("ICY transaction not found or invalid")
		}
	}

	// Initiate BTC transfer if conditions are met
	return c.TriggerSendBTC(btcAddress, btcAmount)
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

func (c *Controller) TriggerSendBTC(address string, amount *model.Web3BigInt) error {
	// Get current BTC balance
	balance, err := c.btcRPC.CurrentBalance()
	if err != nil {
		c.logger.Error("[TriggerSendBTC][CurrentBalance]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Validate sufficient balance
	if !c.hasSufficientBalance(balance, amount) {
		return fmt.Errorf("insufficient BTC balance: have %f, need %f",
			balance.ToFloat(), amount.ToFloat())
	}

	// Estimate transaction fees
	// find the root cause and fix the error AI!
	fees, err := c.btcRPC.EstimateFees()
	if err != nil {
		c.logger.Error("[TriggerSendBTC][EstimateFees]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Select fee rate for 6 confirmations (standard)
	feeRate, ok := fees["6"]
	if !ok {
		return errors.New("unable to get fee rate for 6 confirmations")
	}

	// Estimate transaction fee in USD
	txFeeUSD, err := c.estimateTxFeeUSD(feeRate, amount)
	if err != nil {
		c.logger.Error("[TriggerSendBTC][estimateTxFeeUSD]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Check if transaction fee exceeds maximum threshold
	if txFeeUSD > maxTxFee {
		return fmt.Errorf("transaction fee ($%.2f) exceeds maximum threshold ($%d)", txFeeUSD, maxTxFee)
	}

	// Send BTC
	return c.btcRPC.Send(address, amount)
}

func (c *Controller) WatchSwapEvents() error {
	// This would implement event watching logic
	// For now return not implemented
	return errors.New("event watching not implemented")
}

// Helper functions

func (c *Controller) isPriceChangedSignificantly(current, cached *model.Web3BigInt) bool {
	if cached == nil {
		return false
	}

	currentFloat := current.ToFloat()
	cachedFloat := cached.ToFloat()

	// Calculate percentage change
	change := ((currentFloat - cachedFloat) / cachedFloat) * 100
	return change >= 5 || change <= -5 // 5% threshold
}

func (c *Controller) hasSufficientBalance(balance, required *model.Web3BigInt) bool {
	balanceFloat := balance.ToFloat()
	requiredFloat := required.ToFloat()
	return balanceFloat >= requiredFloat
}

func (c *Controller) validateBTCAddress(address string) error {
	// Basic validation checks
	if address == "" {
		return errors.New("BTC address cannot be empty")
	}

	// Check address length (typical BTC address lengths)
	if len(address) < 26 || len(address) > 35 {
		return errors.New("invalid BTC address length")
	}

	// Check address starts with valid prefix
	if !(address[0] == '1' || address[0] == '3' || address[0:3] == "bc1") {
		return errors.New("invalid BTC address prefix")
	}

	return nil
}

func (c *Controller) estimateTxFeeUSD(feeRate float64, btcAmount *model.Web3BigInt) (float64, error) {
	// Estimate transaction size (approximation)
	// Assuming 1 input, 2 outputs (recipient and change)
	txSizeBytes := 250 // Typical SegWit transaction size

	// Calculate fee in satoshis
	txFeeSats := int64(float64(txSizeBytes) * feeRate)

	// Convert fee to BTC
	txFeeBTC := float64(txFeeSats) / 100000000.0

	// Get current BTC/USD price
	btcPrice, err := c.oracle.GetRealtimeICYBTC()
	if err != nil {
		return 0, err
	}

	// Calculate fee in USD
	btcPriceFloat := btcPrice.ToFloat()
	txFeeUSD := txFeeBTC * btcPriceFloat

	return txFeeUSD, nil
}
