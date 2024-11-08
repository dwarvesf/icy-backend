package btcrpc

import (
<<<<<<< HEAD
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
=======
	"encoding/json"
	"io"
	"net/http"
	"strconv"

>>>>>>> 716fb94 (feat: implement icy oracle)
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

var (
	// NetworkParams can be used to toggle between testnet and mainnet
	NetworkParams = &chaincfg.TestNet3Params
)

type BtcRpc struct {
	appConfig   *config.AppConfig
	logger      *logger.Logger
	blockstream blockstream.IBlockStream
}

func New(appConfig *config.AppConfig, logger *logger.Logger) IBtcRpc {
	return &BtcRpc{
		appConfig:   appConfig,
		logger:      logger,
		blockstream: blockstream.New(appConfig, logger),
	}
}

func (b *BtcRpc) Send(receiverAddressStr string, amount *model.Web3BigInt) error {
	// Get sender's priv key and address
	privKey, senderAddress, err := b.getSelfPrivKeyAndAddress(b.appConfig.Bitcoin.WalletWIF)
	if err != nil {
		b.logger.Error("[btcrpc.Send][getSelfPrivKeyAndAddress]", map[string]string{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to get self private key: %v", err)
	}

	// Get receiver's address
	receiverAddress, err := btcutil.DecodeAddress(receiverAddressStr, NetworkParams)
	if err != nil {
		b.logger.Error("[btcrpc.Send][DecodeAddress]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	amountToSend, ok := amount.Int64()
	if !ok {
		b.logger.Error("[btcrpc.Send][Int64]", map[string]string{
			"value": amount.Value,
		})
		return fmt.Errorf("failed to convert amount to int64")
	}

	// Select required UTXOs and calculate change amount
	selectedUTXOs, changeAmount, err := b.selectUTXOs(senderAddress.EncodeAddress(), amountToSend)
	if err != nil {
		b.logger.Error("[btcrpc.Send][selectUTXOs]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Create new tx and prepare inputs/outputs
	tx, err := b.prepareTx(selectedUTXOs, receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		b.logger.Error("[btcrpc.Send][prepareTx]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Sign tx
	err = b.sign(tx, privKey, senderAddress, selectedUTXOs)
	if err != nil {
		b.logger.Error("[btcrpc.Send][sign]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	// Serialize & broadcast tx
	err = b.broadcast(tx)
	if err != nil {
		b.logger.Error("[btcrpc.Send][broadcast]", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	return nil
}

func (b *BtcRpc) BalanceOf(address string) (*model.Web3BigInt, error) {
	url := b.baseURL + "/address/" + address

	resp, err := b.client.Get(url)
	if err != nil {
		b.logger.Error("[BtcRpc][BalanceOf]", map[string]string{
			"error": err.Error(),
		})
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b.logger.Error("[BtcRpc][BalanceOf]", map[string]string{
			"error": "unexpected status code",
		})

		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		b.logger.Error("[BtcRpc][BalanceOf]", map[string]string{
			"error": err.Error(),
		})

		return nil, err
	}

	var response *GetBalanceResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		b.logger.Error("[BtcRpc][BalanceOf]", map[string]string{
			"error": err.Error(),
		})

		return nil, err
	}

	return &model.Web3BigInt{
		Value:   strconv.Itoa(response.ChainStats.FundedTxoSum),
		Decimal: 10,
	}, nil
}
