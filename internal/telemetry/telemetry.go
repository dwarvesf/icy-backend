package telemetry

import (
	"errors"
	"fmt"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/store"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
	"gorm.io/gorm"
)

type Telemetry struct {
	db        *gorm.DB
	store     *store.Store
	appConfig *config.AppConfig
	logger    *logger.Logger
	btcRpc    btcrpc.IBtcRpc
}

func New(db *gorm.DB, store *store.Store, appConfig *config.AppConfig, logger *logger.Logger, btcRpc btcrpc.IBtcRpc) *Telemetry {
	return &Telemetry{
		db:        db,
		store:     store,
		appConfig: appConfig,
		logger:    logger,
		btcRpc:    btcRpc,
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

	txHash := ""
	if latestTx != nil {
		txHash = latestTx.TransactionHash
	}

	t.logger.Info(fmt.Sprintf("[IndexBtcTransaction] Latest BTC transaction: %s", txHash))

	txs, err := t.btcRpc.GetTransactionsByAddress(t.appConfig.Blockchain.BTCTreasuryAddress, txHash)
	if err != nil {
		t.logger.Error("[IndexBtcTransaction][GetTransactionsByAddress]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

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
