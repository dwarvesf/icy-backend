package btcrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/patrickmn/go-cache"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
)

const (
	p2wpkhInputSize  = 68 // SegWit P2WPKH input size
	p2wpkhOutputSize = 31 // SegWit P2WPKH output size
	txOverhead       = 10 // Transaction overhead
)

// calculateTxFee estimates the transaction fee based on current network conditions
func (b *BtcRpc) calculateTxFee(feeRates map[string]float64, numInputs, numOutputs, targetBlocks int) (int64, error) {
	// Get fee rate for target blocks
	target := fmt.Sprintf("%d", targetBlocks)
	feeRate, ok := feeRates[target]
	if !ok {
		return 0, fmt.Errorf("no fee rate available for target %d blocks", targetBlocks)
	}

	// Calculate total transaction size
	txSize := calculateTxSize(numInputs, numOutputs)

	// Calculate fee in satoshis
	fee := int64(float64(txSize) * feeRate)
	return fee, nil
}

// calculateTxSize calculates the total transaction size in bytes
func calculateTxSize(numInputs, numOutputs int) int {
	return txOverhead + (numInputs * p2wpkhInputSize) + (numOutputs * p2wpkhOutputSize)
}

func (b *BtcRpc) getSelfPrivKeyAndAddress(wifStr string) (*secp256k1.PrivateKey, *btcutil.AddressWitnessPubKeyHash, error) {
	// Decode private key from WIF format
	wif, err := btcutil.DecodeWIF(wifStr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode wif: %v", err)
	}

	privKey := wif.PrivKey
	pubKey := privKey.PubKey()
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())

	// Create new SegWit address
	address, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, NetworkParams)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create sender address: %v", err)
	}

	return privKey, address, nil
}

// getSelfAddress decodes WIF private key and return address from derived public key hash
func (b *BtcRpc) getSelfAddress(pubKey *secp256k1.PublicKey) (addr *btcutil.AddressWitnessPubKeyHash, err error) {
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err = btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, NetworkParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create sender address: %v", err)
	}

	return
}

// prepareTxInputs creates and returns transaction inputs from UTXOs
func (b *BtcRpc) prepareTxInputs(utxos []blockstream.UTXO) ([]*wire.TxIn, error) {
	var inputs []*wire.TxIn

	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, fmt.Errorf("failed to create hash: %v", err)
		}
		input := wire.NewTxIn(wire.NewOutPoint(hash, uint32(utxo.Vout)), nil, nil)
		inputs = append(inputs, input)
	}

	return inputs, nil
}

// prepareTxOutputs creates both recipient and change outputs
func (b *BtcRpc) prepareTxOutputs(
	receiverAddress btcutil.Address,
	senderAddress *btcutil.AddressWitnessPubKeyHash,
	amountToSend int64,
	changeAmount int64,
) ([]*wire.TxOut, error) {
	// Prepare recipient output
	pkScript, err := txscript.PayToAddrScript(receiverAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create recipient output script: %v", err)
	}
	recipientOutput := wire.NewTxOut(amountToSend, pkScript)

	// Prepare change output
	changeAddress, err := btcutil.DecodeAddress(senderAddress.EncodeAddress(), NetworkParams)
	if err != nil {
		return nil, fmt.Errorf("failed to decode change address: %v", err)
	}
	changePkScript, err := txscript.PayToAddrScript(changeAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create change output script: %v", err)
	}
	changeOutput := wire.NewTxOut(changeAmount, changePkScript)

	return []*wire.TxOut{recipientOutput, changeOutput}, nil
}

