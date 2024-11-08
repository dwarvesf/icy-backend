package btcrpc

import (
	"bytes"
	"encoding/hex"
	"math/big"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
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
	// Decode private key from WIF format
	wif, err := btcutil.DecodeWIF(b.appConfig.Bitcoin.WalletWIF)
	if err != nil {
		b.logger.Fatal("failed to decode wif", map[string]string{
			"error": err.Error(),
		})
	}
	privKey := wif.PrivKey
	pubKey := privKey.PubKey()
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	senderAddress, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, &chaincfg.TestNet3Params)
	if err != nil {
		b.logger.Fatal("failed to create sender address", map[string]string{
			"error": err.Error(),
		})
	}

	// Get latest UTXO for sender address
	latestUTXO, err := b.GetLatestUTXO(senderAddress.EncodeAddress())
	if err != nil {
		b.logger.Fatal("failed to get latest UTXO", map[string]string{
			"error": err.Error(),
		})
	}

	prevTxHash := latestUTXO.TxID
	prevTxIndex := latestUTXO.Vout
	prevTxAmount := latestUTXO.Value

	// Recipient address
	receiverAddress, err := btcutil.DecodeAddress(receiverAddressStr, &chaincfg.TestNet3Params)
	if err != nil {
		b.logger.Fatal("failed to create recipient address", map[string]string{
			"error":    err.Error(),
			"receiver": receiverAddressStr,
		})
	}

	// Transaction amounts
	amt, ok := new(big.Int).SetString(amount.Value, 10)
	if !ok {
		b.logger.Fatal("failed to convert amount to big int", map[string]string{
			"error":  err.Error(),
			"amount": amount.Value,
		})
	}
	amountToSend := amt.Int64()
	txFee := int64(1000)

	// After verifying UTXO
	// Calculate change amount using the actual UTXO amount
	changeAmount := prevTxAmount - amountToSend - txFee
	if changeAmount < 0 {
		b.logger.Fatal("insufficient funds in UTXO")
	}

	// Verify the amounts add up
	if amountToSend+txFee+changeAmount != prevTxAmount {
		b.logger.Fatal("amount mismatch! transaction amounts don't add up to UTXO amount", map[string]string{
			"error": err.Error(),
		})
	}

	// Create a new Bitcoin transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	hash, err := chainhash.NewHashFromStr(prevTxHash)
	if err != nil {
		b.logger.Fatal("failed to create hash", map[string]string{
			"error": err.Error(),
		})
	}

	// Create the transaction input
	txIn := wire.NewTxIn(wire.NewOutPoint(hash, uint32(prevTxIndex)), nil, nil)
	tx.AddTxIn(txIn)

	// Add recipient as output
	pkScript, err := txscript.PayToAddrScript(receiverAddress)
	if err != nil {
		b.logger.Fatal("failed to create recipient output script", map[string]string{
			"error":    err.Error(),
			"receiver": receiverAddressStr,
		})
	}
	txOut := wire.NewTxOut(amountToSend, pkScript)
	tx.AddTxOut(txOut)

	// Add change output (optional, if needed)
	// Here, replace the changeAddressStr with your change address
	changeAddressStr := senderAddress.EncodeAddress()
	changeAddress, err := btcutil.DecodeAddress(changeAddressStr, &chaincfg.TestNet3Params)
	if err != nil {
		b.logger.Fatal("failed to decode change address", map[string]string{
			"error":  err.Error(),
			"sender": changeAddressStr,
		})
	}
	changePkScript, err := txscript.PayToAddrScript(changeAddress)
	if err != nil {
		b.logger.Fatal("failed to create change output script", map[string]string{
			"error": err.Error(),
		})
	}

	// Get the previous output's public key script
	prevOutScript, err := txscript.PayToAddrScript(senderAddress)
	if err != nil {
		b.logger.Fatal("failed to create sender output script", map[string]string{
			"error": err.Error(),
		})
	}

	prevOuts := txscript.NewCannedPrevOutputFetcher(prevOutScript, prevTxAmount)

	tx.AddTxOut(wire.NewTxOut(changeAmount, changePkScript))

	// Sign the transaction
	for i := range tx.TxIn {
		witness, err := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, prevOuts),
			i, prevTxAmount, prevOutScript, txscript.SigHashAll, privKey, true)
		if err != nil {
			b.logger.Fatal("failed to sign transaction", map[string]string{
				"error": err.Error(),
			})
		}
		tx.TxIn[i].Witness = witness
		// Clear any existing signature script
		tx.TxIn[i].SignatureScript = nil
	}

	// Serialize the transaction
	var signedTx bytes.Buffer
	tx.Serialize(&signedTx)

	// Convert to hex and print the signed transaction
	txHex := hex.EncodeToString(signedTx.Bytes())

	// Broadcast using Blockstream testnet API
	if _, err := b.blockstream.BroadcastTx(txHex); err != nil {
		b.logger.Fatal("failed to broadcast transaction", map[string]string{
			"error": err.Error(),
		})
	}

	return nil
}

func (b *BtcRpc) BalanceOf(address string) (*model.Web3BigInt, error) {
	return nil, nil
}
