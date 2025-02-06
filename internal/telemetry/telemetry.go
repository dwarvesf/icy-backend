package telemetry

import (
	"errors"
	"fmt"
	"slices"

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
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.logger.Error("[IndexIcyTransaction][GetLatestTransaction]", map[string]string{
				"error": err.Error(),
			})
			return err
		}
	}

	txHash := ""
	if latestTx != nil {
		txHash = latestTx.TransactionHash
	}

	t.logger.Info(fmt.Sprintf("[IndexIcyTransaction] Latest ICY transaction: %s", txHash))

	markedTxHash := ""
	txs := []model.OnchainIcyTransaction{}
	for {
		markedTxs, err := t.baseRpc.GetTransactionsByAddress(t.appConfig.Blockchain.ICYContractAddr, markedTxHash)
		if err != nil {
			t.logger.Error("[IndexIcyTransaction][GetTransactionsByAddress]", map[string]string{
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
			_, err := t.store.OnchainIcyTransaction.Create(tx, &onchainTx)
			if err != nil {
				t.logger.Error("[IndexIcyTransaction][Create]", map[string]string{
					"error": err.Error(),
				})
				return err
			}
			t.logger.Info(fmt.Sprintf("Tx Hash: %s - Amount: %s [%s]", onchainTx.TransactionHash, onchainTx.Amount, onchainTx.Type))
		}
		return nil
	})
}

// fix this AI!
func (t *Telemetry) GetIcyTransactionByHash(txHash string) (*model.OnchainIcyTransaction, error) {
	return t.store.OnchainIcyTransaction.GetByTransactionHash(t.db, txHash)
}
