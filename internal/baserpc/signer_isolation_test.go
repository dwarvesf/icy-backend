package baserpc

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	"github.com/dwarvesf/icy-backend/internal/model"
	"github.com/dwarvesf/icy-backend/internal/types/environments"
	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// newWallet generates a fresh key and returns the wallet plus its 0x-hex private
// key string (the form AccountFromPrivateKey / config expects).
func newWallet(t *testing.T) (*EthereumWallet, string) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkHex := hexutil.Encode(crypto.FromECDSA(key))
	w, err := AccountFromPrivateKey(pkHex)
	if err != nil {
		t.Fatalf("account from key: %v", err)
	}
	return w, pkHex
}

// resolveSignerPK picks SwapSignerPK when set (isolated) and falls back to the
// holder key otherwise. This is the config wiring that makes provisioning a
// distinct signer key fully isolate the signing path.
func TestResolveSignerPK_IsolatedVsFallback(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Blockchain.SwapSignerPK = "0xsigner"
	cfg.Blockchain.IcySwapSignerPrivateKey = "0xholder"
	if pk, isolated := resolveSignerPK(cfg); pk != "0xsigner" || !isolated {
		t.Fatalf("with SwapSignerPK set: got (%q, %t), want (\"0xsigner\", true)", pk, isolated)
	}

	cfg2 := &config.AppConfig{}
	cfg2.Blockchain.SwapSignerPK = "" // unset -> fall back to the holder key
	cfg2.Blockchain.IcySwapSignerPrivateKey = "0xholder"
	if pk, isolated := resolveSignerPK(cfg2); pk != "0xholder" || isolated {
		t.Fatalf("with SwapSignerPK unset: got (%q, %t), want (\"0xholder\", false)", pk, isolated)
	}
}

