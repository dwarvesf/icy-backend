package swap

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"golang.org/x/time/rate"
)

// Wallet-signature auth for /swap/generate-signature.
//
// The endpoint's ApiKey is a NEXT_PUBLIC_ value inlined into the browser
// bundle, so it identifies nobody: a shared secret handed to every visitor.
// Instead the caller signs an EIP-712 SwapRequest with the wallet that will
// call swap(), and we recover the address from it. That gives a real caller
// identity for attribution and per-wallet throttling, without a shared secret.
//
// This does NOT make the swap signature non-transferable, the on-chain hash
// still omits msg.sender, so a signature remains bearer until the contract
// binds the caller. This is the off-chain half.

const (
	// eip712Name/Version identify this request domain. They must match the
	// frontend's domain exactly or recovery yields a different address.
	eip712Name    = "IcySwap"
	eip712Version = "1"

	// walletAuthMaxAge bounds how long a captured auth signature stays usable.
	// The deadline is carried inside the signed payload so it cannot be
	// altered in transit.
	walletAuthMaxAge = 5 * time.Minute
)

var (
	ErrWalletAuthMissing  = errors.New("wallet signature is required")
	ErrWalletAuthInvalid  = errors.New("wallet signature is invalid")
	ErrWalletAuthExpired  = errors.New("wallet signature has expired")
	ErrWalletAuthMismatch = errors.New("wallet signature does not cover this request")
)

// walletLimiter is the INNER, precise gate: once a caller is authenticated we
// throttle by wallet rather than IP. A wallet is the better key, an IP is
// shared behind NAT and trivially rotated, whereas signing as a wallet needs
// its private key. The outer per-IP limit in the transport layer still bounds
// unauthenticated traffic.
var walletLimiter = newWalletRateLimiter(walletRatePerMinute, walletBurst)

const (
	walletRatePerMinute = 10
	walletBurst         = 4
	walletVisitorTTL    = 10 * time.Minute
)

type walletVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type walletRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*walletVisitor
	every    rate.Limit
	burst    int
}

