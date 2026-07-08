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

		// Only log progress every 100K blocks to reduce noise
		if currentStart%100000 == 0 {
			t.logger.Info("[IndexIcySwapTransaction] progress", map[string]string{
				"startBlock":  fmt.Sprintf("%d", currentStart),
				"endBlock":    fmt.Sprintf("%d", currentEnd),
				"latestBlock": fmt.Sprintf("%d", latestBlock),
				"remaining":   fmt.Sprintf("%d", latestBlock-currentStart),
			})
		}

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
					// Idempotent re-index / reorg guard. The block cursor resets to
					// InitialICYSwapBlockNumber whenever the latest-tx receipt fetch
					// fails, which re-filters every past Swap event. Without this skip
					// the batch would insert duplicate swap rows (inflating the ICY
					// backing sum) and then abort on the payout unique index. Skipping
					// an already-recorded swap keeps re-indexing idempotent.
					if _, gerr := t.store.OnchainIcySwapTransaction.GetByTransactionHash(tx, swapTx.TransactionHash); gerr == nil {
						t.logger.Info("[IndexIcySwapTransaction][DedupSwap] already indexed, skipping", map[string]string{
							"tx_hash": swapTx.TransactionHash,
						})
						continue
					} else if !errors.Is(gerr, gorm.ErrRecordNotFound) {
						return gerr
					}

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

					if err := t.CreateBtcPayoutForSwap(tx, swapTx); err != nil {
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

// ErrIcyDepositMismatch marks a swap whose on-chain ICY deposit does NOT cover
// the amount the payout would be based on (short, absent, or unparseable). It is
// a business rejection, not an infrastructure failure: the caller drops the
// payout for this swap but does NOT abort the batch.
var ErrIcyDepositMismatch = errors.New("icy deposit does not cover the swap amount")

// CreateBtcPayoutForSwap creates the single BTC payout row for one indexed swap
// event, behind two money-gates that must BOTH pass, otherwise no payout row is
// created and therefore no BTC can ever leave for it:
//
//  1. Dedup. If a payout already exists for this swap tx hash (the populated,
//     DB-unique key), skip. This makes a re-indexed / reorg-re-emitted swap
//     never mint a second payout. Returns nil (idempotent, not an error).
//  2. ICY-deposit verification. The swap tx must actually have transferred at
//     least IcyAmount of ICY into the treasury (the swap contract). A short /
//     absent / unparseable deposit is rejected (no payout). A transient RPC
//     error while verifying is returned so the enclosing DoInTx rolls back and
//     retries later, never paying an unverified deposit (fail-closed).
//
// Exported so the money-gates are testable end-to-end from a package whose test
// binary builds (internal/server), mirroring how SG-05 tested settlement.
func (t *Telemetry) CreateBtcPayoutForSwap(tx *gorm.DB, swapTx *model.OnchainIcySwapTransaction) error {
	// Gate 1: dedup on the populated swap_transaction_hash key.
	if _, err := t.store.OnchainBtcProcessedTransaction.GetBySwapTransactionHash(tx, swapTx.TransactionHash); err == nil {
		t.logger.Info("[IndexIcySwapTransaction][DedupPayout] payout already exists, skipping", map[string]string{
			"swap_tx_hash": swapTx.TransactionHash,
		})
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Gate 2: verify the ICY deposit actually landed in the treasury.
	if err := t.verifyIcyDeposit(swapTx); err != nil {
		if errors.Is(err, ErrIcyDepositMismatch) {
			// Real short/absent deposit: reject this payout permanently. No BTC.
			t.logger.Error("[IndexIcySwapTransaction][VerifyIcyDeposit] REJECTED, no BTC payout", map[string]string{
				"swap_tx_hash": swapTx.TransactionHash,
				"icy_amount":   swapTx.IcyAmount,
				"error":        err.Error(),
			})
			return nil
		}
		// Infrastructure error (could not verify): abort the batch and retry later.
		t.logger.Error("[IndexIcySwapTransaction][VerifyIcyDeposit] verify failed, aborting batch", map[string]string{
			"swap_tx_hash": swapTx.TransactionHash,
			"error":        err.Error(),
		})
		return err
	}

	// Calculate service fee based on config
	subtotalBig := new(big.Int)
	subtotalBig.SetString(swapTx.BtcAmount, 10)

	// Calculate percentage-based fee
	svcfeeRate := t.appConfig.Bitcoin.ServiceFeeRate
	svcFee := new(big.Float).Mul(
		new(big.Float).SetInt(subtotalBig),
		new(big.Float).SetFloat64(svcfeeRate),
	)

	// Convert to int for comparison with min fee
	var svcFeeInt big.Int
	svcFee.Int(&svcFeeInt)

	// Get minimum fee from config
	minFeeBig := big.NewInt(t.appConfig.Bitcoin.MinSatshiFee)

	// Use the larger of percentage fee or minimum fee
	var serviceFee big.Int
	if svcFeeInt.Cmp(minFeeBig) < 0 {
		serviceFee = *minFeeBig
	} else {
		serviceFee = svcFeeInt
	}

	// Calculate total (subtotal - service fee)
	totalBig := new(big.Int).Sub(subtotalBig, &serviceFee)

	// Populate IcyTransactionHash too (previously always NULL): it revives the
	// GetByIcyTransactionHash lookup and records which ICY tx the payout settles.
	icyTxHash := swapTx.TransactionHash
	_, err := t.store.OnchainBtcProcessedTransaction.Create(tx, &model.OnchainBtcProcessedTransaction{
		IcyTransactionHash:  &icyTxHash,
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
	return nil
}

// verifyIcyDeposit confirms the swap tx moved at least IcyAmount of ICY into the
// treasury (the ICY swap contract, baseRpc.GetContractAddress()). The treasury
// address and ICY token address are read from config via baseRpc, never
// hardcoded. A deposit below the required amount, or an unparseable/zero
// required amount, returns ErrIcyDepositMismatch (business rejection); an RPC
// failure returns a wrapped infra error.
func (t *Telemetry) verifyIcyDeposit(swapTx *model.OnchainIcySwapTransaction) error {
	required, ok := new(big.Int).SetString(swapTx.IcyAmount, 10)
	if !ok || required.Sign() <= 0 {
		return fmt.Errorf("%w: unparseable or non-positive icy amount %q", ErrIcyDepositMismatch, swapTx.IcyAmount)
	}

	treasury := t.baseRpc.GetContractAddress()
	deposited, err := t.baseRpc.ICYTransferredTo(swapTx.TransactionHash, treasury)
	if err != nil {
		return fmt.Errorf("verify icy deposit for %s: %w", swapTx.TransactionHash, err)
	}
	if deposited == nil || deposited.Cmp(required) < 0 {
		return fmt.Errorf("%w: deposited %v < required %s to treasury %s",
			ErrIcyDepositMismatch, deposited, required.String(), treasury.Hex())
	}
	return nil
}
