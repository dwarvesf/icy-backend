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

// ErrWalletAuthReplay is returned when a wallet re-submits a signature that
// carries a nonce already seen for that wallet.
var ErrWalletAuthReplay = errors.New("wallet signature nonce has already been used")

const (
	// nonceSlack is added to the signature deadline to decide how long a used
	// nonce is remembered. It only needs to outlive the signature's own validity
	// (deadline), after which RecoverSwapRequestSignerWithNonce rejects it as
	// expired anyway, so a small slack is enough.
	nonceSlack = 1 * time.Minute
	// nonceSweepInterval is how often expired nonces are purged.
	nonceSweepInterval = 1 * time.Minute
)

// nonceReplayGuard remembers (wallet,nonce) pairs until their signature expires
// so a signed request carrying a nonce cannot be replayed.
//
// CEILING: this is an in-PROCESS set. It protects a SINGLE replica only; two
// replicas do not share it, so the same nonce could be replayed once per
// replica. If this service is ever scaled past one replica, move the seen-nonce
// set to the database (a UNIQUE (wallet, nonce) row) so the check is global.
type nonceReplayGuard struct {
	mu   sync.Mutex
	seen map[string]time.Time // key -> expiry
}

// nonceGuard is the process-wide replay set. It is only consulted for requests
// that actually carry a nonce; legacy nonce-less requests never touch it.
var nonceGuard = newNonceReplayGuard()

func newNonceReplayGuard() *nonceReplayGuard {
	g := &nonceReplayGuard{seen: make(map[string]time.Time)}
	go func() {
		for {
			time.Sleep(nonceSweepInterval)
			g.sweep()
		}
	}()
	return g
}

// checkAndRecord returns true when key was ALREADY recorded and still unexpired
// (a replay, reject it); otherwise it records key with the given expiry and
// returns false (first use, accept it). The read and the write are one locked
// step so two concurrent replays cannot both pass.
func (g *nonceReplayGuard) checkAndRecord(key string, expiry time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if exp, ok := g.seen[key]; ok && exp.After(now) {
		return true
	}
	g.seen[key] = expiry
	return false
}

func (g *nonceReplayGuard) sweep() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for k, exp := range g.seen {
		if !exp.After(now) {
			delete(g.seen, k)
		}
	}
}

// normalizeNonce validates a bytes32 nonce (0x + exactly 64 hex chars) and
// returns it lowercased. An empty input means "no nonce" and is valid (ok=true,
// nonce=""). Any malformed non-empty input is rejected (ok=false).
func normalizeNonce(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return "", false
	}
	hexPart := s[2:]
	if len(hexPart) != 64 {
		return "", false
	}
	for _, c := range hexPart {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return "", false
		}
	}
	return "0x" + strings.ToLower(hexPart), true
}

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

	// A malformed nonce is rejected before any recovery or RPC. An empty nonce
	// is valid and selects the legacy 3-field digest.
	nonce, ok := normalizeNonce(req.WalletNonce)
	if !ok {
		return "", ErrWalletAuthInvalid
	}

	addr, err := RecoverSwapRequestSignerWithNonce(
		h.appConfig.ApiServer.WalletAuthChainID,
		h.appConfig.Blockchain.ICYSwapContractAddr,
		req.ICYAmount,
		req.BTCAddress,
		req.WalletDeadline,
		nonce,
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

	// Replay gate: only requests that carry a nonce are protected. The check runs
	// AFTER the ICY-balance and rate-limit gates, so an attacker must already hold
	// ICY and be within the per-wallet rate limit to insert a nonce, which bounds
	// how fast the seen-set can grow (entries also expire at deadline+slack).
	if nonce != "" {
		expiry := time.Unix(req.WalletDeadline, 0).Add(nonceSlack)
		if nonceGuard.checkAndRecord(wallet+":"+nonce, expiry) {
			return "", ErrWalletAuthReplay
		}
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
//
// A non-empty nonce adds a 4th "nonce" (bytes32) field to the SwapRequest and
// carries it in the message; an empty nonce reproduces the legacy 3-field
// payload BYTE-FOR-BYTE (same field set, same order), so a client that signs no
// nonce still recovers exactly as before.
func swapRequestTypedData(
	chainID int64,
	verifyingContract string,
	icyAmount string,
	btcAddress string,
	deadline int64,
	nonce string,
) apitypes.TypedData {
	fields := []apitypes.Type{
		{Name: "icyAmount", Type: "uint256"},
		{Name: "btcAddress", Type: "string"},
		{Name: "deadline", Type: "uint256"},
	}
	message := apitypes.TypedDataMessage{
		"icyAmount":  icyAmount,
		"btcAddress": btcAddress,
		"deadline":   fmt.Sprintf("%d", deadline),
	}
	if nonce != "" {
		fields = append(fields, apitypes.Type{Name: "nonce", Type: "bytes32"})
		message["nonce"] = nonce
	}

	return apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"SwapRequest": fields,
		},
		PrimaryType: "SwapRequest",
		Domain: apitypes.TypedDataDomain{
			Name:              eip712Name,
			Version:           eip712Version,
			ChainId:           (*math.HexOrDecimal256)(big.NewInt(chainID)),
			VerifyingContract: verifyingContract,
		},
		Message: message,
	}
}

// RecoverSwapRequestSigner verifies sig over the legacy 3-field SwapRequest and
// returns the wallet address that produced it. Unchanged digest: this is the
// path a client that signs no nonce takes.
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
	return RecoverSwapRequestSignerWithNonce(chainID, verifyingContract, icyAmount, btcAddress, deadline, "", sig)
}

// RecoverSwapRequestSignerWithNonce is RecoverSwapRequestSigner plus an optional
// bytes32 nonce mixed into the signed payload. An empty nonce yields the legacy
// 3-field digest (identical to RecoverSwapRequestSigner); a non-empty nonce
// yields the 4-field digest. The nonce itself is NOT validated here (the caller
// normalizes/validates its shape); this only binds it into the recovered digest.
func RecoverSwapRequestSignerWithNonce(
	chainID int64,
	verifyingContract string,
	icyAmount string,
	btcAddress string,
	deadline int64,
	nonce string,
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

	typed := swapRequestTypedData(chainID, verifyingContract, icyAmount, btcAddress, deadline, nonce)

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
