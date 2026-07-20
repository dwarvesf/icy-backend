package btcrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"sort"
	"strings"
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

// dustChangeThreshold is the change-output floor, in satoshi. A change output at
// or below this is uneconomical (its own future spend costs more than it holds)
// and standard relay rules reject it as dust, which would make the whole payout
// unbroadcastable. 546 is the conservative P2PKH dust bound (the highest across
// address types), so using it here never leaves a truly-spendable change output
// behind. Change below it is folded into the miner fee (by omitting the output)
// rather than created.
const dustChangeThreshold = 546

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
	address, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, b.networkParam)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create sender address: %v", err)
	}

	return privKey, address, nil
}

// getSelfAddress decodes WIF private key and return address from derived public key hash
func (b *BtcRpc) getSelfAddress(pubKey *secp256k1.PublicKey) (addr *btcutil.AddressWitnessPubKeyHash, err error) {
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err = btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, b.networkParam)
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

	// Omit a zero or dust change output. A change output at or below the dust
	// threshold is uneconomical and rejected by relay rules, which would make the
	// whole tx unbroadcastable. Dropping it folds the remainder into the miner fee
	// (inputs - amountToSend is what the network takes), which is the standard way
	// to handle sub-dust change.
	if changeAmount < dustChangeThreshold {
		return []*wire.TxOut{recipientOutput}, nil
	}

	// Prepare change output
	changeAddress, err := btcutil.DecodeAddress(senderAddress.EncodeAddress(), b.networkParam)
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

	var txID string
	alreadyKnown := false
	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		txID, err = bs.BroadcastTx(txHex)
		if errors.Is(err, blockstream.ErrTxAlreadyKnown) {
			// The node already has this tx: it is LIVE. Stop retrying (returning
			// the error would make withRetry hop endpoints and re-POST). The node
			// gives no txid on this path, so use the one we computed locally.
			alreadyKnown = true
			return nil
		}
		return err
	})
	if alreadyKnown {
		return tx.TxHash().String(), nil
	}
	if err != nil {
		return "", err
	}

	return txID, nil
}

// verifyAndSelectUTXOs checks if there are sufficient funds across all UTXOs
// and returns selected UTXOs that cover the required amount
func (b *BtcRpc) verifyAndSelectUTXOs(address string, amountToSend, txFee int64) ([]blockstream.UTXO, bool) {
	var utxos []blockstream.UTXO
	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		utxos, err = bs.GetUTXOs(address)
		return err
	})
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

// orderSpendableUTXOs returns the treasury's own UTXOs in SELECTION order:
// confirmed first (value desc), then unconfirmed (value desc).
//
// Every UTXO passed here was returned by GetUTXOs(treasuryAddress), i.e. the
// esplora/mempool `/address/{addr}/utxo` set for the treasury's OWN address, so
// an unconfirmed entry is by construction a change output paying back to the
// treasury (self-change from a not-yet-confirmed prior payout). That is why no
// per-UTXO address field is needed: the query address IS the ownership proof.
//
// Confirmed-first ordering makes the greedy selector spend confirmed funds first
// and only chain onto unconfirmed self-change when confirmed funds cannot cover
// the payout. This stops back-to-back payouts from stranding treasury liquidity
// in an unconfirmed change output.
func orderSpendableUTXOs(utxos []blockstream.UTXO) []blockstream.UTXO {
	var confirmed, unconfirmed []blockstream.UTXO
	for _, utxo := range utxos {
		if utxo.Status.Confirmed {
			confirmed = append(confirmed, utxo)
		} else {
			unconfirmed = append(unconfirmed, utxo)
		}
	}
	sort.Slice(confirmed, func(i, j int) bool { return confirmed[i].Value > confirmed[j].Value })
	sort.Slice(unconfirmed, func(i, j int) bool { return unconfirmed[i].Value > unconfirmed[j].Value })

	ordered := make([]blockstream.UTXO, 0, len(confirmed)+len(unconfirmed))
	ordered = append(ordered, confirmed...)
	ordered = append(ordered, unconfirmed...)
	return ordered
}

// getSpendableUTXOs fetches the treasury address's UTXO set and returns it in
// selection order (see orderSpendableUTXOs): confirmed first, then unconfirmed
// self-change as a fallback.
func (b *BtcRpc) getSpendableUTXOs(address string) ([]blockstream.UTXO, error) {
	var utxos []blockstream.UTXO
	err := b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		utxos, err = bs.GetUTXOs(address)
		return err
	})
	if err != nil {
		return nil, err
	}

	return orderSpendableUTXOs(utxos), nil
}

