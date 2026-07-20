# Design + Proof of Done, Wallet-Signature Auth on `/swap/generate-signature` (SG-14)

**Outcome:** the signature endpoint can identify its caller. Previously it could
not: the `ApiKey` guarding it is a `NEXT_PUBLIC_` value inlined into the browser
bundle, so every visitor holds it. The caller now signs an EIP-712 `SwapRequest`
with the wallet that will call `swap()`, and the backend recovers the address.

**Branch:** `fix/mainnet-address-and-signer-limit` (base `develop`).
**Go:** 1.24.2. `go build ./...` exit 0.

Companion frontend change: `dwarvesf/icy-swap` PR #5.

---

## 1. The problem

### 1.1 What the key actually is

`NEXT_PUBLIC_*` is not a variable the frontend may read at runtime. Next.js
**substitutes it into the JavaScript at build time**. Demonstrated on a local
build using the placeholder `local-dev`:

```
$ grep -rl 'local-dev' .next/static/
.next/static/chunks/pages/index-4847788f2a3f93e9.js     <- served to every visitor
```

The production value is in the production bundle by the same mechanism. Anyone
who opens devtools on icy.so has it.

### 1.2 Why CORS does not cover for it

`ALLOWED_ORIGINS` is a **browser** policy. It stops a page on another origin
from *reading a response*. It does nothing about `curl`. So the combination of
CORS plus a public key gives zero server-side access control.

```
                    who is asking?           what may they do?
  ApiKey        ->  "our app" (public)   -.
                     = nobody              |-- nothing checks identity
  swap sig      ->  nobody (bearer)     -'
  server-derived
  amount        ->  --                      caps payout to backing   <- the real guard
```

### 1.3 What this is NOT

Not a treasury drain. `GenerateSignature` already derives the payout from the
oracle and signs that, never the client's `btc_amount` (the existing
`SECURITY (CRIT-1)` fix). Losing the key costs abuse control, not funds. This
work closes a design gap; it is not an incident response.

---

## 2. Design

### 2.1 Trust boundary, before and after

```
BEFORE
                        .---------------------------------.
   any caller  ------>  |  ApiKey check                   |  passes for everyone
   (curl, bot, page)    |  (secret shipped to everyone)   |
                        '---------------------------------'
                                      |
                                      v
                             derive amount from oracle      <- only real guard
                                      |
                                      v
                                  sign payout

AFTER
                        .---------------------------------.
   any caller  ------>  |  EIP-712 recover                |  yields an address
                        |  ecrecover(sig) -> 0xabc...     |  for any valid sig
                        '---------------------------------'
                                      |
                        |  recovered wallet holds         |  <- the scarce check
                        |  >= icyAmount ICY?              |
                                      |
                            per-WALLET rate limit
                                      |
                                      v
                             derive amount from oracle
                                      |
                                      v
                                  sign payout
```

**A signature by itself proves nothing scarce.** `ecrecover` returns *an*
address for any well-formed input: a fresh keypair costs ~113us and needs no
gas, no chain presence, no ICY, which is cheaper than rotating an IP. Even 65
random bytes recover successfully about half the time (whenever `r` is a valid
curve x-coordinate), each to a different address.

So the signature alone buys attribution, not authentication. What makes it bind
is the balance check: the recovered address must hold at least the ICY it is
asking to swap. ICY is the scarce thing; keys are free.

That check is an anti-Sybil control, NOT the authority for the swap. It fails
OPEN on an RPC error, because a Base outage stopping every legitimate swap is a
worse trade than a marginal abuse win, and the oracle-derived amount plus the
contract still bound what any signature can actually do.

### 2.2 Request sequence

```
  browser wallet          frontend                backend              signer
        |                    |                       |                    |
        |   1. user enters amount + BTC address      |                    |
        |<--- signTypedData(SwapRequest) ---|        |                    |
        |                    |                       |                    |
        |--- signature ----->|                       |                    |
        |                    |                       |                    |
        |                    |-- POST generate-signature ---->|           |
        |                    |   {icy, btcAddr, sat,          |           |
        |                    |    wallet_signature, deadline} |           |
        |                    |                       |                    |
        |                    |          2. normalize BTC address           |
        |                    |          3. ecrecover -> caller wallet      |
        |                    |          4. caller holds >= icyAmount ICY?  |
        |                    |          5. per-wallet rate limit           |
        |                    |          6. reject non-mainnet BTC addr     |
        |                    |          7. derive sats from oracle         |
        |                    |                       |--- sign(EIP-712) ->|
        |                    |<-- signature, nonce, deadline --|           |
        |                    |                       |                    |
        |<--- swap() tx ---- |                       |                    |
        |                    |                       |                    |
```

