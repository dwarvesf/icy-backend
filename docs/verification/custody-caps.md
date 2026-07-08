# Proof of Done, Payout Caps + EIP-712 Signer-Key Isolation (SG-06)

**Outcome:** a key leak or one bad authorization can no longer drain the whole
treasury. BTC payouts are bounded IN CODE by a per-payout cap and a rolling 24h
cap, and the EIP-712 swap-signer key is isolated so it can only produce payout
signatures, never send a transaction, pay gas, or hold ICY.

**Branch:** `fix/custody-caps-signer` (base `develop`).
**Go:** 1.24.2. `go build ./...` exit 0.

Money/security change. The cap logic is verified from `internal/server` (the
settlement orchestrator) and `internal/store/onchainbtcprocessedtransaction` (the
rolling-sum primitive), and the signer isolation from `internal/baserpc`, all of
whose test binaries build. `internal/btcrpc`'s test package is pre-existing
build-broken (config drift, see §6), mirroring how SG-05/11 verified settlement
logic from `internal/server`.

---

## 1. Acceptance criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | **Per-payout cap.** A payout whose sendable amount (subtotal - service fee) exceeds the cap is REFUSED before any broadcast and marked `failed` (terminal; a fixed-size payout can never shrink under the cap). Zero sends. | PASS | orchestrator guard in `telemetry.ProcessPendingBtcTransactions` + `btcrpc.Send` backstop; `TestProcessPending_OverPerPayoutCap_RefusedFailed` |
| 2 | **Rolling daily cap.** A payout that would push the sum of the last 24h of sent payouts over the daily cap is REFUSED and routed to `needs_reconcile` (not `failed`: size is fine, it is a rate limit). Never left `pending` to auto-retry forever. | PASS | orchestrator guard + `store.SumSentInWindow`; `TestProcessPending_DailyCapCrossed_RefusesCrosser`, `TestProcessPending_SequenceCrossesDailyCap_RefusesOnlyCrosser` |
| 3 | **Under both caps proceeds.** A payout within both caps broadcasts once and completes. | PASS | `TestProcessPending_UnderBothCaps_Proceeds`, `TestProcessPending_UnderDailyCap_Proceeds` (negative controls) |
| 4 | **Rolling sum computed from the store.** The 24h total is the summed sendable amount of sent/possibly-sent rows (`broadcasted` / `completed` / `needs_reconcile`) in the window; non-sent states excluded; fails closed on an unparseable amount. | PASS | `store.SumSentInWindow`; `TestSumSentInWindow_*` (5 tests) |
| 5 | **Both caps are config knobs.** Env with safe defaults; `0` disables a cap. | PASS | `Bitcoin.MaxPayoutSatoshi` / `Bitcoin.MaxDailyPayoutSatoshi` (§7) |
| 6 | **Signer-key isolation.** A DISTINCT signer key config is used ONLY by `GenerateSignature`'s signing path. The signing path never initiates a value-moving tx; a distinct key fully isolates it. Falls back to the existing key when unconfigured. | PASS | `Blockchain.SwapSignerPK`, `baserpc.signerWallet`, `resolveSignerPK`; `TestGenerateSignature_SignsWithSignerKeyNotHolder`, `TestResolveSignerPK_IsolatedVsFallback` |
| R | **No regression.** All pre-existing SG-05/07/11/12/13 settlement + dedup + signing tests stay green. | PASS | `internal/baserpc` + `internal/server` + `internal/store/onchainbtcprocessedtransaction`: 62 passed (was 49) |

---

## 2. Implementation

### Caps: where enforced (the chokepoint)

Every real payout passes through `telemetry.ProcessPendingBtcTransactions`, which
claims a `pending` row (`pending -> processing`) and then calls `btcRpc.Send`. The
caps are enforced in that orchestrator, AFTER the atomic claim and the existing
negative-amount guard, and BEFORE `Send`. Because the row is already claimed to
`processing`, refusing it to a terminal state (`failed` / `needs_reconcile`) is
safe: it is NOT sent and NOT left `pending`, so it can never be re-claimed or
auto-retried forever.

