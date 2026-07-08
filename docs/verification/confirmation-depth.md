# Proof of Done, Confirmation-Depth Gate + Confirm-Before-Complete + Stuck-Tx Handling (SG-11 / DF-82)

**Outcome:** a BTC payout can only fire after the triggering Base swap event is
buried deep enough that a shallow reorg cannot unwind it, and an outgoing BTC send
is not called "completed" until it is actually confirmed on-chain. A stuck (never
confirming) send is detected and routed for a manual fee-bump, never auto-resent.

**Branch:** `fix/confirmation-depth` (base `develop`).
**Go:** 1.24.2. `go build ./...` exit 0.

Money/security change. Verified from `internal/server` (whose test binary builds),
mirroring how SG-05/07/12 tested settlement logic, because `internal/telemetry`
and `internal/btcrpc` have pre-existing build-broken test files (see §6).

---

## 1. Acceptance criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | **Base-side confirmation gate.** A swap event triggers a payout only when it is buried `>= MinSwapConfirmations` deep relative to the chain tip. An under-confirmed event creates NO payout row (and no swap row), so a reorg that unwinds it leaves nothing. Once it crosses the threshold it IS actioned, exactly once. | PASS | `telemetry.EnoughConfirmations`, `telemetry.ProcessConfirmedSwap`; `TestEnoughConfirmations_Boundary`, `TestProcessConfirmedSwap_UnderThenOverThreshold`, `TestProcessConfirmedSwap_ConfirmedTwice_NoDoubleAction` |
| 1b | **Respects SG-12 dedup.** Once a confirmed swap is actioned it is never re-actioned (no second swap row, no second payout). | PASS | swap dedup + `CreateBtcPayoutForSwap` gates; `TestProcessConfirmedSwap_ConfirmedTwice_NoDoubleAction` |
| 2 | **Confirm-before-complete.** A successful broadcast lands the payout in the intermediate `broadcasted` state, NOT `completed`. It becomes `completed` only once it has `>= MinBtcConfirmations` on-chain. | PASS | `model.BtcProcessingStatusBroadcasted`, `store.UpdateToBroadcasted`, `telemetry.ConfirmBroadcastedBtcTransactions`; `TestProcessPending_BroadcastNotConfirmed_StaysBroadcasted`, `TestProcessPending_BroadcastConfirmed_Completes` |
| 2b | **Crash between broadcast and confirmation loses nothing / completes nothing early.** A row stranded in `broadcasted` is never re-claimed, never re-broadcast, never prematurely completed (respects SG-05's `processing`/`needs_reconcile` machine, does not weaken it). | PASS | `GetPendingTransactions` filters to `pending` only; `TestProcessPending_CrashStrandedBroadcasted_NoResendNoComplete` |
| 3 | **Stuck-tx handling = detect-and-reconcile (no auto-RBF).** A broadcasted send unconfirmed past `StuckTxTimeoutSeconds` is routed to `needs_reconcile` for a MANUAL fee-bump. It is NEVER auto-resent, so no second, independent, competing BTC tx can be created. | PASS | `telemetry.ConfirmBroadcastedBtcTransactions`; `TestConfirmBroadcasted_StuckTx_RoutesNeedsReconcile_NoSecondSend`, `TestConfirmBroadcasted_RecentUnconfirmed_StaysBroadcasted` |
| 3b | **Cannot double-pay.** The stuck path calls `btcRpc.Send` zero times; the original tx hash is preserved for the manual RBF. | PASS | `send count == 0` + hash-preserved assertions in `TestConfirmBroadcasted_StuckTx_RoutesNeedsReconcile_NoSecondSend` |
| R | **No regression.** All pre-existing SG-05/07/12 settlement + dedup tests stay green. | PASS | `internal/server` + `internal/store/onchainbtcprocessedtransaction`: 42 passed (was 28) |

**Auto-RBF vs detect-and-reconcile:** item 3's spec permits either a true
same-input RBF or detect-and-reconcile, and forbids shipping an unsafe second
send. We chose **detect-and-reconcile**. Rationale in §3.

---

## 2. Implementation

### Item 1, Base-side confirmation gate (`internal/telemetry/swap.go`)

- **`EnoughConfirmations(eventBlock, latestBlock, minConfirmations uint64) bool`**,
  pure predicate. Inclusion block counts as the first confirmation:
  `confirmations = latestBlock - eventBlock + 1`. `minConfirmations == 0` disables
  the gate; `latestBlock < eventBlock` (a reorg that shortened the chain) returns
  false (fail-closed).
- **`ProcessConfirmedSwap(tx, swapTx, latestBlock) (bool, error)`**, the
  confirmation-gated per-event action the indexer's `DoInTx` loop now calls. It
  gates FIRST: an under-confirmed event returns `(false, nil)` and persists
  **nothing** (no swap row, no payout). Because the block cursor advances only
  past STORED swaps, a deferred event is simply re-evaluated on a later scan once
  it is deep enough, nothing is lost. A confirmed event is deduped (SG-12), stored,
  then handed to the unchanged `CreateBtcPayoutForSwap`.
- **`IndexIcySwapTransaction`**, the inline per-swap body (dedup + create + payout)
  moved into `ProcessConfirmedSwap`; the loop now counts only `actioned` rows.
- Config knob `Blockchain.MinSwapConfirmations` (default 6).

**Why gate before storing, not just before paying:** the block cursor is derived
from the latest STORED swap's receipt. If we stored an under-confirmed swap now
(advancing the cursor) but deferred only its payout, the payout would never be
created on a later scan (the event is never re-filtered), so it would be lost
forever. Deferring the whole row keeps the cursor behind the un-matured event, so
it is re-scanned and paid once deep enough. Under-confirmed events are always the
highest block numbers in a range, so storing the confirmed (lower-block) ones never
skips a deferred (higher-block) one.

### Item 2, confirm-before-complete (`internal/telemetry/btc.go`, model, store)

- New non-terminal status **`broadcasted`** (`internal/model`): broadcast
  succeeded (tx hash + fee + `processed_at` recorded) but not yet confirmed.
  `GetPendingTransactions` never returns it, so it is never re-claimed / re-sent.
- **`store.UpdateToBroadcasted(id, btcTxHash, networkFee)`** and
  **`store.GetBroadcastedTransactions()`** (added to `IStore`).
- **`ProcessPendingBtcTransactions`**, the broadcast-success path now calls
  `UpdateToBroadcasted` instead of `UpdateToCompleted`, and fires NO terminal
  webhook there (`broadcasted` is not a settled state). The claim gate and the
  ambiguous/`ErrNotBroadcast` routing (SG-05) are untouched.
- **`ConfirmBroadcastedBtcTransactions()`** (new), the confirm-before-complete
  sweep, called at the end of each `ProcessPendingBtcTransactions` tick (no new
  cron wiring). For each `broadcasted` row it queries live confirmations and:
  `>= MinBtcConfirmations` -> `completed` + the SG-07 completed webhook (the only
  path to `completed`); below threshold and older than the stuck timeout -> §3;
  else left `broadcasted`. A transient confirmation-query error leaves the row
  `broadcasted` (retried next tick), never completed/reconciled on an unverified
  count. `MinBtcConfirmations` is floored at 1.

### Confirmation query (`internal/btcrpc`, `internal/btcrpc/blockstream`)

- **`blockstream.GetTransactionConfirmations(txID)`**, reads `/tx/:txid` (status)
  and, if confirmed, `/blocks/tip/height`, returning `tip - blockHeight + 1`.
  Unconfirmed / not-found / tip-behind returns `(0, nil)`; only real RPC/parse
  failures error. Added to `IBlockStream`.
- **`btcrpc.GetTransactionConfirmations(txHash)`**, `withRetry` wrapper (endpoint
  failover), added to `IBtcRpc`; passthrough added to the monitoring
  circuit-breaker wrapper so the CB-wrapped rpc still satisfies `IBtcRpc`.

### Item 3, stuck-tx (`internal/telemetry/btc.go`)

Handled inside `ConfirmBroadcastedBtcTransactions` (see §3). Config knob
`Bitcoin.StuckTxTimeoutSeconds` (default 10800 = 3h).

---

## 3. Why detect-and-reconcile, not auto-RBF

A safe RBF (BIP-125) must rebuild the SAME transaction over the SAME inputs
(UTXOs) with a higher fee, so that only ONE of {original, replacement} can ever
confirm. `btcrpc.Send` re-selects UTXOs freshly on every call and does not persist
the input set of a broadcast tx. An automatic "bump" through the existing send path
would therefore be a second, INDEPENDENT transaction over possibly-different UTXOs,
which could confirm ALONGSIDE the original = a double-pay of real BTC. That is
exactly the unsafe second-send the sub-goal forbids.

A true same-input RBF would require persisting/reconstructing the exact vins of the
broadcast tx and re-signing with an RBF-signalling sequence, and it could not be
verified here because the `internal/btcrpc` test package is build-broken (§6). So
the auto-path is **detect-and-reconcile**: a broadcasted send that has not confirmed
past `StuckTxTimeoutSeconds` is flagged (Discord webhook, SG-07) and routed to the
terminal `needs_reconcile` state, where the treasurer performs the fee-bump/replace
manually against the preserved original tx. This trades liveness (a stuck row waits
for a human) for absolute safety (a double-pay is impossible), the correct direction
for money, and it matches the existing SG-05 `needs_reconcile` model.

`TestConfirmBroadcasted_StuckTx_RoutesNeedsReconcile_NoSecondSend` proves the
handler issues **zero** additional `Send` calls and preserves the original tx hash.

---

## 4. Run table

`go build ./...` exit 0. `go test ./internal/server/... ./internal/store/onchainbtcprocessedtransaction/... -count=1` -> **42 passed in 2 packages**.

| # | Test | Proves | Result |
|---|------|--------|--------|
| 1 | `TestEnoughConfirmations_Boundary` | 5 confs < N=6 not enough; exactly 6 enough; reorg-shortened chain fail-closed | PASS |
| 1 | `TestEnoughConfirmations_ZeroDisablesGate` | N=0 turns the gate off | PASS |
| 1 | `TestProcessConfirmedSwap_UnderThenOverThreshold` | under N: 0 payout + 0 swap rows; at N: exactly 1 of each | PASS |
| 1 | `TestProcessConfirmedSwap_ConfirmedTwice_NoDoubleAction` | dedup holds under the gate | PASS |
| 2 | `TestProcessPending_BroadcastNotConfirmed_StaysBroadcasted` | 0-conf broadcast stays `broadcasted`, completes once confirmed, send count 1 | PASS |
| 2 | `TestProcessPending_BroadcastConfirmed_Completes` (neg ctrl) | confirmed -> completes in one tick | PASS |
| 2 | `TestProcessPending_CrashStrandedBroadcasted_NoResendNoComplete` | stranded `broadcasted` row not lost / not re-sent / not completed | PASS |
| 3 | `TestConfirmBroadcasted_StuckTx_RoutesNeedsReconcile_NoSecondSend` | stuck -> `needs_reconcile`, **0** second sends, orig hash kept | PASS |
| 3 | `TestConfirmBroadcasted_RecentUnconfirmed_StaysBroadcasted` (neg ctrl) | recent unconfirmed not mistaken for stuck | PASS |
| R | SG-05/07/12 suite (settlement + webhook + dedup + store) | no regression, 28 pre-existing still green | PASS |

Visual proof: `docs/proof/sg11-confirmation-depth.png` (source `docs/proof/sg11-run-table.txt`).

---

## 5. Negative controls

- **Confirmation gate (item 1):** the SAME event is under-confirmed (tip = block+4,
  5 confs) -> 0 payout AND 0 swap rows; then over-confirmed (tip = block+5, 6
  confs) -> exactly 1 of each. The under leg is the control for the over leg.
- **Confirm-before-complete (item 2):** `TestProcessPending_BroadcastConfirmed_Completes`
  is the confirmed-mock control for `..._BroadcastNotConfirmed_StaysBroadcasted`,
  same shape, confirmed vs unconfirmed, completed vs broadcasted.
- **Stuck-tx (item 3):** `TestConfirmBroadcasted_RecentUnconfirmed_StaysBroadcasted`
  is the not-yet-stuck control for `..._StuckTx_RoutesNeedsReconcile...`, a recent
  unconfirmed tx is left broadcasted, proving the timeout, not merely "unconfirmed",
  is what routes to reconcile.
- **Double-pay control:** the stuck test asserts `send count == 0`. Reverting the
  handler to auto-resend through `btcRpc.Send` would make this count >= 1, the
  double-pay this sub-goal forbids.

---

## 6. Pre-existing out-of-scope breakage

`internal/telemetry` and `internal/btcrpc` (and `internal/btcrpc/blockstream`)
carry build-broken test files on `develop` HEAD, unrelated to this change. They
reference config fields / constructors that never existed in production:
`config.BitcoinConfig.EndpointRetryDelay`, `RequestTimeout`, `EndpointTimeout`,
`EndpointLoadBalancing`, `CircuitBreakerTimeout`, `blockstream.NewMultiEndpoint`,
`client.GetEndpointHealth`, `config.ApiServer`, `config.Bitcoin`,
`generateLargeBTCResponse` used as a const. Confirmed identical to untouched HEAD
(none of those files are in this branch's diff). None reference this change's new
symbols (`GetTransactionConfirmations`, `MinBtcConfirmations`,
`MinSwapConfirmations`, `StuckTxTimeoutSeconds`, `broadcasted`,
`UpdateToBroadcasted`). All new logic is therefore tested from `internal/server`,
per the SG-05/07/12 precedent.

---

## 7. Config keys

| Env key | Config field | Meaning | Default |
|---|---|---|---|
| `BLOCKCHAIN_MIN_SWAP_CONFIRMATIONS` | `Blockchain.MinSwapConfirmations` | Base confirmations a swap event needs before it triggers a payout (item 1). 0 disables the gate. | `6` |
| `BTC_MIN_CONFIRMATIONS` | `Bitcoin.MinBtcConfirmations` | On-chain confirmations an outgoing BTC payout needs before it is marked completed (item 2). Floored at 1. | `1` |
| `BTC_STUCK_TX_TIMEOUT_SECONDS` | `Bitcoin.StuckTxTimeoutSeconds` | How long a broadcasted-but-unconfirmed payout may sit before it is routed to `needs_reconcile` for a manual RBF (item 3). | `10800` (3h) |

---

## 8. Reproduce

```bash
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH
git checkout fix/confirmation-depth
go build ./...   # exit 0
# Do NOT `go test ./...` (mainnet RPC hangs); run the touched building packages:
go test ./internal/server/... ./internal/store/onchainbtcprocessedtransaction/... -count=1
# -> 42 passed in 2 packages
```

---

## 9. Rollback

Pure additive change, no migration. Revert the branch merge commit to restore the
prior behavior (swap events actioned at zero confirmations; outgoing payouts marked
`completed` immediately on broadcast). No data migration is needed: the new
`broadcasted` status is a string value in the existing status column; on rollback,
any rows sitting in `broadcasted` should be manually reconciled (checked on-chain,
then set to `completed` or `needs_reconcile`) since the pre-rollback code does not
recognize that state. New env keys are optional (safe defaults apply if unset).
