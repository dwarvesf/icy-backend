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

// mkNonce assembles a valid bytes32 nonce (0x + 64 hex) from a short tag, so no
// 64-char hex literal appears in the source.
func mkNonce(tag string) string {
	return "0x" + strings.Repeat("0", 64-len(tag)) + tag
}

// signAsWalletWithNonce is signAsWallet plus the 4th bytes32 "nonce" field, so
// the digest matches the 4-field path in RecoverSwapRequestSignerWithNonce. Like
// signAsWallet it is an INDEPENDENT construction of the digest.
func signAsWalletWithNonce(t *testing.T, key string, deadline int64, nonce string) string {
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
				{Name: "nonce", Type: "bytes32"},
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
			"nonce":      nonce,
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
	sig[64] += 27
	return "0x" + common_Bytes2Hex(sig)
}

// A 4-field (nonce) signature recovers to the signer, exactly like the 3-field
// path. If the nonce were not folded into the digest identically on both sides,
// this would recover a different address.
func TestRecoverSwapRequestSignerWithNonce_RoundTrip(t *testing.T) {
	key, want := testWallet(t)
	deadline := time.Now().Add(2 * time.Minute).Unix()
	nonce := mkNonce("ab")

	got, err := RecoverSwapRequestSignerWithNonce(
		testChainID, testContract, testICY, testBTCAddr, deadline, nonce,
		signAsWalletWithNonce(t, key, deadline, nonce),
	)
	if err != nil {
		t.Fatalf("valid 4-field signature rejected: %v", err)
	}
	if strings.ToLower(got.Hex()) != want {
		t.Fatalf("recovered %s, want %s", strings.ToLower(got.Hex()), want)
	}
}

// A signature signed WITH a nonce must not verify against the legacy 3-field
// digest, and vice-versa: the two digests are distinct domains.
func TestRecoverSwapRequestSigner_NonceAndLegacyDigestsDiffer(t *testing.T) {
	key, want := testWallet(t)
	deadline := time.Now().Add(2 * time.Minute).Unix()
	nonce := mkNonce("ab")

	// A nonce-signed signature fed to the legacy (nonce-less) recovery recovers
	// someone other than the signer.
	nonceSig := signAsWalletWithNonce(t, key, deadline, nonce)
	got, err := RecoverSwapRequestSigner(testChainID, testContract, testICY, testBTCAddr, deadline, nonceSig)
	if err == nil && strings.ToLower(got.Hex()) == want {
		t.Fatal("nonce-signed signature was accepted by the legacy 3-field digest")
	}

	// And the reverse: a legacy signature does not verify under the 4-field digest.
	legacySig := signAsWallet(t, key, deadline)
	got2, err := RecoverSwapRequestSignerWithNonce(testChainID, testContract, testICY, testBTCAddr, deadline, nonce, legacySig)
	if err == nil && strings.ToLower(got2.Hex()) == want {
		t.Fatal("legacy signature was accepted by the 4-field nonce digest")
	}
}

func TestNormalizeNonce(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
		out  string
	}{
		{"empty is legacy", "", true, ""},
		{"valid lowercased", "0x" + strings.Repeat("A", 64), true, "0x" + strings.Repeat("a", 64)},
		{"missing 0x", strings.Repeat("a", 64), false, ""},
		{"too short", "0x" + strings.Repeat("a", 63), false, ""},
		{"too long", "0x" + strings.Repeat("a", 65), false, ""},
		{"non-hex", "0x" + strings.Repeat("g", 64), false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ok := normalizeNonce(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && out != tc.out {
				t.Fatalf("out = %q, want %q", out, tc.out)
			}
		})
	}
}

// The replay guard: a (wallet,nonce) accepted once is rejected on the second
// call; a DIFFERENT nonce is accepted (negative control); and an entry whose
// expiry has passed is accepted again (the signature would itself be expired).
func TestNonceReplayGuard(t *testing.T) {
	g := &nonceReplayGuard{seen: make(map[string]time.Time)}
	future := time.Now().Add(1 * time.Minute)
	nonce := mkNonce("ab")

	if replay := g.checkAndRecord("wallet:"+nonce, future); replay {
		t.Fatal("first use flagged as replay")
	}
	if replay := g.checkAndRecord("wallet:"+nonce, future); !replay {
		t.Fatal("second use of the same nonce not flagged as replay")
	}
	if replay := g.checkAndRecord("wallet:"+mkNonce("cd"), future); replay {
		t.Fatal("a different nonce was flagged as replay")
	}
	// An already-expired entry is treated as not seen (accepted again).
	g.seen["wallet:expired"] = time.Now().Add(-1 * time.Second)
	if replay := g.checkAndRecord("wallet:expired", future); replay {
		t.Fatal("an expired nonce entry was flagged as replay")
	}
}

// End-to-end through authenticateCaller: a nonce-bearing request is accepted the
// first time and rejected as a replay the second time; the legacy no-nonce path
// still authenticates; a malformed nonce is rejected outright.
func TestAuthenticateCaller_NoncePathAndReplay(t *testing.T) {
	key, wantWallet := testWallet(t)
	h := newAuthTestHandler(t)

	t.Run("nonce accepted once then rejected on replay", func(t *testing.T) {
		deadline := time.Now().Add(2 * time.Minute).Unix()
		nonce := mkNonce("11ab")
		req := &GenerateSignatureRequest{
			ICYAmount:       testICY,
			BTCAddress:      testBTCAddr,
			WalletDeadline:  deadline,
			WalletNonce:     nonce,
			WalletSignature: signAsWalletWithNonce(t, key, deadline, nonce),
		}
		got, err := h.authenticateCaller(req)
		if err != nil {
			t.Fatalf("first use rejected: %v", err)
		}
		if got != wantWallet {
			t.Fatalf("wallet = %s, want %s", got, wantWallet)
		}
		if _, err := h.authenticateCaller(req); err != ErrWalletAuthReplay {
			t.Fatalf("replay error = %v, want ErrWalletAuthReplay", err)
		}
	})

	t.Run("legacy no-nonce still authenticates", func(t *testing.T) {
		deadline := time.Now().Add(2 * time.Minute).Unix()
		req := &GenerateSignatureRequest{
			ICYAmount:       testICY,
			BTCAddress:      testBTCAddr,
			WalletDeadline:  deadline,
			WalletSignature: signAsWallet(t, key, deadline),
		}
		got, err := h.authenticateCaller(req)
		if err != nil {
			t.Fatalf("legacy request rejected: %v", err)
		}
		if got != wantWallet {
			t.Fatalf("wallet = %s, want %s", got, wantWallet)
		}
	})

	t.Run("malformed nonce rejected", func(t *testing.T) {
		deadline := time.Now().Add(2 * time.Minute).Unix()
		req := &GenerateSignatureRequest{
			ICYAmount:       testICY,
			BTCAddress:      testBTCAddr,
			WalletDeadline:  deadline,
			WalletNonce:     "0xdeadbeef", // not 32 bytes
			WalletSignature: signAsWallet(t, key, deadline),
		}
		if _, err := h.authenticateCaller(req); err != ErrWalletAuthInvalid {
			t.Fatalf("malformed nonce error = %v, want ErrWalletAuthInvalid", err)
		}
	})
}
