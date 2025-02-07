package telemetry

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k0kubun/pp"
	"gorm.io/gorm"

	"github.com/dwarvesf/icy-backend/internal/baserpc"
	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type Telemetry struct {
	db        *gorm.DB
	store     *store.Store
	appConfig *config.AppConfig
	logger    *logger.Logger
	btcRpc    btcrpc.IBtcRpc
	baseRpc   baserpc.IBaseRPC
}

func New(db *gorm.DB, store *store.Store, appConfig *config.AppConfig, logger *logger.Logger, btcRpc btcrpc.IBtcRpc, baseRpc baserpc.IBaseRPC) *Telemetry {
	return &Telemetry{
		db:        db,
		store:     store,
		appConfig: appConfig,
		logger:    logger,
		btcRpc:    btcRpc,
		baseRpc:   baseRpc,
	}
}

func (t *Telemetry) IndexBtcTransaction() error {
	t.logger.Info("[IndexBtcTransaction] Start indexing BTC transactions...")

	var latestTx *model.OnchainBtcTransaction
	latestTx, err := t.store.OnchainBtcTransaction.GetLatestTransaction(t.db)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.logger.Error("[IndexBtcTransaction][GetLatestTransaction]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
	}

	//TODO: Should add first transaction to db manually.
	txHash := ""
	if latestTx != nil {
		txHash = latestTx.TransactionHash
	}

	t.logger.Info(fmt.Sprintf("[IndexBtcTransaction] Latest BTC transaction: %s", txHash))

	markedTxHash := ""
	txs := []model.OnchainBtcTransaction{}
	for {
		markedTxs, err := t.btcRpc.GetTransactionsByAddress(t.appConfig.Blockchain.BTCTreasuryAddress, markedTxHash)
		if err != nil {
			t.logger.Error("[IndexBtcTransaction][GetTransactionsByAddress]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
		for i, tx := range markedTxs {
			if tx.TransactionHash == txHash {
				markedTxs = markedTxs[:i]
				break
			}
		}
		txs = append(txs, markedTxs...)
		if len(markedTxs) < 25 {
			break
		}
		markedTxHash = markedTxs[len(markedTxs)-1].TransactionHash
	}

	slices.Reverse(txs)

	return store.DoInTx(t.db, func(tx *gorm.DB) error {
		for _, onchainTx := range txs {
			_, err := t.store.OnchainBtcTransaction.Create(tx, &onchainTx)
			if err != nil {
				t.logger.Error("[IndexBtcTransaction][Create]", map[string]string{
					"error": err.Error(),
				})
				return err
			}
			t.logger.Info(fmt.Sprintf("Tx Hash: %s - Amount: %s [%s]", onchainTx.TransactionHash, onchainTx.Amount, onchainTx.Type))
		}
		return nil
	})
}

func (t *Telemetry) IndexIcyTransaction() error {
	t.logger.Info("[IndexIcyTransaction] Start indexing ICY transactions...")

	var latestTx *model.OnchainIcyTransaction
	latestTx, err := t.store.OnchainIcyTransaction.GetLatestTransaction(t.db)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		t.logger.Error("[IndexIcyTransaction][GetLatestTransaction]", map[string]string{
			"error": err.Error(),
		})
		return err
	}
	pp.Println(latestTx)

	// Define a maximum block range to prevent exceeding limits
	const maxBlockRange = 10000 // Adjust this value as needed

	// If no previous transaction, start from the contract creation block
	startBlock := uint64(0)
	if latestTx != nil {
		// Get the block number of the last transaction
		receipt, err := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(latestTx.TransactionHash))
		if err != nil {
			t.logger.Error("[IndexIcyTransaction][GetTransactionReceipt]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
		startBlock = receipt.BlockNumber.Uint64() + 1
	}

	// Get the current latest block
	latestBlock, err := t.baseRpc.Client().BlockNumber(context.Background())
	if err != nil {
		t.logger.Error("[IndexIcyTransaction][GetLatestBlock]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Process transactions in batches
	for currentStart := startBlock; currentStart <= latestBlock; currentStart += maxBlockRange {
		currentEnd := currentStart + maxBlockRange
		if currentEnd > latestBlock {
			currentEnd = latestBlock
		}

		markedTxs, err := t.baseRpc.GetTransactionsByAddress(t.appConfig.Blockchain.ICYContractAddr, "")
		if err != nil {
			t.logger.Error("[IndexIcyTransaction][GetTransactionsByAddress]", map[string]string{
				"error": err.Error(),
			})
			return err
		}

		// Filter transactions within the current block range
		var txs []model.OnchainIcyTransaction
		for _, tx := range markedTxs {
			receipt, err := t.baseRpc.Client().TransactionReceipt(context.Background(), common.HexToHash(tx.TransactionHash))
			if err != nil {
				continue
			}
			blockNum := receipt.BlockNumber.Uint64()
			if blockNum >= currentStart && blockNum <= currentEnd {
				txs = append(txs, tx)
			}
		}

		// Store transactions in batches
		if len(txs) > 0 {
			err = store.DoInTx(t.db, func(tx *gorm.DB) error {
				for _, onchainTx := range txs {
					_, err := t.store.OnchainIcyTransaction.Create(tx, &onchainTx)
					if err != nil {
						return err
					}
					t.logger.Info(fmt.Sprintf("Tx Hash: %s - Amount: %s [%s]", onchainTx.TransactionHash, onchainTx.Amount, onchainTx.Type))
				}
				return nil
			})
			if err != nil {
				t.logger.Error("[IndexIcyTransaction][CreateTransactions]", map[string]string{
					"error": err.Error(),
				})
				return err
			}
		}
	}

	return nil
}

func (t *Telemetry) GetIcyTransactionByHash(txHash string) (*model.OnchainIcyTransaction, error) {
	return t.store.OnchainIcyTransaction.GetByTransactionHash(t.db, txHash)
}