- **Per-payout cap** (`Bitcoin.MaxPayoutSatoshi`, satoshi, `0` disables):
  `amtInt > cap` -> mark `failed` (terminal; the amount is fixed and can never
  shrink under the cap), fire the settlement webhook, skip `Send`.
- **Rolling daily cap** (`Bitcoin.MaxDailyPayoutSatoshi`, satoshi, `0` disables):
  `SumSentInWindow(now-24h) + amtInt > cap` -> mark `needs_reconcile` (treasurer
  review; not the payout's fault), fire the webhook, skip `Send`. A `SumSentInWindow`
  ERROR also routes to `needs_reconcile` (fail closed: never send on an unverified
  rolling total).

### `btcrpc.Send` per-payout backstop (defence-in-depth)

`btcrpc.Send` independently refuses `amountToSend > Bitcoin.MaxPayoutSatoshi`
(pre-broadcast, tagged `ErrNotBroadcast` = definitely-not-sent, `0` disables). At
the same threshold the orchestrator already marks such a payout `failed` and never
reaches `Send`, so this branch is inert on the normal path; it exists so ANY direct
caller of `Send` (bypassing the orchestrator) still cannot exceed the per-payout
cap. This is the `internal/btcrpc` "cap enforcement on the send path" layer.

### Rolling sum: `store.SumSentInWindow(db, since)` (`internal/store/onchainbtcprocessedtransaction`)

Sums the sendable amount (`subtotal - service_fee`) of every payout in a **sent
state** whose send timestamp is `>= since`:

- **Counted states** (`sentStates`): `broadcasted`, `completed` (definitely put on
  the wire), and `needs_reconcile` (AMBIGUOUS post-broadcast, or a stuck broadcast,
  the tx MAY be live). Counting `needs_reconcile` makes the daily total a true
  **ceiling** on outflow, over-counting a maybe-sent row is the safe direction for a
  drain-prevention control. `pending` / `processing` / `failed` are excluded (no BTC
  left).
- **Send timestamp** = `processed_at` when set (the broadcast/completion time),
  else `updated_at`. So a `broadcasted`/`completed` row uses its true send time,
  and an ambiguous `needs_reconcile` row that only stamped status (no
  `processed_at`) still counts via `updated_at` (conservative).
- **Fails closed:** a row whose amount string cannot be parsed returns an error
  (the caller then refuses the payout) rather than being silently skipped, which
  would under-count the daily total and weaken the cap. A negative row contributes 0.

### Signer-key isolation (`internal/baserpc`, config)

Before this change one key (`IcySwapSignerPrivateKey`, env
`BLOCKCHAIN_SWAP_SIGNER_PRIVATE_KEY`) did everything: it signed the EIP-712 payload
AND was the wallet wired into the on-chain transactor (`Approve` + `Swap`), so it
paid gas and held ICY. A leak of that one key could both forge signatures AND move
funds.

- **New config** `Blockchain.SwapSignerPK` (env `SWAP_SIGNER_PK`, or a vault-transit
  ref in prod): the DEDICATED EIP-712 signer key.
- **`resolveSignerPK(cfg)`** picks `SwapSignerPK` when set (isolated), else falls
  back to `IcySwapSignerPrivateKey` (pre-provision compatibility). Pure, RPC-free,
  unit-tested.
- **`BaseRPC.signerWallet`** is built from that resolved key in `initClient`. It is
  referenced in EXACTLY ONE place: the `crypto.Sign(digest, b.signerWallet.GetPrivateKey())`
  call inside `GenerateSignature`.
- **`BaseRPC.wallet`** (the holder/gas wallet from `IcySwapSignerPrivateKey`) remains
  the ONLY wallet passed to `bind.NewKeyedTransactorWithChainID` and used as
  `opts.From` in `Swap`.

**Why a value-moving tx cannot use the signer key:** the only way to send a tx here
is to build a keyed transactor, and that is only ever built from `b.wallet`. The
signer key never touches a transactor, never sets `opts.From`, never signs an
`Approve`/`Swap`. So an address provisioned into `SWAP_SIGNER_PK` holds no funds and
cannot pay gas: a leak of it can only forge payout signatures, which the on-chain
contract (nonce, deadline, per-swap ICY deposit verification, the SG-03 oracle rate
gate) still bounds. `initClient` logs the two public addresses + `signer_isolated`
so ops can confirm the split took, and never logs key material.

---

## 3. Run table

See `docs/proof/sg06-run-table.txt` (rendered to `docs/proof/sg06-custody-caps.png`).
Summary:

```
$ go test ./internal/baserpc/... ./internal/server/... \
         ./internal/store/onchainbtcprocessedtransaction/... -count=1
  -> 62 passed in 3 packages   (was 49; +13 new)
```

| Test | Proves | Result |
|---|---|---|
| `server.TestProcessPending_UnderBothCaps_Proceeds` | under both caps -> send x1, completed | PASS |
| `server.TestProcessPending_OverPerPayoutCap_RefusedFailed` | amount > per-payout cap -> send x0, row `failed` | PASS |
| `server.TestProcessPending_DailyCapCrossed_RefusesCrosser` | 900 sent + 900 > 1000 cap -> send x0, `needs_reconcile`; prior row untouched | PASS |
| `server.TestProcessPending_SequenceCrossesDailyCap_RefusesOnlyCrosser` | 3x900 under 2000: first two send, 3rd (-> 2700) refused | PASS |
| `server.TestProcessPending_UnderDailyCap_Proceeds` | same shape, cap raised -> proceeds | PASS |
| `store.TestSumSentInWindow_CountsSentStatesInWindow` | completed + broadcasted in window summed (900 + 1900) | PASS |
| `store.TestSumSentInWindow_ExcludesRowsOutsideWindow` | 25h-old send not counted | PASS |
| `store.TestSumSentInWindow_ExcludesNonSentStates` | pending / processing / failed not counted | PASS |
| `store.TestSumSentInWindow_NeedsReconcileCountedViaUpdatedAt` | ambiguous (no processed_at) counted via updated_at | PASS |
| `store.TestSumSentInWindow_UnparseableAmountFailsClosed` | bad amount -> error (fail closed) | PASS |
| `baserpc.TestResolveSignerPK_IsolatedVsFallback` | SwapSignerPK set -> used; unset -> holder key | PASS |
| `baserpc.TestGenerateSignature_SignsWithSignerKeyNotHolder` | recovered signer == signer key, != holder/gas wallet | PASS |
| `baserpc.TestGenerateSignature_RecoveryRejectsWrongSigner` | recovery harness discriminates (not vacuous) | PASS |

---

## 4. Negative controls

Each guard was temporarily reverted, the relevant test re-run to confirm it FAILS,
then restored (all restored to green):

| Reverted change | Test | Observed failure |
|---|---|---|
| signing path `b.signerWallet` -> `b.wallet` | `TestGenerateSignature_SignsWithSignerKeyNotHolder` | recovered address is the HOLDER, not the signer (`signature signer = 0x3E9E...`) |
| per-payout guard `&&`-disabled | `TestProcessPending_OverPerPayoutCap_RefusedFailed` | over-cap payout sent (`send count = 1, want 0`) |
| daily-cap crossing check `&&`-disabled | `TestProcessPending_DailyCapCrossed_RefusesCrosser`, `...SequenceCrossesDailyCap...` | over-cap payouts sent (`send count = 1`; `3, want 2`) |

Built-in contrast controls (same inputs, cap moved): `TestProcessPending_UnderBothCaps_Proceeds`
and `TestProcessPending_UnderDailyCap_Proceeds` prove the caps do not over-refuse.

---

## 5. Adversarial notes (rung-4: "exceed the cap" / "use the signer to move funds")

- **Exceed the per-payout cap.** Blocked in TWO layers: the orchestrator marks the
  row `failed` before `Send`, and `btcrpc.Send` refuses independently. Both read the
  same `Bitcoin.MaxPayoutSatoshi`.
- **Drain via many payouts.** The rolling-24h sum re-evaluates before EVERY payout,
  including within a single settlement pass (each broadcast in the loop counts toward
  the next row's check, proven by `...SequenceCrossesDailyCap...`). The crossing
  payout is refused.
- **Blind the daily cap.** If `SumSentInWindow` errors (e.g. a corrupt amount), the
  orchestrator fails closed to `needs_reconcile`, it does not send.
- **Use the swap signer key to move funds / pay gas.** The signer key is never wired
  into a transactor (only `b.wallet` is), so it cannot build a tx or pay gas. Proven
  by recovering the on-chain-visible signer from a real `GenerateSignature` output
  and asserting it is the isolated signer, not the holder.

---

## 6. Pre-existing out-of-scope breakage

`internal/btcrpc`'s test package (`btcrpc_multi_endpoint_test.go`,
`caching_improvements_test.go`, `config_test.go`, `blockstream/multi_endpoint_test.go`)
fails to build on `develop` HEAD, unrelated to this change. It references config
fields / constructors that never existed in production: `EndpointRetryDelay`,
`RequestTimeout`, `EndpointTimeout`, `blockstream.NewMultiEndpoint`,
`config.ApiServer` / `config.Bitcoin` (used as if package-level). This branch edits
only the NON-test `internal/btcrpc/btcrpc.go`; the test breakage is byte-identical to
untouched HEAD (no btcrpc test file is in this branch's diff). All cap logic is
therefore verified from `internal/server` + `internal/store/onchainbtcprocessedtransaction`,
and signer isolation from `internal/baserpc`, per the SG-05/11 precedent.

---

## 7. Config keys

| Env key | Config field | Meaning | Default |
|---|---|---|---|
| `BTC_MAX_PAYOUT_SATOSHI` | `Bitcoin.MaxPayoutSatoshi` | Per-payout hard cap in satoshi. Over-cap payout -> `failed`. `0` disables. | `5_000_000` (0.05 BTC) |
| `BTC_MAX_DAILY_PAYOUT_SATOSHI` | `Bitcoin.MaxDailyPayoutSatoshi` | Rolling-24h outflow cap in satoshi. A payout that would cross it -> `needs_reconcile`. `0` disables. | `25_000_000` (0.25 BTC) |
| `SWAP_SIGNER_PK` | `Blockchain.SwapSignerPK` | Dedicated EIP-712 payout-authorization signer key. Used ONLY to sign; never sends a tx / pays gas / holds ICY. Unset -> falls back to the holder key. In prod, provisionable as a vault-transit KV `SWAP_SIGNER_PK`. | (unset) |

Both cap defaults are **SAFE PLACEHOLDERS**, not tuned prod values.

---

## 8. Step-08 deploy notes (to ACT ON at provisioning, not in this PR)

1. **Provision a DISTINCT signer key.** Generate a fresh keypair whose address is
   the on-chain-authorized swap signer, set `SWAP_SIGNER_PK` (env or vault-transit
   KV) to it, and ensure that address is registered as the contract's signer.
   The signer address MUST be different from the holder/gas wallet
   (`IcySwapSignerPrivateKey`) and MUST hold no funds. Confirm via the `initClient`
   log line: `signer_isolated=true` and `holder_address != signer_address`.
2. **Set the caps to real values before the BTC deposit.** The defaults
   (0.05 BTC per-payout / 0.25 BTC daily) are safe placeholders; confirm the real
   ceilings with the treasurer and set `BTC_MAX_PAYOUT_SATOSHI` /
   `BTC_MAX_DAILY_PAYOUT_SATOSHI` accordingly. `0` disables a cap (do not ship
   disabled).

---

## 9. Reproduce

```bash
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH
git checkout fix/custody-caps-signer
go build ./...   # exit 0
# Do NOT `go test ./...` bare. internal/btcrpc's test package is pre-existing
# build-broken (§6); run the touched building packages:
go test ./internal/baserpc/... ./internal/server/... \
        ./internal/store/onchainbtcprocessedtransaction/... -count=1
# -> 62 passed in 3 packages
```

---

## 10. Rollback

Additive change, no migration. Revert the branch merge commit to restore the prior
behaviour (uncapped payouts; the swap-signer key also sends txs / pays gas). No data
migration: the new config keys are optional (safe defaults apply if unset), no new
DB columns or statuses were introduced (the caps reuse the existing `failed` /
`needs_reconcile` states). Any rows the caps parked in `needs_reconcile` remain valid
after rollback and can be reconciled normally.
