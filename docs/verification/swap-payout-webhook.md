# Proof of Done, Durable Discord Webhook on BTC Payout Settlement (SG-07)

**Outcome:** when a BTC payout reaches a terminal state (`completed`, `failed`,
or the SG-05 `needs_reconcile` state), the backend posts a Discord webhook
carrying the swap fields (ICY amount, BTC amount, destination address, tx
hash, status). This is detection, not prevention: today the only signal is an
in-page browser toast; a drain or anomaly on the settlement path is now
visible without someone having the page open.

**Branch:** `feat/swap-payout-webhook` (base `develop`).
**Go:** 1.24.2. `go build ./...` exit 0.

---

## 1. Acceptance criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | A payout reaching a terminal state (`completed`, `failed`, `needs_reconcile`) in `ProcessPendingBtcTransactions` emits a Discord webhook with the swap fields (ICY amount, BTC amount, address, tx hash, status). | PASS | `internal/telemetry/btc.go` `emitSwapPayoutWebhook` + 4 call sites; `TestProcessPending_Completed_FiresPayoutWebhook`, `TestProcessPending_UnpayableRow_FiresFailedWebhook`, `TestProcessPending_NegativeAmount_FiresFailedWebhook`, `TestProcessPending_AmbiguousError_FiresNeedsReconcileWebhook` |
| 2 | Webhook URL is a secret, read via the existing Vault/config mechanism, never hardcoded. Missing URL degrades gracefully (skip, no crash). | PASS | `internal/utils/config/config.go` `AppConfig.SwapPayoutWebhookURL` (env `SWAP_PAYOUT_WEBHOOK_URL`, vault key `SWAP_PAYOUT_WEBHOOK_URL`); `TestProcessPending_NoWebhookURLConfigured_SettlesWithoutNotifying`, `TestCallSwapPayoutWebhook_EmptyURL_NoRequest` |
| 3 | Emitting never breaks settlement: a webhook failure is logged, never blocks or errors the payout state transition (fire-and-forget with error logging). | PASS | detached goroutine + bounded context in `emitSwapPayoutWebhook`; `webhook.CallSwapPayoutWebhook` has no error return (all failures logged internally, mirrors `CallUptimeWebhook`); `TestCallSwapPayoutWebhook_UnreachableEndpoint_DoesNotPanic` |
| 4 | A test proves the webhook fires with the expected payload on a settled event, success and failure. | PASS | §3 run-table, 11 new tests across `internal/utils/webhook` and `internal/server` |
| 5 | Non-terminal transitions (release-to-pending retry) do NOT fire a webhook. | PASS | `TestProcessPending_ReleasedToPending_NoWebhook` |

---

## 2. Implementation

**Reused the existing webhook util, added one method, no new subsystem.**
`internal/utils/webhook/webhook.go` already had `Client` (a thin
`http.Client` wrapper) and `CallUptimeWebhook` (GET, no body, used only for
job-health pings from `internal/monitoring`). Added:

- `SwapPayoutEvent` struct: `Status`, `IcyAmount`, `BtcAmount`, `BtcAddress`,
  `BtcTxHash`.
- `CallSwapPayoutWebhook(ctx, webhookURL, event)`: POSTs a Discord-formatted
  JSON body (`{"content": "..."}`) with the swap fields. Same contract as
  `CallUptimeWebhook`: no error return, every failure (marshal, request,
  transport) is logged and swallowed inside the client.

**Config: one new field, same mechanism as every other webhook URL.**
`internal/utils/config/config.go` `AppConfig.SwapPayoutWebhookURL`, sourced
from `os.Getenv("SWAP_PAYOUT_WEBHOOK_URL")` locally and `vc.GetKV("SWAP_PAYOUT_WEBHOOK_URL")`
from Vault in non-local environments, identical pattern to the four existing
`UptimeWebhooks.*URL` fields just above it.

**Settlement wiring: `internal/telemetry/btc.go` only.**

- `emitSwapPayoutWebhook(client, pendingTx, status, btcAmount, btcTxHash)`: a
  new unexported method. No-ops if `SwapPayoutWebhookURL` is empty. Best-effort
  enriches the ICY amount via `t.store.OnchainIcySwapTransaction.GetByTransactionHash`
  (the field does not live on `OnchainBtcProcessedTransaction` itself; a lookup
  failure is logged and the field left empty, never blocks). Fires the actual
  HTTP call in a **detached goroutine** with its own `10s` bounded context
  (`swapPayoutWebhookTimeout`), decoupled from the settlement loop's own
  control flow, mirrors the existing fire-and-forget pattern in
  `internal/monitoring/job_monitoring.go`'s `CallUptimeWebhook` call.
- `ProcessPendingBtcTransactions` constructs one `webhook.Client` per batch
  (`webhook.New(t.logger)`, stateless, cheap to reuse) and calls
  `emitSwapPayoutWebhook` at the four terminal transitions:
  1. unpayable row -> `failed` (amount = `pendingTx.Subtotal`, no tx hash)
  2. negative-amount guard -> `failed` (amount = the computed `amount.Value`, no tx hash)
  3. ambiguous `Send` error -> `needs_reconcile` (amount = `amount.Value`, no tx hash)
  4. successful broadcast -> `completed` (amount = `amount.Value`, tx hash = the broadcast tx)