// prepareTx prepares both inputs and outputs for a transaction
func (b *BtcRpc) prepareTx(
	utxos []blockstream.UTXO,
	receiverAddress btcutil.Address,
	senderAddress *btcutil.AddressWitnessPubKeyHash,
	amountToSend int64,
	changeAmount int64,
) (*wire.MsgTx, error) {
	// Create new transaction
	tx := wire.NewMsgTx(2)

	// Prepare inputs
	inputs, err := b.prepareTxInputs(utxos)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare inputs: %v", err)
	}

	// Add inputs to transaction
	for _, input := range inputs {
		tx.AddTxIn(input)
	}

	// Prepare outputs
	outputs, err := b.prepareTxOutputs(receiverAddress, senderAddress, amountToSend, changeAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare outputs: %v", err)
	}

	// Add outputs to transaction
	for _, output := range outputs {
		tx.AddTxOut(output)
	}

	return tx, nil
}

// sign signs the transaction with the private key for each input
func (b *BtcRpc) sign(
	tx *wire.MsgTx,
	privKey *secp256k1.PrivateKey,
	senderAddress *btcutil.AddressWitnessPubKeyHash,
	selectedUTXOs []blockstream.UTXO,
) error {
	prevOutScript, err := txscript.PayToAddrScript(senderAddress)
	if err != nil {
		return fmt.Errorf("failed to create sender output script: %v", err)
	}

	// Sign each input with corresponding UTXO amount
	for i, utxo := range selectedUTXOs {
		prevOuts := txscript.NewCannedPrevOutputFetcher(prevOutScript, utxo.Value)
		witness, err := txscript.WitnessSignature(
			tx,
			txscript.NewTxSigHashes(tx, prevOuts),
			i,
			utxo.Value,
			prevOutScript,
			txscript.SigHashAll,
			privKey,
			true,
		)
		if err != nil {
			return fmt.Errorf("failed to sign transaction input %d: %v", i, err)
		}
		tx.TxIn[i].Witness = witness
		tx.TxIn[i].SignatureScript = nil
	}

	return nil
}

// broadcast serializes the signed transaction and broadcasts it
func (b *BtcRpc) broadcast(tx *wire.MsgTx) (string, error) {
	var signedTx bytes.Buffer
	tx.Serialize(&signedTx)
	txHex := hex.EncodeToString(signedTx.Bytes())

	txID, err := b.blockstream.BroadcastTx(txHex)
	if err != nil {
		return "", err
	}

	return txID, nil
}

// verifyAndSelectUTXOs checks if there are sufficient funds across all UTXOs
// and returns selected UTXOs that cover the required amount
func (b *BtcRpc) verifyAndSelectUTXOs(address string, amountToSend, txFee int64) ([]blockstream.UTXO, bool) {
	utxos, err := b.blockstream.GetUTXOs(address)
	if err != nil {
		return nil, false
	}

	// Filter confirmed UTXOs and sort by value in descending order
	var confirmedUTXOs []blockstream.UTXO
	for _, utxo := range utxos {
		if utxo.Status.Confirmed {
			confirmedUTXOs = append(confirmedUTXOs, utxo)
		}
	}
	sort.Slice(confirmedUTXOs, func(i, j int) bool {
		return confirmedUTXOs[i].Value > confirmedUTXOs[j].Value
	})

	// Calculate total required amount
	requiredAmount := amountToSend + txFee

	// Select UTXOs and calculate total amount
	var totalSelected int64
	var selectedUTXOs []blockstream.UTXO
	for _, utxo := range confirmedUTXOs {
		selectedUTXOs = append(selectedUTXOs, utxo)
		totalSelected += utxo.Value
		if totalSelected >= requiredAmount {
			return selectedUTXOs, true
		}
	}

	return nil, false
}

func (b *BtcRpc) getConfirmedUTXOs(address string) ([]blockstream.UTXO, error) {
	utxos, err := b.blockstream.GetUTXOs(address)
	if err != nil {
		return nil, err
	}

	// Filter confirmed UTXOs and sort by value in descending order
	var confirmedUTXOs []blockstream.UTXO
	for _, utxo := range utxos {
		if utxo.Status.Confirmed {
			confirmedUTXOs = append(confirmedUTXOs, utxo)
		}
	}
	sort.Slice(confirmedUTXOs, func(i, j int) bool {
		return confirmedUTXOs[i].Value > confirmedUTXOs[j].Value
	})

	return confirmedUTXOs, nil
}

