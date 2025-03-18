package btcrpc

import (
	"fmt"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/patrickmn/go-cache"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

type BtcRpc struct {
	appConfig    *config.AppConfig
	logger       *logger.Logger
	blockstream  blockstream.IBlockStream
	cch          *cache.Cache
	networkParam *chaincfg.Params
}

func New(appConfig *config.AppConfig, logger *logger.Logger) IBtcRpc {
	networkParams := &chaincfg.TestNet3Params
	if appConfig.ApiServer.AppEnv == "prod" {
		networkParams = &chaincfg.MainNetParams
	}
	return &BtcRpc{
		appConfig:    appConfig,
		logger:       logger,
		blockstream:  blockstream.New(appConfig, logger),
		cch:          cache.New(1*time.Minute, 2*time.Minute),
		networkParam: networkParams,
	}
}

func (b *BtcRpc) Send(receiverAddressStr string, amount *model.Web3BigInt) (string, int64, error) {
	// Get sender's priv key and address
	privKey, senderAddress, err := b.getSelfPrivKeyAndAddress(b.appConfig.Bitcoin.WalletWIF)
	if err != nil {
		b.logger.Error("[btcrpc.Send][getSelfPrivKeyAndAddress]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, fmt.Errorf("failed to get self private key: %v", err)
	}

	// Get receiver's address
	receiverAddress, err := btcutil.DecodeAddress(receiverAddressStr, b.networkParam)
	if err != nil {
		b.logger.Error("[btcrpc.Send][DecodeAddress]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	amountToSend, ok := amount.Int64()
	if !ok {
		b.logger.Error("[btcrpc.Send][Int64]", map[string]string{
			"value": amount.Value,
		})
		return "", 0, fmt.Errorf("failed to convert amount to int64")
	}

	// Select required UTXOs and calculate change amount
	selectedUTXOs, changeAmount, fee, err := b.selectUTXOs(senderAddress.EncodeAddress(), amountToSend)
	if err != nil {
		b.logger.Error("[btcrpc.Send][selectUTXOs]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	// Create new tx and prepare inputs/outputs
	tx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		b.logger.Error("[btcrpc.Send][prepareTx]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	// Sign tx
	err = b.sign(tx, privKey, senderAddress, selectedUTXOs)
	if err != nil {
		b.logger.Error("[btcrpc.Send][sign]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	// Serialize & broadcast tx with potential fee adjustment
	txID, err := b.broadcastWithFeeAdjustment(tx, selectedUTXOs, receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		b.logger.Error("[btcrpc.Send][broadcast]", map[string]string{
			"error": err.Error(),
		})
		return "", 0, err
	}

	return txID, fee, nil
}

// broadcastWithFeeAdjustment attempts to broadcast the transaction,
// and if it fails due to minimum relay fee, attempts to increase the fee by 5%
func (b *BtcRpc) broadcastWithFeeAdjustment(
	tx *wire.MsgTx,
	selectedUTXOs []blockstream.UTXO,
	receiverAddress btcutil.Address,
	senderAddress *btcutil.AddressWitnessPubKeyHash,
	amountToSend, changeAmount int64,
) (string, error) {
	// First attempt to broadcast
	txID, err := b.broadcast(tx)
	if err == nil {
		return txID, nil
	}

	// Check if the error is specifically about minimum relay fee
	broadcastErr, ok := err.(*blockstream.BroadcastTxError)
	if ok {
		b.logger.Info("[btcrpc.Send][FeeAdjustment]", map[string]string{
			"message": "Attempting to adjust transaction fee",
		})

		// Use the minimum fee from the error if available
		var adjustedFee, currentFee int64
		if broadcastErr.MinFee > 0 {
			// Use the minimum fee from the error
			adjustedFee = broadcastErr.MinFee

			// Fallback to calculating current fee if no minimum fee in error
			feeRates, err := b.blockstream.EstimateFees()
			if err != nil {
				return "", fmt.Errorf("failed to get fee rates for adjustment: %v", err)
			}

			currentFee, err = b.calculateTxFee(feeRates, len(selectedUTXOs), 2, 6)
			if err != nil {
				return "", fmt.Errorf("failed to calculate current fee: %v", err)
			}

			if adjustedFee > int64(float64(currentFee)*1.05) {
				return "", fmt.Errorf("fee too high to adjust, adjusted fee: %d, current fee: %d", adjustedFee, currentFee)
			}
		} else {
			// Fallback to calculating fee if no minimum fee in error
			feeRates, err := b.blockstream.EstimateFees()
			if err != nil {
				return "", fmt.Errorf("failed to get fee rates for adjustment: %v", err)
			}

			currentFee, err = b.calculateTxFee(feeRates, len(selectedUTXOs), 2, 6)
			if err != nil {
				return "", fmt.Errorf("failed to calculate current fee: %v", err)
			}

			// Adjust fee to be 5% higher
			adjustedFee = int64(float64(currentFee) * 1.05)
		}

		b.logger.Info("[btcrpc.Send][FeeAdjustment]", map[string]string{
			"currentFee":   strconv.FormatInt(currentFee, 10),
			"adjustedFee":  strconv.FormatInt(adjustedFee, 10),
			"changeAmount": strconv.FormatInt(changeAmount, 10),
			"amountToSend": strconv.FormatInt(amountToSend, 10),
		})

		// Calculate adjusted change amount
		adjustedChangeAmount := changeAmount - (adjustedFee - currentFee)

		// If adjusted change amount becomes negative, we can't proceed
		if adjustedChangeAmount < 0 {
			return "", fmt.Errorf("insufficient funds to adjust transaction fee")
		}

		// Recreate transaction with adjusted fee
		adjustedTx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, adjustedChangeAmount)
		if err != nil {
			return "", fmt.Errorf("failed to prepare adjusted transaction: %v", err)
		}

		// Re-sign the transaction
		privKey, _, err := b.getSelfPrivKeyAndAddress(b.appConfig.Bitcoin.WalletWIF)
		if err != nil {
			return "", fmt.Errorf("failed to get private key for re-signing: %v", err)
		}

		err = b.sign(adjustedTx, privKey, senderAddress, selectedUTXOs)
		if err != nil {
			return "", fmt.Errorf("failed to sign adjusted transaction: %v", err)
		}

		// Attempt to broadcast adjusted transaction
		return b.broadcast(adjustedTx)
	}

	// If it's a different error, return the original error
	return "", err
}

func (b *BtcRpc) CurrentBalance() (*model.Web3BigInt, error) {
	balance, err := b.blockstream.GetBTCBalance(b.appConfig.Blockchain.BTCTreasuryAddress)
	if err != nil {
		b.logger.Error("[CurrentBalance][GetBTCBalance]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	return balance, nil
}

func (b *BtcRpc) GetTransactionsByAddress(address string, fromTxId string) ([]model.OnchainBtcTransaction, error) {
	rawTx, err := b.blockstream.GetTransactionsByAddress(address, fromTxId)
	if err != nil {
		b.logger.Error("[GetTransactionsByAddress][GetTransactionsByAddress]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}

	// Filter out unconfirmed transactions
	confirmedTx := make([]blockstream.Transaction, 0)
	for _, tx := range rawTx {
		if tx.TxID == fromTxId {
			break
		}
		if tx.Status.Confirmed {
			confirmedTx = append(confirmedTx, tx)
		}
	}

	transactions := make([]model.OnchainBtcTransaction, 0)
	for _, tx := range confirmedTx {
		var isOutgoing bool
		var senderAddress string
		for _, input := range tx.Vin {
			prevOut := input.Prevout
			if prevOut != nil {
				if prevOut.ScriptPubKeyAddress == address {
					isOutgoing = true
				} else {
					senderAddress = prevOut.ScriptPubKeyAddress
				}
			}
		}

		if isOutgoing {
			for _, output := range tx.Vout {
				if output.ScriptPubKeyAddress != address {
					transactions = append(transactions, model.OnchainBtcTransaction{
						TransactionHash: tx.TxID,
						Amount:          strconv.FormatInt(output.Value, 10),
						Type:            model.Out,
						OtherAddress:    output.ScriptPubKeyAddress,
						BlockTime:       tx.Status.BlockTime,
						InternalID:      tx.TxID,
						Fee:             strconv.FormatInt(tx.Fee, 10),
					})
				}
			}
		} else {
			for _, output := range tx.Vout {
				if output.ScriptPubKeyAddress == address {
					transactions = append(transactions, model.OnchainBtcTransaction{
						TransactionHash: tx.TxID,
						Amount:          strconv.FormatInt(output.Value, 10),
						Type:            model.In,
						OtherAddress:    senderAddress,
						BlockTime:       tx.Status.BlockTime,
						InternalID:      tx.TxID,
					})
				}
			}
		}
	}
	return transactions, nil
}

// EstimateFees retrieves current Bitcoin transaction fee estimates
func (b *BtcRpc) EstimateFees() (map[string]float64, error) {
	fees, err := b.blockstream.EstimateFees()
	if err != nil {
		b.logger.Error("[EstimateFees][blockstream.EstimateFees]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}
	return fees, nil
}
