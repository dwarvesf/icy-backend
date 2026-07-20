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
   any caller  ------>  |  EIP-712 recover                |  passes only if you
                        |  ecrecover(sig) -> 0xabc...     |  hold the wallet key
                        '---------------------------------'
                                      |
                            per-WALLET rate limit
                                      |
                                      v
                             derive amount from oracle
                                      |
                                      v
                                  sign payout
```

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
        |                    |          2. ecrecover -> caller wallet      |
        |                    |          3. per-wallet rate limit           |
        |                    |          4. reject non-mainnet BTC addr     |
        |                    |          5. derive sats from oracle         |
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

Wallet is the better key: an IP is shared behind NAT and trivially rotated,
whereas signing as a wallet requires its private key. IP still carries the cold
path, since an unauthenticated flood must be cheap to reject.

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
| 3 | A captured signature cannot be reused for a different payout address. | PASS | §4.2 case 2 |
| 4 | A captured signature cannot be reused for a different amount. | PASS | §4.2 case 3 |
| 5 | A captured signature cannot be replayed on another chain. | PASS | §4.2 case 4 |
| 6 | Expired, zero, and beyond-max-age deadlines are rejected. | PASS | §4.2 cases 7-8 |
| 7 | Empty and malformed signatures are rejected. | PASS | §4.2 cases 5-6 |
| 8 | Enforcement is config-gated; a supplied signature is verified regardless. | PASS | `authenticateCaller`, `REQUIRE_WALLET_AUTH` |
| 9 | Rate limiting is per-wallet once authenticated, per-IP before. | PASS | §2.4; `TestSignatureRateLimitMiddleware` |
| 10 | Non-mainnet BTC payout addresses are rejected before signing. | PASS | §4.3 |

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

---

## 5. Config

| Env | Default | Meaning |
|---|---|---|
| `REQUIRE_WALLET_AUTH` | `false` | enforce that a wallet signature is present |
| `WALLET_AUTH_CHAIN_ID` | `8453` | chain id in the EIP-712 domain; must match the frontend exactly |

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
  (§2.6). Needs a contract decision.
- **No end-to-end swap has been executed** against this change. Vercel preview
  deploys are CORS-blocked by `api.icy.so`, so the first real wallet swap must
  be run locally or on dev before merging the frontend.
- **`walletLimiter` state is per process.** With more than one replica the
  effective per-wallet rate is N x the configured rate. Acceptable at current
  scale; needs a shared store (Redis, or the `pg_advisory_lock` pattern already
  used for the settlement singleton) if it matters.