// selectUTXOs picks UTXOs until we have enough to cover amountToSend + fee
// returns selected UTXOs and change amount
// change amount is the amount sent back to sender after sending total amount of selected UTXOs to recipient
// changeAmount = total amount of selected UTXOs - amountToSend - fee
func (b *BtcRpc) selectUTXOs(address string, amountToSend int64) (selected []blockstream.UTXO, changeAmount int64, err error) {
	confirmedUTXOs, err := b.getConfirmedUTXOs(address)
	if err != nil {
		return nil, 0, err
	}

	// Get current fee rate from mempool
	feeRates, err := b.blockstream.EstimateFees()
	if err != nil {
		return nil, 0, err
	}

	// Iteratively select UTXOs until we have enough to cover amount + fee
	var totalSelected int64
	var fee int64

	for _, utxo := range confirmedUTXOs {
		selected = append(selected, utxo)
		totalSelected += utxo.Value

		// calculate tx fee based on the size of the transaction
		// n inputs: number of UTXOs whose total amount can cover the required amount (amountToSend + fee)
		// 2 outputs: 1 output tx for sending `amountToSend` to recipient, 1 output tx for sending `changeAmount` back to sender
		// targetBlocks confirmations: widely accepted standard for bitcoin transactions
		fee, err = b.calculateTxFee(feeRates, len(selected), 2, 6)
		if err != nil {
			return nil, 0, err
		}

		if fee > amountToSend {
			return nil, 0, fmt.Errorf("fee exceeds amount to send: fee %d, amountToSend %d", fee, amountToSend)
		}

		satoshiRate, err := b.getSatoshiUSDPrice()
		if err != nil {
			return nil, 0, err
		}

		// calculate and round up to 1 decimal places
		usdFee := math.Ceil(float64(fee)/satoshiRate*10) / 10

		if usdFee > b.appConfig.Bitcoin.MaxTxFeeUSD {
			return nil, 0, fmt.Errorf("fee exceeds maximum threshold: usdFee %0.1f, MaxTxFeeUSD %0.1f", usdFee, b.appConfig.Bitcoin.MaxTxFeeUSD)
		}

		// if we have enough to cover amount + current fee => return selected UTXOs and change amount
		if totalSelected >= amountToSend+fee {
			b.logger.Info("[selectUTXOs] calculateTxFee", map[string]string{
				"amountToSend": fmt.Sprintf("%d", amountToSend),
				"fee":          fmt.Sprintf("%d", fee),
				"usdFee":       fmt.Sprintf("%0.1f", usdFee),
			})
			changeAmount = totalSelected - amountToSend - fee
			return selected, changeAmount, nil
		}
	}

	return nil, 0, fmt.Errorf(
		"insufficient funds: have %d satoshis, need %d satoshis",
		totalSelected,
		amountToSend+fee,
	)
}

type CoinGeckoResponse struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
	} `json:"bitcoin"`
}

func (b *BtcRpc) getSatoshiUSDPrice() (float64, error) {
	// call from cache
	if x, found := b.cch.Get("satoshiPerUSD"); found {
		b.logger.Info("[getSatoshiUSDPrice] cache hit", map[string]string{
			"satoshiPerUSD": fmt.Sprintf("%0.1f", x),
		})
		rate := x.(float64)
		return rate, nil
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var data CoinGeckoResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}

	bitcoinPrice := data.Bitcoin.USD
	if bitcoinPrice <= 0 {
		return 0, fmt.Errorf("invalid bitcoin price: %f", bitcoinPrice)
	}

	// Calculate Satoshi/USD (1 BTC = 100,000,000 Satoshi)
	satoshiPerUSD := 100_000_000 / bitcoinPrice

	// cache the rate
	b.cch.Set("satoshiPerUSD", satoshiPerUSD, cache.DefaultExpiration)

	return satoshiPerUSD, nil
}
