package telemetry

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
)

func (t *Telemetry) ProcessSwapRequests() error {
	// Fetch pending swap requests by querying for 'pending' status
	pendingSwapRequests, err := t.fetchPendingSwapRequests()
	if err != nil {
		t.logger.Error("[ProcessSwapRequests][FetchPendingRequests]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	for _, req := range pendingSwapRequests {
		// Validate BTC address
		if err := t.validateBTCAddress(req.BTCAddress); err != nil {
			t.logger.Error("[ProcessSwapRequests][ValidateBTCAddress]", map[string]string{
				"error":       err.Error(),
				"btc_address": req.BTCAddress,
			})
			continue
		}

		latestPrice, err := t.confirmLatestPrice()
		if err != nil {
			t.logger.Error("[ProcessSwapRequests][ConfirmLatestPrice]", map[string]string{
				"error": err.Error(),
			})
			continue
		}

		icyAmountFloat, err := strconv.ParseFloat(req.ICYAmount, 64)
		if err != nil {
			t.logger.Error("[ProcessSwapRequests][ParseFloat]", map[string]string{
				"error": err.Error(),
			})
			continue
		}

		icyAmount := &model.Web3BigInt{
			Value:   fmt.Sprintf("%.0f", icyAmountFloat*math.Pow(10, 18)),
			Decimal: 18,
		}

		// Multiply ICY amount by 10^18 to preserve precision
		icyAmountBig := new(big.Int)
		icyAmountBig.SetString(icyAmount.Value, 10)

		priceAmountBig := new(big.Int)
		priceAmountBig.SetString(latestPrice.Value, 10)

		// Perform division with high precision
		satAmountBig := new(big.Int).Div(icyAmountBig, priceAmountBig)

		satAmount := &model.Web3BigInt{
			Value:   satAmountBig.String(),
			Decimal: consts.BTC_DECIMALS,
		}

		// Trigger swap
		swapTxHash, err := t.baseRpc.Swap(icyAmount, req.BTCAddress, satAmount)
		if err != nil {
			t.logger.Error("[ProcessSwapRequests][Swap]", map[string]string{
				"error":       err.Error(),
				"icy_amount":  req.ICYAmount,
				"btc_address": req.BTCAddress,
			})
			continue
		}

		tx := t.db.Begin()

		// Update swap request status
		if err := t.store.SwapRequest.UpdateStatus(tx, req.IcyTx, "completed"); err != nil {
			t.logger.Error("[ProcessSwapRequests][UpdateSwapRequest]", map[string]string{
				"error":   err.Error(),
				"tx_hash": req.IcyTx,
			})
			tx.Rollback()
			continue
			// Continue even if update fails to ensure we don't retry the swap
		}

		// create onchain btc processed transaction
		_, err = t.store.OnchainBtcProcessedTransaction.Create(tx, &model.OnchainBtcProcessedTransaction{
			SwapTransactionHash: swapTxHash.Hash().Hex(),
			IcyTransactionHash:  req.IcyTx,
			BTCAddress:          req.BTCAddress,
			Amount:              satAmount.Value,
			Status:              model.BtcProcessingStatusPending,
		})
		if err != nil {
			t.logger.Error("[ProcessSwapRequests][CreateBtcProcessedTx]", map[string]string{
				"error": err.Error(),
			})
			tx.Rollback()
			continue
		}

		t.logger.Info("[ProcessSwapRequests][SwapCompleted]", map[string]string{
			"tx_hash":     req.IcyTx,
			"icy_amount":  req.ICYAmount,
			"btc_address": req.BTCAddress,
		})

		tx.Commit()
	}

	return nil
}

// fetchPendingSwapRequests retrieves swap requests with 'pending' status
func (t *Telemetry) fetchPendingSwapRequests() ([]*model.SwapRequest, error) {
	// This method would typically be implemented in the swap request store
	// For now, we'll use a direct query
	var pendingRequests []*model.SwapRequest
	result := t.db.Where("status = ?", "pending").Find(&pendingRequests)
	if result.Error != nil {
		return nil, result.Error
	}
	return pendingRequests, nil
}

// validateBTCAddress validates the format of a BTC address
func (t *Telemetry) validateBTCAddress(address string) error {
	// Implement BTC address validation logic
	// This is a placeholder - you'll want to add proper validation
	if address == "" {
		return errors.New("BTC address cannot be empty")
	}
	// Add more specific validation based on BTC address format requirements
	return nil
}

// calculateSatAmount is a placeholder method to convert ICY amount to SAT amount
func (t *Telemetry) calculateSatAmount(icyAmount *model.Web3BigInt) (*model.Web3BigInt, error) {
	// This is a placeholder - you'll want to implement proper price conversion
	// Typically, this would involve:
	// 1. Fetching current ICY/BTC exchange rate
	// 2. Converting ICY amount to SAT based on current rate
	price, err := t.oracle.GetRealtimeICYBTC()
	if err != nil {
		return nil, fmt.Errorf("failed to get realtime price: %w", err)
	}

	// Perform conversion calculation
	// This is a simplified example and should be replaced with actual conversion logic
	satAmount, err := convertICYToSat(icyAmount, price)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ICY to SAT: %w", err)
	}

	return satAmount, nil
}

func (t *Telemetry) confirmLatestPrice() (*model.Web3BigInt, error) {
	// Get realtime price from oracle
	price, err := t.oracle.GetRealtimeICYBTC()
	if err != nil {
		return nil, err
	}

	return price, nil
}

// convertICYToSat is a placeholder conversion function
func convertICYToSat(icyAmount *model.Web3BigInt, price *model.Web3BigInt) (*model.Web3BigInt, error) {
	// Implement actual conversion logic
	// This is a very simplified example and should be replaced with proper conversion
	icyFloat := icyAmount.ToFloat()
	priceFloat := price.ToFloat()

	satFloat := icyFloat * priceFloat

	// Convert back to big int with SAT decimal places (assuming 8 for BTC)
	satBigInt := new(big.Int).SetInt64(int64(satFloat * 1e8))

	return &model.Web3BigInt{
		Value:   satBigInt.String(),
		Decimal: 8, // SAT decimal places
	}, nil
}
