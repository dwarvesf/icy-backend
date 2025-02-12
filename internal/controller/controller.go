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

func (c *Controller) TriggerSwap(icyAmount *model.Web3BigInt, satAmount *model.Web3BigInt, btcAddress string) (string, error) {
	// Input validation
	if satAmount == nil {
		return "", errors.New("BTC amount cannot be nil")
	}

	// Check for zero or negative BTC amount
	btcFloat := satAmount.ToFloat()
	if btcFloat <= 0 {
		return "", errors.New("BTC amount must be greater than zero")
	}

	// Validate BTC address format
	if err := c.validateBTCAddress(btcAddress); err != nil {
		return "", fmt.Errorf("invalid BTC address: %w", err)
	}

	// TODO: Implement proper validation that BTC address belongs to user

	tx, err := c.baseRPC.Swap(icyAmount, btcAddress, satAmount)
	if err != nil {
		c.logger.Error("[TriggerSwap][Swap]", map[string]string{
			"error":      err.Error(),
			"icy_amount": icyAmount.Value,
			"sat_amount": satAmount.Value,
			"address":    btcAddress,
		})
		return "", fmt.Errorf("swap transaction failed: %w", err)
	}

	// // Log successful swap transaction
	txHash := tx.Hash().Hex()
	c.logger.Info("[TriggerSwap][Swap]", map[string]string{
		"tx_hash":     txHash,
		"icy_amount":  icyAmount.Value,
		"sat_amount":  satAmount.Value,
		"btc_address": btcAddress,
	})

	return txHash, nil
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
