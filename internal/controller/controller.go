package controller

import (
	"errors"
	"math"
	"math/big"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/oracle"
	"github.com/dwarvesf/icy-backend/internal/telemetry"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

const (
	// Maximum transaction fee threshold (5% of transaction amount)
	maxTxFeePercentage = 5
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

func (c *Controller) TriggerSwap(icyAmount *model.Web3BigInt, btcAddress string) error {
	// First confirm latest price to ensure swap rate is valid
	latestPrice, err := c.ConfirmLatestPrice()
	if err != nil {
		c.logger.Error("[TriggerSwap][ConfirmLatestPrice]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Calculate BTC amount based on ICY amount and latest price
	icyFloat := icyAmount.ToFloat()
	priceFloat := latestPrice.ToFloat()
	btcFloat := icyFloat / priceFloat

	// Convert BTC amount to Web3BigInt with 8 decimals
	btcValue := new(big.Float).Mul(new(big.Float).SetFloat64(btcFloat), new(big.Float).SetFloat64(math.Pow10(8)))
	btcValueInt, _ := btcValue.Int(nil)
	btcAmount := &model.Web3BigInt{
		Value:   btcValueInt.String(),
		Decimal: 8,
	}

	// Trigger telemetry indexing to ensure latest state
	if err := c.telemetry.IndexBtcTransaction(); err != nil {
		c.logger.Error("[TriggerSwap][IndexBtcTransaction]", map[string]string{
			"error": err.Error(),
		})
		return err
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