- The **release-to-pending** branch (a `btcrpc.ErrNotBroadcast` error, retried
  on the next cron tick) is deliberately **not** wired: it is not a terminal
  state, so there is nothing settled yet to notify about.
- `amount.Value` (the already-fee-adjusted `Web3BigInt` computed earlier in
  the function for the actual `Send` call) is passed in rather than reading
  `pendingTx.Total` off the row, so the notified amount is always the exact
  figure that was (or would have been) paid, not a possibly-stale column.

**No touches to `swap.go` or the claim/classification logic** beyond the four
one-line `emitSwapPayoutWebhook` calls added at existing terminal-transition
sites; the atomic claim, the negative-amount guard, and the
ambiguous-vs-not-broadcast classification (all SG-05) are untouched.

---

## 3. Confirmation run-table

Environment: `export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH`
(go1.24.2). `go test ./...` is NOT run bare (mainnet RPC hangs). `internal/telemetry`
carries a PRE-EXISTING broken test package, confirmed identical on untouched
`develop` via `git stash` (see §6); tests for the settlement orchestrator instead
live in `internal/server` (the composition root), mirroring SG-05's approach.

| Command | Exit | Result |
|---------|------|--------|
| `go build ./...` | 0 | builds |
| `go vet ./internal/utils/webhook/... ./internal/server/...` | 0 | clean |
| `go test ./internal/telemetry/... ./internal/utils/webhook/... ./internal/server/... -count=1` | 1 (telemetry build-failed only, pre-existing) | `internal/utils/webhook` ok, `internal/server` ok |
| `go test ./internal/utils/webhook/... ./internal/server/... -race -count=1` | 0 | ok, race-clean |

Per-test (11 new for SG-07, PASS):

```
PASS  webhook  TestCallSwapPayoutWebhook_Completed_PostsExpectedPayload
PASS  webhook  TestCallSwapPayoutWebhook_Failed_PostsExpectedPayload
PASS  webhook  TestCallSwapPayoutWebhook_NeedsReconcile_PostsExpectedPayload
PASS  webhook  TestCallSwapPayoutWebhook_EmptyURL_NoRequest
PASS  webhook  TestCallSwapPayoutWebhook_UnreachableEndpoint_DoesNotPanic
PASS  server   TestProcessPending_Completed_FiresPayoutWebhook
PASS  server   TestProcessPending_UnpayableRow_FiresFailedWebhook
PASS  server   TestProcessPending_NegativeAmount_FiresFailedWebhook
PASS  server   TestProcessPending_AmbiguousError_FiresNeedsReconcileWebhook
PASS  server   TestProcessPending_ReleasedToPending_NoWebhook
PASS  server   TestProcessPending_NoWebhookURLConfigured_SettlesWithoutNotifying
```

Plus all 13 pre-existing `internal/server` settlement tests (SG-03/04/05) still
green, unaffected by the addition (see full transcript in `docs/proof/sg07-payout-webhook.png`).

`internal/server`'s tests capture the fire-and-forget goroutine's POST body on
a buffered channel (`captureWebhookServer`) and assert on it with a bounded
`select`/timeout rather than a fixed sleep, so they are deterministic without
depending on goroutine scheduling latency.

---

## 4. Negative control

Added a single `return` as the first line of `emitSwapPayoutWebhook` in
`internal/telemetry/btc.go` (disables the emit path itself; all four call
sites and every terminal status transition untouched), rebuilt (`go build
./...` still exit 0, an early return is a vet-only unreachable-code notice,
not a build error), and re-ran the notification tests:

```
$ go test ./internal/server/... -run 'TestProcessPending_(Completed_FiresPayoutWebhook|UnpayableRow_FiresFailedWebhook|NegativeAmount_FiresFailedWebhook|AmbiguousError_FiresNeedsReconcileWebhook)' -count=1 -v
=== RUN   TestProcessPending_Completed_FiresPayoutWebhook
    swap_payout_webhook_test.go:83: timed out waiting for the swap payout webhook to fire
--- FAIL: TestProcessPending_Completed_FiresPayoutWebhook (2.01s)
=== RUN   TestProcessPending_UnpayableRow_FiresFailedWebhook
    swap_payout_webhook_test.go:119: timed out waiting for the swap payout webhook to fire
--- FAIL: TestProcessPending_UnpayableRow_FiresFailedWebhook (2.00s)
=== RUN   TestProcessPending_NegativeAmount_FiresFailedWebhook
    swap_payout_webhook_test.go:147: timed out waiting for the swap payout webhook to fire
--- FAIL: TestProcessPending_NegativeAmount_FiresFailedWebhook (2.00s)
=== RUN   TestProcessPending_AmbiguousError_FiresNeedsReconcileWebhook
    swap_payout_webhook_test.go:174: timed out waiting for the swap payout webhook to fire
--- FAIL: TestProcessPending_AmbiguousError_FiresNeedsReconcileWebhook (2.00s)
FAIL
```