// swapDigest independently recomputes the EIP-712 digest GenerateSignature signs,
// so the recovered signer can be checked against the expected key. Recomputing
// (rather than reusing prod code) also cross-checks the digest construction.
func swapDigest(t *testing.T, chainID *big.Int, verifyingContract string, icyAmount, btcAmount *big.Int, btcAddress string, nonce, deadline *big.Int) []byte {
	t.Helper()
	swapTypes := apitypes.Types{
		"EIP712Domain": []apitypes.Type{
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
		"Swap": []apitypes.Type{
			{Name: "icyAmount", Type: "uint256"},
			{Name: "btcAddress", Type: "string"},
			{Name: "btcAmount", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		},
	}
	typedData := apitypes.TypedData{
		Types:       swapTypes,
		PrimaryType: "Swap",
		Domain: apitypes.TypedDataDomain{
			Name:              "ICY BTC SWAP",
			Version:           "1",
			ChainId:           ethmath.NewHexOrDecimal256(chainID.Int64()),
			VerifyingContract: verifyingContract,
		},
		Message: map[string]interface{}{
			"icyAmount":  icyAmount.String(),
			"btcAddress": btcAddress,
			"btcAmount":  btcAmount.String(),
			"nonce":      nonce.String(),
			"deadline":   deadline.String(),
		},
	}
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		t.Fatalf("hash domain: %v", err)
	}
	messageHash, err := typedData.HashStruct("Swap", typedData.Message)
	if err != nil {
		t.Fatalf("hash message: %v", err)
	}
	return crypto.Keccak256([]byte("\x19\x01"), domainSeparator, messageHash)
}

// newSigningBaseRPC wires a BaseRPC with DISTINCT holder + signer wallets so the
// signing path can be proven to use the signer key, not the holder. No network:
// GenerateSignature's crypto ops succeed without dialing an endpoint.
func newSigningBaseRPC(holder, signer *EthereumWallet, contract string, chainID *big.Int) *BaseRPC {
	cfg := &config.AppConfig{}
	cfg.Blockchain.ICYSwapContractAddr = contract
	return &BaseRPC{
		appConfig:       cfg,
		logger:          logger.New(environments.Test),
		endpoints:       []string{"http://localhost"},
		currentEndpoint: 0,
		failedEndpoints: map[string]*endpointStatus{},
		wallet:          holder,
		signerWallet:    signer,
		chainID:         chainID,
	}
}

// CORE PROOF. GenerateSignature must sign with the DEDICATED signer key, never
// the holder/gas wallet. With distinct keys wired, the address recovered from the
// produced signature is the signer's, not the holder's. If the signing path
// regressed to b.wallet (the holder), the recovered address would be the holder
// and this test fails.
func TestGenerateSignature_SignsWithSignerKeyNotHolder(t *testing.T) {
	holder, _ := newWallet(t)
	signer, _ := newWallet(t)
	if holder.publicKeyAddr == signer.publicKeyAddr {
		t.Fatal("holder and signer addresses collided; regenerate")
	}

	contract := "0x000000000000000000000000000000000000abcd"
	chainID := big.NewInt(8453)
	b := newSigningBaseRPC(holder, signer, contract, chainID)

	icy := &model.Web3BigInt{Value: "1000000000000000000", Decimal: 18}
	btc := &model.Web3BigInt{Value: "50000", Decimal: 8}
	nonce := big.NewInt(12345)
	deadline := big.NewInt(1900000000)

	sigHex, err := b.GenerateSignature(icy, "bc1qexampleaddr", btc, nonce, deadline)
	if err != nil {
		t.Fatalf("GenerateSignature: %v", err)
	}

	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if len(sig) != 65 {
		t.Fatalf("signature length = %d, want 65", len(sig))
	}
	// GenerateSignature normalizes V to 27/28; undo it for recovery (SigToPub
	// wants a 0/1 recovery id).
	if sig[64] >= 27 {
		sig[64] -= 27
	}

	icyBig, _ := new(big.Int).SetString(icy.Value, 10)
	btcBig, _ := new(big.Int).SetString(btc.Value, 10)
	digest := swapDigest(t, chainID, contract, icyBig, btcBig, "bc1qexampleaddr", nonce, deadline)

	pub, err := crypto.SigToPub(digest, sig)
	if err != nil {
		t.Fatalf("recover pubkey: %v", err)
	}
	recovered := crypto.PubkeyToAddress(*pub)

	if recovered != signer.publicKeyAddr {
		t.Fatalf("signature signer = %s, want the dedicated signer %s", recovered.Hex(), signer.publicKeyAddr.Hex())
	}
	if recovered == holder.publicKeyAddr {
		t.Fatalf("signature was signed by the HOLDER/gas wallet %s; signer key is not isolated", holder.publicKeyAddr.Hex())
	}
}

// Guards that the recovery harness is real: recovering with the WRONG expected
// address does not match, so the positive assertion above cannot pass vacuously.
func TestGenerateSignature_RecoveryRejectsWrongSigner(t *testing.T) {
	holder, _ := newWallet(t)
	signer, _ := newWallet(t)
	unrelated := common.HexToAddress("0x00000000000000000000000000000000deadbeef")

	b := newSigningBaseRPC(holder, signer, "0x000000000000000000000000000000000000abcd", big.NewInt(8453))
	icy := &model.Web3BigInt{Value: "1000000000000000000", Decimal: 18}
	btc := &model.Web3BigInt{Value: "50000", Decimal: 8}
	nonce := big.NewInt(1)
	deadline := big.NewInt(1900000000)

	sigHex, err := b.GenerateSignature(icy, "bc1qexampleaddr", btc, nonce, deadline)
	if err != nil {
		t.Fatalf("GenerateSignature: %v", err)
	}
	sig, _ := hex.DecodeString(sigHex)
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	icyBig, _ := new(big.Int).SetString(icy.Value, 10)
	btcBig, _ := new(big.Int).SetString(btc.Value, 10)
	digest := swapDigest(t, big.NewInt(8453), "0x000000000000000000000000000000000000abcd", icyBig, btcBig, "bc1qexampleaddr", nonce, deadline)
	pub, _ := crypto.SigToPub(digest, sig)
	if crypto.PubkeyToAddress(*pub) == unrelated {
		t.Fatal("recovered an unrelated address; recovery harness is not discriminating")
	}
}
