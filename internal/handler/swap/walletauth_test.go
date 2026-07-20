package swap

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	testChainID  = int64(8453)
	testContract = "0xdA3E22edf0357c781154D8DEDcfC32D7B6B0B12D"
	testICY      = "20000000000000000000"
	testBTCAddr  = "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq"
)

// signAsWallet reproduces what a browser wallet does for eth_signTypedData_v4:
// keccak(0x1901 || domainSeparator || hashStruct(message)), signed with
// secp256k1, v returned as 27/28.
//
// This is deliberately an INDEPENDENT construction of the digest rather than a
// call into the code under test, so a mistake in the production path cannot
// cancel itself out in the test.
func signAsWallet(t *testing.T, key string, deadline int64) string {
	t.Helper()

	priv, err := crypto.HexToECDSA(key)
	if err != nil {
		t.Fatalf("bad test key: %v", err)
	}

	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"SwapRequest": []apitypes.Type{
				{Name: "icyAmount", Type: "uint256"},
				{Name: "btcAddress", Type: "string"},
				{Name: "deadline", Type: "uint256"},
			},
		},
		PrimaryType: "SwapRequest",
		Domain: apitypes.TypedDataDomain{
			Name:              "IcySwap",
			Version:           "1",
			ChainId:           (*math.HexOrDecimal256)(big.NewInt(testChainID)),
			VerifyingContract: testContract,
		},
		Message: apitypes.TypedDataMessage{
			"icyAmount":  testICY,
			"btcAddress": testBTCAddr,
			"deadline":   big.NewInt(deadline).String(),
		},
	}

	domainSep, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	if err != nil {
		t.Fatalf("domain hash: %v", err)
	}
	msgHash, err := typed.HashStruct("SwapRequest", typed.Message)
	if err != nil {
		t.Fatalf("message hash: %v", err)
	}

	digest := crypto.Keccak256([]byte{0x19, 0x01}, domainSep, msgHash)
	sig, err := crypto.Sign(digest, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[64] += 27 // wallets return v as 27/28
	return "0x" + common_Bytes2Hex(sig)
}

func common_Bytes2Hex(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexDigits[v>>4]
		out[i*2+1] = hexDigits[v&0x0f]
	}
	return string(out)
}

func testWallet(t *testing.T) (key string, addr string) {
	t.Helper()
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return common_Bytes2Hex(crypto.FromECDSA(priv)),
		strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
}

// The whole point: a signature produced the way a wallet produces one must
// recover to that wallet's address. If the digest construction is off by a
// byte this fails, and every real user would be rejected.
func TestRecoverSwapRequestSigner_RoundTrip(t *testing.T) {
	key, want := testWallet(t)
	deadline := time.Now().Add(2 * time.Minute).Unix()

	got, err := RecoverSwapRequestSigner(
		testChainID, testContract, testICY, testBTCAddr, deadline,
		signAsWallet(t, key, deadline),
	)
	if err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if strings.ToLower(got.Hex()) != want {
		t.Fatalf("recovered %s, want %s", strings.ToLower(got.Hex()), want)
	}
}

// A signature must not be reusable for a different payout address or amount,
// which is the property that makes this auth rather than decoration.
func TestRecoverSwapRequestSigner_BoundToRequest(t *testing.T) {
	key, want := testWallet(t)
	deadline := time.Now().Add(2 * time.Minute).Unix()
	sig := signAsWallet(t, key, deadline)

	t.Run("different btc address recovers someone else", func(t *testing.T) {
		got, err := RecoverSwapRequestSigner(
			testChainID, testContract, testICY,
			"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", deadline, sig,
		)
		if err == nil && strings.ToLower(got.Hex()) == want {
			t.Fatal("signature was accepted for a different payout address")
		}
	})

	t.Run("different amount recovers someone else", func(t *testing.T) {
		got, err := RecoverSwapRequestSigner(
			testChainID, testContract, "999000000000000000000",
			testBTCAddr, deadline, sig,
		)
		if err == nil && strings.ToLower(got.Hex()) == want {
			t.Fatal("signature was accepted for a different amount")
		}
	})

	t.Run("different chain recovers someone else", func(t *testing.T) {
		got, err := RecoverSwapRequestSigner(
			1, testContract, testICY, testBTCAddr, deadline, sig,
		)
		if err == nil && strings.ToLower(got.Hex()) == want {
			t.Fatal("signature was accepted for a different chain")
		}
	})
}

func TestRecoverSwapRequestSigner_Rejects(t *testing.T) {
	key, _ := testWallet(t)
	valid := time.Now().Add(2 * time.Minute).Unix()

	cases := []struct {
		name     string
		deadline int64
		sig      string
	}{
		{"empty signature", valid, ""},
		{"garbage signature", valid, "0xdeadbeef"},
		{"expired deadline", time.Now().Add(-1 * time.Minute).Unix(), signAsWallet(t, key, valid)},
		{"zero deadline", 0, signAsWallet(t, key, valid)},
		{
			"deadline further out than allowed",
			time.Now().Add(24 * time.Hour).Unix(),
			signAsWallet(t, key, valid),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RecoverSwapRequestSigner(
				testChainID, testContract, testICY, testBTCAddr, tc.deadline, tc.sig,
			); err == nil {
				t.Fatal("expected rejection, got none")
			}
		})
	}
}