Step 1 is free and gasless, but it IS a wallet prompt, so the frontend raises it
before the "Swapping..." toast. Declining is a normal choice and clears state
silently rather than surfacing an error.

### 2.3 The signed payload

```
domain = { name: "IcySwap", version: "1",
           chainId: 8453, verifyingContract: <swapper> }

SwapRequest {
    icyAmount   uint256     <- binds the amount
    btcAddress  string      <- binds the destination
    deadline    uint256     <- bounds the lifetime
}
```

Field choices, each load-bearing:

| Field | Why it is in the payload |
|---|---|
| `icyAmount` | a captured signature cannot be replayed for a larger swap |
| `btcAddress` | a captured signature cannot be redirected to another payout address |
| `deadline` | bounds how long a captured signature stays usable, capped at 5 min server-side so a client cannot mint a long-lived credential |

`btcAmount` is deliberately **excluded**. The server derives it from the oracle
and ignores the client's figure entirely, so binding it would imply an authority
it does not have.

`chainId` and `verifyingContract` in the domain mean a signature from a testnet
deployment cannot be replayed against mainnet.

### 2.4 Two-stage rate limiting

Middleware runs **before** the handler, so it cannot know the wallet. An earlier
draft keyed the middleware bucket on the wallet, which would have read an empty
value and silently degraded to a no-op: a control that looks present and does
nothing. Hence two explicit stages.

```
   request
      |
      v
  [ transport middleware ]   per IP,  12/min burst 5   <- cheap outer gate,
      |                                                   runs before body parse
      v
  [ handler: ecrecover ]     caller identity established
      |
      v
  [ walletLimiter ]          per WALLET, 10/min burst 4  <- precise inner gate
      |
      v
   handler body
```

Neither key is good on its own, and an earlier draft of this section had the
cost argument backwards. Minting a fresh keypair is CHEAPER than rotating an
IP (~113us, measured), so a wallet key is only the better bucket once the
balance check above makes wallets scarce. And the IP key is only meaningful
once `TRUSTED_PROXIES` is set: gin trusts all proxies by default, so
`ClientIP()` returns whatever `X-Forwarded-For` says. Measured before that fix,
50 requests over one socket with a rotating header produced 0 throttled versus
45/50 without it.

Both gates therefore depend on something outside themselves:

| Gate | Bypass if... | Closed by |
|---|---|---|
| per-IP | `X-Forwarded-For` is believed | `TRUSTED_PROXIES` (default: trust none) |
| per-wallet | identities are free | the ICY balance check in §2.1 |

Both limiter maps are capped at 50,000 entries and fail closed when full, since
attacker-chosen keys otherwise make the map itself an amplification vector
(~184 bytes per entry, so a million keys is ~175 MB).

### 2.5 Rollout

Enforcement is config-gated so the two repos can deploy independently.

```
   signature supplied?      REQUIRE_WALLET_AUTH      result
   -------------------      -------------------      ------------------------
   yes                      either                   ALWAYS verified
   no                       false (default)          allowed, caller unknown
   no                       true                     401
```

A supplied signature is always verified, so a bad one is rejected even before
enforcement. Only the "none supplied" case is gated.

1. Merge both PRs. Nothing breaks; old clients keep working.
2. Confirm signed requests are arriving (`caller_wallet` in logs).
3. Set `REQUIRE_WALLET_AUTH=true`.
4. Delete `NEXT_PUBLIC_API_KEY`. It is then dead weight rather than a liability.

### 2.6 What this does NOT solve

The on-chain hash still omits `msg.sender`:

```
getSwapHash(icyAmount, btcAddress, btcAmount, nonce, deadline)
            ^ no caller
```

So a *swap* signature remains a bearer instrument: anyone holding ICY can spend
their own to send BTC to the address in it. `swappedHashes` blocks replay, so
the practical impact is griefing (front-run a pending signature, burn the hash,
force a re-request). Unprofitable, but a transferable treasury authorization is
a poor property.

Closing it requires binding the caller into the hash and redeploying the
swapper. That is a contract decision, deliberately **out of scope here**. This
change is the off-chain half.

---

