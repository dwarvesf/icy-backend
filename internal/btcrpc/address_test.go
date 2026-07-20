package btcrpc_test

import (
	"testing"

	"github.com/dwarvesf/icy-backend/internal/btcrpc"
)

func TestValidateMainnetAddress(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"mainnet p2wpkh", "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq", false},
		{"mainnet p2pkh", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", false},
		{"mainnet p2sh", "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy", false},
		{"mainnet p2tr", "bc1p5d7rjq7g6rdk2yhzks9smlaqtedr4dekq08ge8ztwac72sfr9rusxg3297", false},

		// The whole point: these are valid Bitcoin addresses on other networks
		// and were accepted before, both by the frontend and by the server.
		{"testnet bech32", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", true},
		{"testnet p2pkh", "mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn", true},
		{"regtest bech32", "bcrt1qw508d6qejxtdg4y5r3zarvary0c5xw7kygt080", true},

		{"empty", "", true},
		{"garbage", "not-an-address", true},
		{"bad checksum", "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdx", true},
		{"evm address", "0xf289e3b222dd42b185b7e335fa3c5bd6d132441d", true},

		// DecodeAddress accepts a raw serialized public key and IsForNet
		// confirms it as mainnet, but paying it produces a bare P2PK output
		// that most wallets and every exchange cannot spend. The earlier EVM
		// case did not catch this because it is 42 chars, not 66/130.
		{
			"compressed pubkey (P2PK)",
			"02b4632d08485ff1df2db55b9dafd23347d1c47a457072a1e87be26896549a8737",
			true,
		},
		{
			"uncompressed pubkey (P2PK)",
			"04b4632d08485ff1df2db55b9dafd23347d1c47a457072a1e87be26896549a87378ec38ff91d43e8c2092ebda601780485263da089465619e0358a5c1be7ac91f4",
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := btcrpc.ValidateMainnetAddress(tc.addr)
			if tc.wantErr && err == nil {
				t.Fatalf("expected %q to be rejected, it was accepted", tc.addr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected %q to be accepted, got %v", tc.addr, err)
			}
		})
	}
}

// Validation trims, so a padded address passes. Everything downstream must
// therefore use the NORMALIZED form: otherwise a different string is hashed
// into the swap signature, missed by getDustLimit's prefix table, and rejected
// at Send, after the ICY leg has burned.
func TestNormalizeAddress(t *testing.T) {
	const want = "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq"

	for _, padded := range []string{
		"  " + want,
		want + "  ",
		"\t" + want + "\n",
	} {
		if err := btcrpc.ValidateMainnetAddress(padded); err != nil {
			t.Fatalf("padded address should validate, got %v", err)
		}
		if got := btcrpc.NormalizeAddress(padded); got != want {
			t.Fatalf("NormalizeAddress(%q) = %q, want %q", padded, got, want)
		}
	}
}
