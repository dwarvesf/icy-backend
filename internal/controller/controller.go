package controller

import (
	"errors"

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
	// Basic validation of ICY transaction hash
	if icyTx == "" {
		return errors.New("BTC address cannot be empty")
	}

	// Validate input parameters
	if btcAmount == nil {
		return errors.New("BTC amount cannot be nil")
	}

	// Check for zero or negative BTC amount
	btcFloat := btcAmount.ToFloat()
	if btcFloat <= 0 {
		return errors.New("BTC amount must be greater than zero")
	}

	// Basic validation of BTC address (can be expanded based on specific BTC address format)
	if btcAddress == "" {
		return errors.New("BTC address cannot be empty")
	}

	// calculate transaction fee when sending BTC AI!

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
		return err
	}

	// Validate sufficient balance
	if !c.hasSufficientBalance(balance, amount) {
		return errors.New("insufficient BTC balance")
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