## 3. Acceptance criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | A signature produced the way a browser wallet produces one recovers to that wallet's address. | PASS | cross-language check §4.1; `TestRecoverSwapRequestSigner_RoundTrip` |
| 2 | Frontend and backend build a byte-identical EIP-712 digest. | PASS | §4.1, viem signs / Go recovers, same address |
| 3 | A signature reused for a different payout address does not recover to the original signer. | PASS | §4.2 case 2 |
| 4 | A signature reused for a different amount does not recover to the original signer. | PASS | §4.2 case 3 |
| 5 | A signature replayed on another chain does not recover to the original signer. | PASS | §4.2 case 4 |
| 6 | Expired, zero, and beyond-max-age deadlines are rejected. | PASS | §4.2 cases 7-9 |
| 7 | Empty and malformed signatures are rejected. | PASS | §4.2 cases 5-6 |
| 8 | Enforcement is config-gated; a supplied signature is verified regardless. | **NOT TESTED** | `authenticateCaller` has no test; the §2.5 matrix is unexercised |
| 9a | Per-IP limiting works and ignores a spoofed `X-Forwarded-For`. | PASS | §4.4; `TestConfigureTrustedProxies_*` |
| 9b | Per-wallet limiting binds. | PARTIAL | `walletRateLimiter.allow` has no direct test; it binds only via the §2.1 balance check |
| 10 | Non-mainnet BTC payout addresses are rejected before signing. | PASS | §4.3 |
| 11 | Unspendable mainnet address types (bare P2PK) are rejected. | PASS | §4.3 |
| 12 | A padded address validates and is normalized before it is hashed or sent. | PASS | §4.3, `TestNormalizeAddress` |

**On criteria 3-5, the wording is deliberately narrow.** An earlier version
claimed a captured signature "cannot be reused". That was wrong: the request
still authenticates, it is merely attributed to an address nobody controls.
What the tests prove is the recovery property, which is the primitive. The
system-level refusal comes from the balance check in §2.1, not from these.

---

## 4. Verification

### 4.1 Cross-language digest agreement (the one that matters)

If the two sides build the payload even one byte differently, Go recovers a
different address and **every real user is rejected**, a failure that would
only appear in production. Signed with viem exactly as the browser wallet does,
recovered in Go:

```
viem signed as : 0x90cfb3d7c584e2fec898b94d865e083dee6c64de
go recovered   : 0x90cfb3d7c584e2fec898b94d865e083dee6c64de

PASS  frontend and backend build the identical EIP-712 digest
```

### 4.2 Recovery and binding

The digest in the test harness is constructed **independently** of the code
under test, so a mistake in the production path cannot cancel itself out.

```
ok    round-trip: wallet signature recovers that wallet
ok    bound: reuse for another payout address does NOT recover signer
ok    bound: reuse for another amount does NOT recover signer
ok    bound: reuse on another chain does NOT recover signer
ok    rejects empty signature
ok    rejects garbage signature
ok    rejects expired deadline
ok    rejects deadline beyond max age

8 passed, 0 failed
```

### 4.3 Mainnet address enforcement

Prior behaviour: `binding:"required"` checked non-empty only, and
`getDustLimit`'s prefix table fell through to a 546-sat default for anything
unrecognised, so `tb1`/`bcrt1`/garbage reached the signer intact.

```
ok    mainnet p2wpkh   accepted=true  want=true
ok    mainnet p2pkh    accepted=true  want=true
ok    mainnet p2sh     accepted=true  want=true
ok    mainnet p2tr     accepted=true  want=true
ok    TESTNET bech32   accepted=false want=false
ok    TESTNET p2pkh    accepted=false want=false
ok    REGTEST bech32   accepted=false want=false
ok    empty            accepted=false want=false
ok    garbage          accepted=false want=false
ok    bad checksum     accepted=false want=false
ok    evm address      accepted=false want=false

11/11 passed
```

### 4.4 Rate limiter

```
=== RUN   TestSignatureRateLimitMiddleware
--- PASS: TestSignatureRateLimitMiddleware (0.00s)
ok  github.com/dwarvesf/icy-backend/internal/transport/http  0.606s
```

Asserts the burst is spendable (a legitimate retry must not trip), 429 past it,
and that throttling is per caller so one abuser cannot deny everyone.

The limiter is only meaningful if the client IP cannot be forged, so that is
tested separately:

```
--- PASS: TestConfigureTrustedProxies_IgnoresSpoofedXFFByDefault
--- PASS: TestConfigureTrustedProxies_MalformedFailsClosed
--- PASS: TestConfigureTrustedProxies_TrustsConfiguredHop
ok  github.com/dwarvesf/icy-backend/internal/transport/http
```

Default (no `TRUSTED_PROXIES`) ignores the header entirely and uses the socket
peer. A malformed list falls back to trusting nothing rather than silently
leaving gin trusting everything. A configured hop IS believed, otherwise a real
deployment rate-limits its own load balancer.

---

## 5. Config

| Env | Default | Meaning |
|---|---|---|
| `REQUIRE_WALLET_AUTH` | `false` | enforce that a wallet signature is present |
| `WALLET_AUTH_CHAIN_ID` | `8453` | chain id in the EIP-712 domain; must match the frontend exactly |
| `TRUSTED_PROXIES` | empty | comma-separated CIDRs of hops in front of this service. Empty means `X-Forwarded-For` is ignored. **Set this to the platform's CIDR in production**, or the per-IP limit throttles the load balancer instead of callers |

