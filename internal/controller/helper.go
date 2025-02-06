package controller

import (
	"errors"

	"github.com/dwarvesf/icy-backend/internal/model"
)

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
