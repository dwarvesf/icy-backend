package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/contracts/icyBtcSwap"
	"github.com/dwarvesf/icy-backend/internal/consts"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
)

// IndexIcySwapTransaction fetches and stores Swap events from the contract
func (t *Telemetry) IndexIcySwapTransaction() error {
	// Prevent concurrent executions
	t.indexIcySwapTransactionMutex.Lock()
	defer t.indexIcySwapTransactionMutex.Unlock()

	t.logger.Info("[IndexIcySwapTransaction] Start indexing ICY swap transactions...")

	// Get latest processed transaction
	latestTx, err := t.store.OnchainIcySwapTransaction.GetLatestTransaction(t.db)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.logger.Error("[IndexIcySwapTransaction][GetLatestTransaction]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
		t.logger.Info("[IndexIcySwapTransaction] No previous transactions found. Starting from the beginning.")
	}

	// Determine the starting block
	startBlock := uint64(t.appConfig.Blockchain.InitialICYSwapBlockNumber)
	if latestTx != nil && latestTx.TransactionHash != "" {
		t.logger.Info(fmt.Sprintf("[IndexIcySwapTransaction] Latest ICY swap transaction: %s", latestTx.TransactionHash))
		receipt, err := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(latestTx.TransactionHash))
		if err != nil {
			t.logger.Error("[IndexIcySwapTransaction][LastTransactionReceipt]", map[string]string{
				"txHash": latestTx.TransactionHash,
				"error":  err.Error(),
			})
		} else {
			startBlock = receipt.BlockNumber.Uint64() + 1
		}
	}

	// Get latest block number
	latestBlock, err := t.baseRpc.Client().BlockNumber(context.Background())
	if err != nil {
		t.logger.Error("[IndexIcySwapTransaction][GetLatestBlock]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Get contract instance
	contract, err := icyBtcSwap.NewIcyBtcSwap(t.baseRpc.GetContractAddress(), t.baseRpc.Client())
	if err != nil {
		t.logger.Error("[IndexIcySwapTransaction][NewIcyBtcSwap]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Process blocks in batches
	const maxBlockRange = uint64(10000)
	var totalProcessed int
	for currentStart := startBlock; currentStart <= latestBlock; currentStart += maxBlockRange {
		currentEnd := currentStart + maxBlockRange
		if currentEnd > latestBlock {
			currentEnd = latestBlock
		}

		t.logger.Info("[IndexIcySwapTransaction]", map[string]string{
			"startBlock":  fmt.Sprintf("%d", currentStart),
			"endBlock":    fmt.Sprintf("%d", currentEnd),
			"latestBlock": fmt.Sprintf("%d", latestBlock),
		})

		// Create filter options for current block range
		filterOpts := &bind.FilterOpts{
			Start:   currentStart,
			End:     &currentEnd,
			Context: context.Background(),
		}

		// Filter Swap events
		swapEvents, err := contract.FilterSwap(filterOpts)
		if err != nil {
			t.logger.Error("[IndexIcySwapTransaction][FilterSwap]", map[string]string{
				"error": err.Error(),
			})
			return err
		}

		// Process events in batches
		var txsToStore []*model.OnchainIcySwapTransaction
		for swapEvents.Next() {
			event := swapEvents.Event
			if event == nil {
				continue
			}

			// Get transaction to extract from address
			transaction, isPending, err := t.baseRpc.Client().TransactionByHash(context.Background(), event.Raw.TxHash)
			if err != nil {
				t.logger.Error("[IndexIcySwapTransaction][GetTransaction]", map[string]string{
					"error":   err.Error(),
					"tx_hash": event.Raw.TxHash.Hex(),
				})
				continue
			}

			// Skip pending transactions
			if isPending {
				continue
			}

			// Get sender address
			signer := types.NewLondonSigner(transaction.ChainId())
			from, err := types.Sender(signer, transaction)
			if err != nil {
				t.logger.Error("[IndexIcySwapTransaction][GetSender]", map[string]string{
					"error":   err.Error(),
					"tx_hash": event.Raw.TxHash.Hex(),
				})
				continue
			}

			// Create transaction record
			tx := &model.OnchainIcySwapTransaction{
				TransactionHash: event.Raw.TxHash.Hex(),
				BlockNumber:     event.Raw.BlockNumber,
				IcyAmount:       event.IcyAmount.String(),
				FromAddress:     from.Hex(),
				BtcAddress:      event.BtcAddress,
				BtcAmount:       event.BtcAmount.String(),
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}

			txsToStore = append(txsToStore, tx)
		}
		swapEvents.Close()

		// Store transactions in a single transaction
		if len(txsToStore) > 0 {
			err = store.DoInTx(t.db, func(tx *gorm.DB) error {
				for _, swapTx := range txsToStore {
					_, err := t.store.OnchainIcySwapTransaction.Create(tx, swapTx)
					if err != nil {
						return err
					}
					t.logger.Info("[IndexIcySwapTransaction][SwapProcessed]", map[string]string{
						"tx_hash":     swapTx.TransactionHash,
						"icy_amount":  swapTx.IcyAmount,
						"btc_address": swapTx.BtcAddress,
						"btc_amount":  swapTx.BtcAmount,
					})
					// Calculate service fee based on config
					subtotalBig := new(big.Int)
					subtotalBig.SetString(swapTx.BtcAmount, 10)

					// Calculate percentage-based fee
					feePercentage := t.appConfig.Bitcoin.ServiceFeePercentage
					percentageFee := new(big.Float).Mul(
						new(big.Float).SetInt(subtotalBig),
						new(big.Float).SetFloat64(feePercentage),
					)

					// Convert to int for comparison with min fee
					var percentageFeeInt big.Int
					percentageFee.Int(&percentageFeeInt)

					// Get minimum fee from config
					minFeeBig := big.NewInt(t.appConfig.Bitcoin.MinSatshiFee)

					// Use the larger of percentage fee or minimum fee
					var serviceFee big.Int
					if percentageFeeInt.Cmp(minFeeBig) < 0 {
						serviceFee = *minFeeBig
					} else {
						serviceFee = percentageFeeInt
					}

					// Calculate total (subtotal - service fee)
					totalBig := new(big.Int).Sub(subtotalBig, &serviceFee)

					_, err = t.store.OnchainBtcProcessedTransaction.Create(tx, &model.OnchainBtcProcessedTransaction{
						SwapTransactionHash: swapTx.TransactionHash,
						BTCAddress:          swapTx.BtcAddress,
						Subtotal:            swapTx.BtcAmount,
						ServiceFee:          serviceFee.String(),
						Total:               totalBig.String(),
						Status:              model.BtcProcessingStatusPending,
					})
					if err != nil {
						t.logger.Error("[IndexIcySwapTransaction][CreateBtcProcessedTx]", map[string]string{
							"error": err.Error(),
						})
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.logger.Error("[IndexIcySwapTransaction][CreateTransactions]", map[string]string{
					"error": err.Error(),
				})
				return err
			}
			totalProcessed += len(txsToStore)
		}

		// Break if we've reached the latest block
		if currentEnd == latestBlock {
			break
		}
	}

	t.logger.Info(fmt.Sprintf("[IndexIcySwapTransaction] Processed %d new transactions", totalProcessed))
	return nil
}

func (t *Telemetry) ProcessSwapRequests() error {
	// Fetch pending swap requests by querying for 'pending' status
	pendingSwapRequests, err := t.store.SwapRequest.FindPendingSwapRequests(t.db)
	if err != nil {
		t.logger.Error("[ProcessSwapRequests][FetchPendingRequests]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	for _, req := range pendingSwapRequests {
		// validate icy tx
		_, err := t.store.OnchainIcyTransaction.GetByTransactionHash(t.db, req.IcyTx)
		if err != nil {
			t.logger.Error("[ProcessSwapRequests][GetIcyTx]", map[string]string{
				"error":   err.Error(),
				"tx_hash": req.IcyTx,
			})
			continue
		}
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

		icyAmount := &model.Web3BigInt{
			Value:   req.ICYAmount,
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
		_, err = t.baseRpc.Swap(icyAmount, req.BTCAddress, satAmount)
		if err != nil {
			t.logger.Error("[ProcessSwapRequests][Swap]", map[string]string{
				"error":        err.Error(),
				"icy_amount":   req.ICYAmount,
				"btc_address":  req.BTCAddress,
				"latest_price": latestPrice.Value,
			})
			continue
		}
	}

	return nil
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