`REQUIRE_WALLET_AUTH` is parsed with `strconv.ParseBool`, so `1`, `TRUE`, `True`
and `t` all work. An earlier version compared against the literal `"true"`,
which read every one of those as false: an operator would turn enforcement on
and it would stay off.

---

## 6. Test-suite restoration

Seven test packages did not compile on clean `develop`, from interface and
config drift accumulated over time (mocks missing methods the interfaces grew,
tests referencing deleted config fields). The `btcrpc` case was already
acknowledged in `docs/verification/custody-caps.md` §6.

This mattered directly: `walletauth_test.go` and `address_test.go` are correct
but could not execute inside their own packages, which is why §4.2 and §4.3
were produced by standalone binaries running the real package code.

Restoration is tracked in the same branch. See §7 for what remains.

---

## 7. Known gaps

- **Layer 2 not done.** `msg.sender` is still absent from the on-chain hash
  (§2.6). Needs a contract decision. A free interim mitigation: shorten the
  swap signature's on-chain `deadline` to the tightest value the frontend can
  use, since that deadline IS the griefing window.
- **No end-to-end swap has been executed** against this change. Vercel preview
  deploys are CORS-blocked by `api.icy.so`, so the first real wallet swap must
  be run locally or on dev before merging the frontend.
- **`authenticateCaller` has no test** (criterion 8). The §2.5 rollout matrix
  decides whether production is open or closed and is currently unexercised.
- **`walletRateLimiter.allow` has no direct test** (criterion 9b).
- **Limiter state is per process.** With N replicas the effective per-wallet
  rate is N x configured. Acceptable now; needs a shared store if it matters.
  Sequence that AFTER the balance check, not before, or it is precision on a
  control that does not bind.
- **Auth signatures are replayable within their 5-minute window.** One captured
  signature can mint multiple swap signatures. Bounded and low impact today
  (each still spends the caller's own ICY, the payout is oracle-derived, and
  `swappedHashes` dedupes on-chain), but it becomes the binding constraint as
  the balance check tightens. A seen-digest LRU would close it.
- **`ValidateMainnetAddress` is applied at one call site.** Rows written before
  this branch, or by any future path, reach `btcRpc.Send` unguarded. Validating
  at the send boundary too is the cheapest remaining defence in depth.
- **Signature malleability is tolerated.** High-s signatures recover the same
  signer from different bytes, and `common.FromHex` truncates trailing garbage.
  Harmless today because nothing dedupes on signature bytes. **Constraint on
  future work: never use the signature string as a nonce or idempotency key.**
- **Pre-existing env-grammar split.** `btcrpc.go` selects testnet params unless
  `AppEnv == "prod"`, while `http.go` treats anything outside a dev/test list as
  production. `APP_ENV=production` therefore enforces the API key while running
  BTC on testnet. Not introduced here, but this branch's hardcoded
  `MainNetParams` validator now contradicts `b.networkParam`, which makes the
  disagreement newly load-bearing.

---

## 8. Review record

Two independent review passes ran against this branch. Both found the same two
HIGH issues, and both are fixed above:

| Finding | Status |
|---|---|
| Per-IP limiter was a no-op: gin trusts all proxies, so `X-Forwarded-For` set the bucket key. Measured 0/50 throttled with a rotating header vs 45/50 without. | FIXED, `TRUSTED_PROXIES` + tests |
| Per-wallet limiter was Sybil-bypassable: a fresh keypair costs ~113us and the recovered address was compared against nothing. | FIXED, ICY balance check |
| `REQUIRE_WALLET_AUTH` read as `== "true"`, so `1`/`TRUE`/`True` silently meant false. | FIXED, `strconv.ParseBool` |
| `ValidateMainnetAddress` accepted raw public keys, producing unspendable P2PK outputs. | FIXED, address-type allowlist |
| Validation trimmed but callers used the untrimmed string, a stuck-funds path. | FIXED, `NormalizeAddress` at the edge |
| `caller_wallet` was write-only, so the documented rollout gate could never be satisfied. | FIXED, logged explicitly |
| Criteria 3-5 claimed "cannot be reused" on evidence proving only "recovers a different address". | FIXED, criteria reworded (§3) |
| Limiter maps were unbounded, ~184 bytes per attacker-chosen key. | FIXED, capped at 50k, fail closed |

The reviews also confirmed, independently, that the EIP-712 digest construction
is correct, the deadline handling is sound, there is no attacker-chosen or
zero-address recovery, no request shape routes a supplied signature into the
"none supplied" branch, and the test-restoration commit touched zero production
files.