All four notification tests correctly time out waiting for a webhook that
never fires. Separately re-ran a sample of the SG-05 settlement-correctness
tests (`TestProcessPending_HappyPath_SendsOnceAndCompletes`,
`TestProcessPending_NegativeAmount_NoSendMarksFailed`,
`TestProcessPending_AmbiguousError_NotReleased_NoResend`) with the same early
return in place: all 3 still PASS, confirming the detection layer is
additive and never load-bearing for settlement correctness. The early
`return` was then removed; all 11 SG-07 tests green again (§3).

---

## 5. Reproduce

```bash
cd <repo>   # branch feat/swap-payout-webhook
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH
go version   # go1.24.2
go build ./...
go test ./internal/utils/webhook/... ./internal/server/... -count=1
```

Do NOT run `go test ./...` bare (mainnet RPC hangs, and `internal/telemetry`'s
test package does not build regardless, see §6).

To exercise the real Discord side once deployed: set
`SWAP_PAYOUT_WEBHOOK_URL` to a Discord channel's incoming-webhook URL (Vault
key `SWAP_PAYOUT_WEBHOOK_URL` in staging/prod, `.env.<env>` locally) and let a
BTC payout settle; a message posts to that channel with the swap fields.

---

## 6. Pre-existing broken package (out of scope)

`internal/telemetry`'s own test package (`btc_multi_endpoint_test.go`) fails to
build against an older `telemetry.New` arity, a since-removed
`IndexBTCTransactions` method, and an undefined `store.MockStore`/`NewMockStore`
(an unrelated multi-endpoint refactor). Confirmed identical on untouched
`develop` via `git stash`:

```
$ git stash push -u
$ go test ./internal/telemetry/... -count=1
FAIL  telemetry  [build failed]  (undefined: store.NewMockStore; not enough
                                  arguments in call to telemetry.New; ...)
$ git stash pop
```

Not touched, per scope. Because `internal/telemetry`'s test package does not
build, `ProcessPendingBtcTransactions` (including the new webhook emit) is
tested from `internal/server`, the composition root that wires the cron ->
telemetry, using a real `telemetry.Telemetry` + real store over an in-memory
sqlite + a Send-counting mock BTC RPC + an `httptest` server standing in for
Discord. Same approach SG-05's `settlement_test.go` already uses.

---

## 7. Rollback

- Code: `git revert` / drop the branch. No data migration, no schema change.
- Config: unset `SWAP_PAYOUT_WEBHOOK_URL` (env or Vault key) at any time to
  silence the notification without a deploy; `emitSwapPayoutWebhook` no-ops on
  an empty URL and settlement is completely unaffected (see
  `TestProcessPending_NoWebhookURLConfigured_SettlesWithoutNotifying`).
- Nothing to reconcile: this is a pure notification side-effect, it never
  writes to the database and never participates in the settlement state
  machine.

---

## 8. Follow-on changes (2026-07-20/21)

The original SG-07 doc above describes the terminal-state text notification.
Three changes shipped after it:

- **Swap-detected emit.** `CreateBtcPayoutForSwap` now fires a `pending` event
  the moment a swap is detected on-chain and its payout row is created, not only
  at settlement. Built from the swap tx directly (the row is still uncommitted in
  the tx), fire-and-forget. Tests: `TestCreateBtcPayout_FiresSwapDetectedWebhook`
  + negative control `TestCreateBtcPayout_ShortDeposit_NoWebhook`.
- **Vault-balance enrichment.** Each notification carries the treasury BTC
  balance (sats + BTC), fetched best-effort inside the detached webhook
  goroutine so the read never slows settlement; a failed lookup omits the field.
  Tests: `TestProcessPending_Completed_WebhookCarriesVaultBalance` and the
  `..._BalanceError_OmitsVaultField` negative control (server), plus
  `TestCallSwapPayoutWebhook_VaultBalance_AppendsFieldWithBtc` /
  `..._NoVaultBalance_OmitsField` (webhook package). **Security note:** this puts
  the live hot-wallet balance and every destination address into the Discord
  channel; the channel must stay scoped to a trusted group (flagged by the
  2026-07-21 security review, accepted by the operator).
- **Embed format.** The payload moved from a plain-text `content` blob to a
  Discord **embed**: title `BTC payout · <status>`, colour by status
  (blurple pending / green completed / red failed / amber needs_reconcile), the
  ICY emoji as thumbnail (custom emoji markup does not render inside embed
  fields), fields for ICY / BTC / destination / (tx, omitted when empty) / vault
  balance, and a timestamp. This also fixed a real bug in the text format: an
  empty tx hash rendered as an unbalanced `` `` `` that spilled the following
  line into monospace. Amounts are now human-formatted (`2000 ICY`, `0.001239
  BTC`) alongside the raw base units. Config vocab: status gained `pending`;
  the emitter reads `SWAP_PAYOUT_WEBHOOK_URL` from Vault as before.

Channel: Dwarves `#logs` (id `1376917394710335590`), webhook item
`discord-webhook-icy-swap-payouts` in 1Password (`op://Toolkit`).
