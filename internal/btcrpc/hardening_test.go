package btcrpc

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/dwarvesf/icy-backend/internal/btcrpc/blockstream"
)

// deriveP2WPKHAddress makes a fresh, valid P2WPKH address for the given network
// from a random key, so tests never embed a private key or a hardcoded address.
func deriveP2WPKHAddress(t *testing.T, params *chaincfg.Params) *btcutil.AddressWitnessPubKeyHash {
	t.Helper()
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	pubKeyHash := btcutil.Hash160(priv.PubKey().SerializeCompressed())
	addr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, params)
	if err != nil {
		t.Fatalf("derive address: %v", err)
	}
	return addr
}

// Item 5: a zero or dust change output is omitted (1 output), so the tx stays
// broadcastable; an above-dust change produces the usual 2 outputs.
func TestPrepareTxOutputs_OmitsDustChange(t *testing.T) {
	params := &chaincfg.MainNetParams
	b := &BtcRpc{networkParam: params}
	receiver := deriveP2WPKHAddress(t, params)
	sender := deriveP2WPKHAddress(t, params)
	const amountToSend = int64(100_000)

	cases := []struct {
		name        string
		changeAmt   int64
		wantOutputs int
	}{
		{"zero change omitted", 0, 1},
		{"below-dust change omitted", 300, 1},
		{"at dust threshold kept", dustChangeThreshold, 2}, // 546 is not below dust
		{"above-dust change kept", 1000, 2},                // negative control
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outputs, err := b.prepareTxOutputs(receiver, sender, amountToSend, tc.changeAmt)
			if err != nil {
				t.Fatalf("prepareTxOutputs: %v", err)
			}
			if len(outputs) != tc.wantOutputs {
				t.Fatalf("outputs = %d, want %d", len(outputs), tc.wantOutputs)
			}
			// The recipient output is always present and unchanged.
			if outputs[0].Value != amountToSend {
				t.Fatalf("recipient value = %d, want %d", outputs[0].Value, amountToSend)
			}
		})
	}
}

// mkUTXO builds a UTXO with the given value and confirmation state without naming
// the inline anonymous Status struct.
func mkUTXO(value int64, confirmed bool) blockstream.UTXO {
	u := blockstream.UTXO{Value: value}
	u.Status.Confirmed = confirmed
	return u
}

// selectGreedy mirrors selectUTXOs's selection loop (accumulate in order until
// the target is covered), so a test over the ORDERED list proves the same
// preference production applies. Fee only raises the target, never reorders.
func selectGreedy(ordered []blockstream.UTXO, target int64) []blockstream.UTXO {
	var total int64
	var selected []blockstream.UTXO
	for _, u := range ordered {
		selected = append(selected, u)
		total += u.Value
		if total >= target {
			break
		}
	}
	return selected
}

func countUnconfirmed(utxos []blockstream.UTXO) int {
	n := 0
	for _, u := range utxos {
		if !u.Status.Confirmed {
			n++
		}
	}
	return n
}

// Item 6: ordering puts confirmed first (value desc), then unconfirmed
// self-change (value desc).
func TestOrderSpendableUTXOs_ConfirmedFirst(t *testing.T) {
	in := []blockstream.UTXO{
		mkUTXO(500, false),  // unconfirmed
		mkUTXO(3000, true),  // confirmed
		mkUTXO(1000, false), // unconfirmed
		mkUTXO(2000, true),  // confirmed
	}
	got := orderSpendableUTXOs(in)

	wantValues := []int64{3000, 2000, 1000, 500}
	wantConfirmed := []bool{true, true, false, false}
	if len(got) != len(wantValues) {
		t.Fatalf("len = %d, want %d", len(got), len(wantValues))
	}
	for i := range got {
		if got[i].Value != wantValues[i] || got[i].Status.Confirmed != wantConfirmed[i] {
			t.Fatalf("pos %d = {value %d, confirmed %v}, want {value %d, confirmed %v}",
				i, got[i].Value, got[i].Status.Confirmed, wantValues[i], wantConfirmed[i])
		}
	}
}

// Item 6, order 1 (negative control): when confirmed funds alone cover the
// payout, the greedy selector never reaches the unconfirmed self-change.
func TestSelection_SufficientConfirmed_LeavesUnconfirmedUntouched(t *testing.T) {
	ordered := orderSpendableUTXOs([]blockstream.UTXO{
		mkUTXO(5000, true),  // confirmed
		mkUTXO(4000, true),  // confirmed
		mkUTXO(9000, false), // unconfirmed self-change
	})
	selected := selectGreedy(ordered, 6000) // covered by the two confirmed (9000)
	if n := countUnconfirmed(selected); n != 0 {
		t.Fatalf("selected %d unconfirmed UTXOs, want 0 (confirmed funds were sufficient)", n)
	}
}

// Item 6, order 2: when confirmed funds cannot cover the payout, the selector
// chains onto the unconfirmed self-change instead of stranding.
func TestSelection_InsufficientConfirmed_ChainsUnconfirmed(t *testing.T) {
	ordered := orderSpendableUTXOs([]blockstream.UTXO{
		mkUTXO(3000, true),  // confirmed
		mkUTXO(8000, false), // unconfirmed self-change
	})
	selected := selectGreedy(ordered, 6000) // 3000 confirmed alone is short
	if n := countUnconfirmed(selected); n == 0 {
		t.Fatal("selected 0 unconfirmed UTXOs, want the self-change chained in (confirmed funds were insufficient)")
	}
	// Confirmed is still spent first.
	if !selected[0].Status.Confirmed {
		t.Fatal("first selected UTXO is unconfirmed; confirmed funds must be preferred")
	}
}