func newWalletRateLimiter(perMinute, burst int) *walletRateLimiter {
	l := &walletRateLimiter{
		visitors: make(map[string]*walletVisitor),
		every:    rate.Every(time.Minute / time.Duration(perMinute)),
		burst:    burst,
	}
	go func() {
		for {
			time.Sleep(walletVisitorTTL)
			l.mu.Lock()
			for k, v := range l.visitors {
				if time.Since(v.lastSeen) > walletVisitorTTL {
					delete(l.visitors, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// maxWalletVisitors caps limiter memory. Keys here are attacker-chosen, so the
// bound matters more than for the IP limiter.
const maxWalletVisitors = 50000

func (l *walletRateLimiter) allow(wallet string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[wallet]
	if !ok {
		// Fail closed when full, same reasoning as the IP limiter.
		if len(l.visitors) >= maxWalletVisitors {
			return false
		}
		v = &walletVisitor{limiter: rate.NewLimiter(l.every, l.burst)}
		l.visitors[wallet] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

// ErrWalletRateLimited is returned when an authenticated wallet asks for
// signatures faster than walletRatePerMinute.
var ErrWalletRateLimited = errors.New("too many signature requests for this wallet")

// ErrInsufficientICY is returned when the recovered wallet does not hold the
// ICY it is asking to swap. This is an anti-Sybil gate, not the swap authority.
var ErrInsufficientICY = errors.New("wallet does not hold enough ICY for this swap")

// authenticateCaller verifies the wallet signature on a swap request and
// returns the recovered address, or "" when auth is not required and none was
// supplied.
//
// Rollout: a signature that IS supplied is always verified, so a bad one is
// rejected even before enforcement. Only the "none supplied" case is governed
// by RequireWalletAuth, which lets the frontend deploy ahead of the flag.
func (h *handler) authenticateCaller(req *GenerateSignatureRequest) (string, error) {
	required := h.appConfig.ApiServer.RequireWalletAuth

	if strings.TrimSpace(req.WalletSignature) == "" {
		if required {
			return "", ErrWalletAuthMissing
		}
		return "", nil
	}

	addr, err := RecoverSwapRequestSigner(
		h.appConfig.ApiServer.WalletAuthChainID,
		h.appConfig.Blockchain.ICYSwapContractAddr,
		req.ICYAmount,
		req.BTCAddress,
		req.WalletDeadline,
		req.WalletSignature,
	)
	if err != nil {
		return "", err
	}
	wallet := strings.ToLower(addr.Hex())

	// ecrecover always yields SOME address, so a signature alone proves only
	// that the caller holds A key, and keys are free: measured at ~113us to
	// mint a fresh identity, cheaper than rotating an IP. Without a comparison
	// against something scarce, the per-wallet limiter never binds and this is
	// attribution rather than authentication.
	//
	// The scarce thing is ICY. A caller who cannot pay for the swap has no
	// business consuming signer capacity, so require the recovered address to
	// actually hold at least the amount it is asking to swap.
	if err := h.requireSufficientICY(wallet, req.ICYAmount); err != nil {
		return "", err
	}

	if !walletLimiter.allow(wallet) {
		return "", ErrWalletRateLimited
	}
	return wallet, nil
}

// requireSufficientICY rejects a caller whose recovered address does not hold
// the ICY it is asking to swap.
//
// This is what makes wallet identity cost something. Note it is a
// rate-limiting/anti-Sybil control, NOT the authority for the swap itself: the
// contract still pulls ICY from msg.sender and the payout stays oracle-derived,
// so a stale balance here cannot authorise value movement.
func (h *handler) requireSufficientICY(wallet string, icyAmount string) error {
	want, ok := new(big.Int).SetString(icyAmount, 10)
	if !ok || want.Sign() <= 0 {
		return ErrWalletAuthInvalid
	}

	balance, err := h.baseRPC.ICYBalanceOf(wallet)
	if err != nil {
		// Fail OPEN on an RPC failure. This gate exists to raise the cost of
		// Sybil identities, not to guard funds; letting a Base outage stop
		// every legitimate swap would trade a real outage for a marginal
		// abuse win. The oracle-derived amount and the contract still bound
		// what a signature can do.
		h.logger.Error("[requireSufficientICY][ICYBalanceOf]", map[string]string{
			"error":  err.Error(),
			"wallet": wallet,
		})
		return nil
	}

	have, ok := new(big.Int).SetString(balance.Value, 10)
	if !ok {
		return nil
	}
	if have.Cmp(want) < 0 {
		return ErrInsufficientICY
	}
	return nil
}

// swapRequestTypedData rebuilds the exact EIP-712 payload the client signed.
// Any divergence here (field order, types, domain) changes the digest and the
// recovered address, so this is the single source of truth for both sides.
func swapRequestTypedData(
	chainID int64,
	verifyingContract string,
	icyAmount string,
	btcAddress string,
	deadline int64,
) apitypes.TypedData {
	return apitypes.TypedData{
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
			Name:              eip712Name,
			Version:           eip712Version,
			ChainId:           (*math.HexOrDecimal256)(big.NewInt(chainID)),
			VerifyingContract: verifyingContract,
		},
		Message: apitypes.TypedDataMessage{
			"icyAmount":  icyAmount,
			"btcAddress": btcAddress,
			"deadline":   fmt.Sprintf("%d", deadline),
		},
	}
}

// RecoverSwapRequestSigner verifies sig over the SwapRequest fields and returns
// the wallet address that produced it.
//
// The signed payload covers icyAmount and btcAddress, so a captured signature
// cannot be repurposed for a different amount or a different payout address,
// which is the property that makes this worth doing at all.
func RecoverSwapRequestSigner(
	chainID int64,
	verifyingContract string,
	icyAmount string,
	btcAddress string,
	deadline int64,
	sig string,
) (common.Address, error) {
	var zero common.Address

	if strings.TrimSpace(sig) == "" {
		return zero, ErrWalletAuthMissing
	}
	if deadline <= 0 {
		return zero, ErrWalletAuthInvalid
	}

	now := time.Now().Unix()
	if deadline < now {
		return zero, ErrWalletAuthExpired
	}
	// Reject a deadline further out than we allow, otherwise a client could mint
	// a signature valid for a year and hand it around.
	if deadline > now+int64(walletAuthMaxAge.Seconds()) {
		return zero, ErrWalletAuthInvalid
	}

	raw := common.FromHex(sig)
	if len(raw) != 65 {
		return zero, ErrWalletAuthInvalid
	}

	// Wallets return v as 27/28; secp256k1 recovery wants 0/1.
	if raw[64] == 27 || raw[64] == 28 {
		raw[64] -= 27
	}
	if raw[64] != 0 && raw[64] != 1 {
		return zero, ErrWalletAuthInvalid
	}

	typed := swapRequestTypedData(chainID, verifyingContract, icyAmount, btcAddress, deadline)

	domainSeparator, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	if err != nil {
		return zero, ErrWalletAuthInvalid
	}
	messageHash, err := typed.HashStruct(typed.PrimaryType, typed.Message)
	if err != nil {
		return zero, ErrWalletAuthInvalid
	}

	digest := crypto.Keccak256(
		[]byte{0x19, 0x01},
		domainSeparator,
		messageHash,
	)

	pub, err := crypto.SigToPub(digest, raw)
	if err != nil {
		return zero, ErrWalletAuthInvalid
	}
	return crypto.PubkeyToAddress(*pub), nil
}