// selectUTXOs picks UTXOs until we have enough to cover amountToSend + fee
// returns selected UTXOs and change amount
// change amount is the amount sent back to sender after sending total amount of selected UTXOs to recipient
// changeAmount = total amount of selected UTXOs - amountToSend - fee
func (b *BtcRpc) selectUTXOs(address string, amountToSend int64) (selected []blockstream.UTXO, changeAmount int64, fee int64, err error) {
	// Confirmed UTXOs first, then unconfirmed self-change as a fallback, so a
	// back-to-back payout can chain onto its own not-yet-confirmed change instead
	// of stranding on "insufficient funds" (see orderSpendableUTXOs).
	spendableUTXOs, err := b.getSpendableUTXOs(address)
	if err != nil {
		return nil, 0, 0, err
	}

	// Get current fee rate from mempool
	var feeRates map[string]float64
	err = b.withRetry(func(bs blockstream.IBlockStream) error {
		var err error
		feeRates, err = bs.EstimateFees()
		return err
	})
	if err != nil {
		return nil, 0, 0, err
	}

	// Iteratively select UTXOs until we have enough to cover amount + fee
	var totalSelected int64

	for _, utxo := range spendableUTXOs {
		selected = append(selected, utxo)
		totalSelected += utxo.Value

		// calculate tx fee based on the size of the transaction
		// n inputs: number of UTXOs whose total amount can cover the required amount (amountToSend + fee)
		// 2 outputs: 1 output tx for sending `amountToSend` to recipient, 1 output tx for sending `changeAmount` back to sender
		// targetBlocks confirmations: widely accepted standard for bitcoin transactions
		fee, err = b.calculateTxFee(feeRates, len(selected), 2, 6)
		if err != nil {
			return nil, 0, 0, err
		}

		if fee > amountToSend {
			return nil, 0, 0, fmt.Errorf("fee exceeds amount to send: fee %d, amountToSend %d", fee, amountToSend)
		}

		satoshiRate, err := b.GetSatoshiUSDPrice()
		if err != nil {
			return nil, 0, 0, err
		}

		// calculate and round up to 1 decimal places
		usdFee := math.Ceil(float64(fee)/satoshiRate*10) / 10

		if usdFee > b.appConfig.Bitcoin.MaxTxFeeUSD {
			return nil, 0, 0, fmt.Errorf("fee exceeds maximum threshold: usdFee %0.1f, MaxTxFeeUSD %0.1f", usdFee, b.appConfig.Bitcoin.MaxTxFeeUSD)
		}

		// if we have enough to cover amount + current fee => return selected UTXOs and change amount
		if totalSelected >= amountToSend+fee {
			b.logger.Info("[selectUTXOs] calculateTxFee", map[string]string{
				"amountToSend": fmt.Sprintf("%d", amountToSend),
				"fee":          fmt.Sprintf("%d", fee),
				"usdFee":       fmt.Sprintf("%0.1f", usdFee),
			})
			changeAmount = totalSelected - amountToSend - fee
			return selected, changeAmount, fee, nil
		}
	}

	return nil, 0, 0, fmt.Errorf(
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

func (b *BtcRpc) GetSatoshiUSDPrice() (float64, error) {
	// call from cache
	if x, found := b.cch.Get("satoshiPerUSD"); found {
		b.logger.Info("[GetSatoshiUSDPrice] cache hit", map[string]string{
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

// GetDustLimit returns the dust limit in satoshis based on the address type.
func (b *BtcRpc) getDustLimit(address string) int64 {
	if strings.HasPrefix(address, "1") {
		return 546 // P2PKH
	} else if strings.HasPrefix(address, "3") {
		return 540 // P2SH-P2WPKH
	} else if strings.HasPrefix(address, "bc1q") {
		return 294 // P2WPKH
	} else if strings.HasPrefix(address, "bc1p") {
		return 330 // P2TR
	}
	return 546 // Unknown type
}

// IsDust checks if the given amount is below the dust limit for the address.
func (b *BtcRpc) IsDust(address string, amount int64) bool {
	dustLimit := b.getDustLimit(address)
	if dustLimit == 0 {
		return false // Handle unknown type as not dust
	}
	return amount < dustLimit
}
