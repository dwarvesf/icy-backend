# Proof of Done, Idempotent BTC Settlement (SG-05 / CRIT-3)

**Outcome:** each swap event pays out BTC exactly once. A crash mid-payout, a
re-run, or two overlapping cron cycles cannot send the same BTC twice.

**Branch:** `fix/settlement-idempotent` (base `develop`).
**Go:** 1.24.2. `go build ./...` exit 0.

---

## 1. Acceptance criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | **Atomic claim.** Pending rows claimed via conditional `UPDATE … SET status='processing' WHERE id=? AND status='pending'`, acting only when `RowsAffected == 1`. No lock-free SELECT-then-send. | PASS | `store.ClaimPendingTransaction`; `TestClaimPendingTransaction_FirstWinsSecondSkips`, `…_ConcurrentExactlyOneWins` |
| 2 | **No cron overlap.** Settlement cron wrapped so a run cannot overlap itself (robfig/cron `SkipIfStillRunning`). | PASS | `server.newSettlementJob`; `TestNewSettlementJob_SkipsOverlappingTick` |
| 3 | **Idempotent send keyed on the swap-event row.** A re-run after crash-between-broadcast-and-complete does NOT re-broadcast. | PASS | claim gate + status filter; `TestProcessPending_CrashStrandedProcessing_NoResend`, `TestClaimPendingTransaction_CrashLeavesProcessing_NoResend`, `TestProcessPending_RerunAfterComplete_NoResend`, `TestProcessPending_ConcurrentOverlap_SendsOnce` |
| 4a | **Negative-amount guard fixed** (checked `.Decimal`, a constant, instead of `.Value`). | PASS | `internal/telemetry/btc.go` `ProcessPendingBtcTransactions`; `TestProcessPending_NegativeAmount_NoSendMarksFailed` |
| 4b | **UNIQUE constraint on `swap_transaction_hash`** (cheap). | PASS | migration `0013_add_unique_swap_transaction_hash_to_btc_processed.{up,down}.sql` |
| 5 | **No re-send of an ambiguous broadcast (double-send fix).** On a `Send` error the row is released `processing → pending` ONLY when the error is `btcrpc.ErrNotBroadcast` (definitely-not-submitted). Any ambiguous/post-POST error moves the row to terminal `needs_reconcile`, never back to `pending`, so the next tick never rebuilds-and-resends a tx that may be live. | PASS | `internal/telemetry/btc.go`, `internal/btcrpc/{btcrpc,helper}.go`, `internal/btcrpc/blockstream/{blockstream,interface}.go`; `TestProcessPending_AmbiguousError_NotReleased_NoResend`, `TestProcessPending_NotBroadcastError_ReleasedForRetry`, `TestBroadcastTx_AlreadyKnown_TreatedAsSuccess`, `TestBroadcastTx_GenuineError_NotAlreadyKnown` |

---

## 2. Implementation

**The exactly-once gate is a single conditional UPDATE.** A payout row moves
`pending → processing` atomically; only the caller that flips it (RowsAffected==1)
may broadcast. Two overlapping cron cycles both SELECT the pending row, but the
database serialises the two UPDATEs: one matches 1 row, the other matches 0. The
loser skips without sending.

- **`internal/model/onchain_btc_processed_transaction.go`**, new status
  `BtcProcessingStatusProcessing` ("processing"), the in-flight state between
  claim and completion.
- **`internal/store/onchainbtcprocessedtransaction/`**, new
  `ClaimPendingTransaction(tx, id) (bool, error)`: the conditional UPDATE,
  returns `RowsAffected == 1`. Added to `IStore`.
- **`internal/telemetry/btc.go` `ProcessPendingBtcTransactions`**, rewritten
  loop: **claim first**, skip if not won; on validation failure or negative
  amount → `failed` (terminal); on `Send` error → release `processing → pending`
  (safe retry, no BTC left the treasury); on `Send` success → `UpdateToCompleted`.
  A crash after a successful broadcast leaves the row in `processing` (never
  `pending` again), so it is never re-sent, the safe failure direction is a
  stranded row for manual reconcile, never a double-send.
- **Negative-amount guard fix**, old code tested `amount.Decimal < 0`
  (`.Decimal` is the fixed `BTC_DECIMALS` = 8, so the branch never fired and a
  negative payout could be broadcast). Now tests the parsed numeric value:
  `amtInt, ok := amount.Int64(); if !ok || amtInt < 0 { mark failed }`.
- **`internal/server/server.go`**, settlement moved to its own cron entry wrapped
  with `cron.NewChain(cron.SkipIfStillRunning(...))` via the extracted, testable
  `newSettlementJob`. Indexers stay on their own fire-and-forget tick.
- **`migrations/schema/0013_*`**, partial UNIQUE index
  `uniq_btc_processed_swap_transaction_hash` on `swap_transaction_hash` where it
  is non-null and non-empty (DB backstop; legacy NULL/empty rows exempt so it
  applies cleanly). golang-migrate up/down, matching migrations 0004/0006 style.
- **`.gitignore`**, anchored the root build-binary patterns `main`/`server` to
  `/main`//`/server`; the bare `server` pattern was silently ignoring the whole
  `internal/server/` directory (any new file there was untracked).

---

## 3. Confirmation run-table

Environment: `export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH`
(go1.24.2). `go test ./...` is NOT run bare (mainnet RPC hangs); only touched
packages. `internal/telemetry` and `internal/btcrpc` carry PRE-EXISTING broken
tests (unrelated multi-endpoint/config refactors) and do not build, confirmed
identical on untouched HEAD via `git stash` (see §6).

| Command | Exit | Result |
|---------|------|--------|
| `go build ./...` | 0 | builds |
| `go vet ./internal/store/onchainbtcprocessedtransaction/... ./internal/server/...` | 0 | clean |
| `go test ./internal/store/onchainbtcprocessedtransaction/... -count=1` | 0 | ok (3 tests) |
| `go test ./internal/server/... -count=1` | 0 | ok (6 tests) |

Per-test (9 PASS):

```
PASS  onchainbtcprocessedtransaction  TestClaimPendingTransaction_FirstWinsSecondSkips
PASS  onchainbtcprocessedtransaction  TestClaimPendingTransaction_ConcurrentExactlyOneWins
PASS  onchainbtcprocessedtransaction  TestClaimPendingTransaction_CrashLeavesProcessing_NoResend
PASS  server  TestNewSettlementJob_SkipsOverlappingTick
PASS  server  TestProcessPending_HappyPath_SendsOnceAndCompletes
PASS  server  TestProcessPending_RerunAfterComplete_NoResend
PASS  server  TestProcessPending_CrashStrandedProcessing_NoResend
PASS  server  TestProcessPending_ConcurrentOverlap_SendsOnce
PASS  server  TestProcessPending_NegativeAmount_NoSendMarksFailed
```

Tests use an in-memory sqlite (pure-Go `glebarez/sqlite`, `MaxOpenConns(1)`); the
conditional-UPDATE + `RowsAffected` contract is portable to the production
Postgres.

---

## 4. Negative control

Revert the atomic claim to lock-free (`ClaimPendingTransaction` returns
`true` unconditionally, ignoring `RowsAffected`) and re-run:

```
FAIL  onchainbtcprocessedtransaction  TestClaimPendingTransaction_FirstWinsSecondSkips
FAIL  onchainbtcprocessedtransaction  TestClaimPendingTransaction_ConcurrentExactlyOneWins
FAIL  onchainbtcprocessedtransaction  TestClaimPendingTransaction_CrashLeavesProcessing_NoResend
FAIL  server  TestProcessPending_ConcurrentOverlap_SendsOnce
      settlement_test.go:219: overlap double-send: send count = 2, want 1
PASS  server  TestNewSettlementJob_SkipsOverlappingTick   (independent: cron guard)
PASS  server  TestProcessPending_CrashStrandedProcessing_NoResend  (second layer: status filter)
```

The overlap test double-sends (`send count = 2`) exactly as the guard is meant to
prevent. The cron-overlap test still passes because `SkipIfStillRunning` is an
independent second mechanism. Claim restored; all 9 green again.

---

## 4b. Double-send fix: never re-send an ambiguous broadcast

**The bug (confirmed exploitable).** `ProcessPendingBtcTransactions` claimed a row
(`pending → processing`), called `btcRpc.Send`, and on ANY non-nil `Send` error
released the row `processing → pending`. But `Send` returns a non-nil error on
paths where the signed tx is ALREADY on the wire: `blockstream.BroadcastTx` used
an `http.Client{}` with NO timeout, so attempt 1 could reach the node and enqueue
the tx while its response was lost (read timeout / 5xx after enqueue / `io.ReadAll`
failure); the 3-attempt retry then re-POSTed the same signed tx and the node
answered `txn-already-known` / `bad-txns-inputs-missingorspent` with a non-200,
returning a hard error while the tx was LIVE. The row went back to `pending`, the
next cron tick re-claimed it and built a FRESH valid tx paying the SAME recipient
from tx1's now-confirmed change output. **BTC sent twice.**

**Fail-safe fix (three parts, all surgical).**

1. **Already-broadcast = success** (`internal/btcrpc/blockstream/`). `BroadcastTx`
   now classifies node responses meaning "the tx is already known/accepted"
   (`txn-already-known`, `transaction already in block chain`,
   `bad-txns-inputs-missingorspent`, already-in-mempool) and returns the exported
   sentinel `ErrTxAlreadyKnown` instead of a hard error. `internal/btcrpc/helper.go`
   `broadcast` maps that to SUCCESS, returning the locally-computed
   `tx.TxHash().String()` (the node body carries no txid on this path). The
   `http.Client` now has a 30s `Timeout` so a hung broadcast fails FAST into the
   ambiguous class instead of blocking the settlement loop.
2. **Typed error** (`internal/btcrpc/btcrpc.go`). New exported sentinel
   `ErrNotBroadcast` marks "definitely NOT submitted": every error raised BEFORE
   the first POST (WIF decode, `DecodeAddress`, amount parse, `selectUTXOs` /
   insufficient-funds, `prepareTx`, `sign`) and the fee-adjustment prep after a
   clean min-relay-fee rejection (fee-too-high, insufficient-funds-to-adjust,
   `EstimateFees`, re-prepare/re-sign) is wrapped with it. Any error AT or AFTER a
   POST (lost response, the re-broadcast POST) is returned UNWRAPPED = ambiguous.
   The default for an unclassified error is therefore ambiguous (fail-safe).
3. **Conditional release** (`internal/telemetry/btc.go`). On a `Send` error the row
   is released `processing → pending` (safe retry) ONLY when
   `errors.Is(err, btcrpc.ErrNotBroadcast)`. For any ambiguous error the row moves
   to the new TERMINAL status `needs_reconcile` (`GetPendingTransactions` never
   picks it up; no reaper re-pendings it), so it is NEVER auto-re-sent. A row
   stranded in `processing`/`needs_reconcile` is the intended SAFE outcome
   (liveness cost, manual reconcile), never a double payout. `needs_reconcile`
   fits `status VARCHAR(20)` with no CHECK constraint, so no migration is needed.

**Confirmation run-table.** Environment: `export
PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH` (go1.24.2).

| Command | Exit | Result |
|---------|------|--------|
| `go build ./...` | 0 | builds |
| `go test ./internal/store/onchainbtcprocessedtransaction/... ./internal/server/... -count=1` | 0 | ok (both packages) |

Per-test (13 PASS; the 4 new for this fix in **bold**):

```
PASS  onchainbtcprocessedtransaction  TestClaimPendingTransaction_FirstWinsSecondSkips
PASS  onchainbtcprocessedtransaction  TestClaimPendingTransaction_ConcurrentExactlyOneWins
PASS  onchainbtcprocessedtransaction  TestClaimPendingTransaction_CrashLeavesProcessing_NoResend
PASS  server  TestNewSettlementJob_SkipsOverlappingTick
PASS  server  TestProcessPending_HappyPath_SendsOnceAndCompletes
PASS  server  TestProcessPending_RerunAfterComplete_NoResend
PASS  server  TestProcessPending_CrashStrandedProcessing_NoResend
PASS  server  TestProcessPending_ConcurrentOverlap_SendsOnce
PASS  server  TestProcessPending_NegativeAmount_NoSendMarksFailed
PASS  server  TestProcessPending_AmbiguousError_NotReleased_NoResend     <- core fix
PASS  server  TestProcessPending_NotBroadcastError_ReleasedForRetry      <- liveness
PASS  server  TestBroadcastTx_AlreadyKnown_TreatedAsSuccess              <- already-known
PASS  server  TestBroadcastTx_GenuineError_NotAlreadyKnown               <- neg control
```

**Negative control (double-send reproduced).** Revert the ambiguous branch in
`btc.go` to release `processing → pending` (the old buggy behavior) and re-run the
core test:

```
$ (patch btc.go: ambiguous error -> BtcProcessingStatusPending)
$ go test ./internal/server/... -run TestProcessPending_AmbiguousError_NotReleased_NoResend -count=1
    settlement_test.go:278: status = "pending", want needs_reconcile (NOT released to pending)
--- FAIL: TestProcessPending_AmbiguousError_NotReleased_NoResend (0.00s)
FAIL
```

The row is released to `pending`, so the next tick re-claims and re-sends it, the
double-send the fix forbids. Patch reverted; all 13 green again. The
`TestProcessPending_NotBroadcastError_ReleasedForRetry` test is the positive
contrast: same shape, a `btcrpc.ErrNotBroadcast` error, the row IS released and the
next tick retries (send count 1 -> 2, ending `completed`), so the fix does not cost
liveness on the safe path.

---

## 5. Reproduce

```bash
cd <repo>   # branch fix/settlement-idempotent
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH
go version   # go1.24.2
go build ./...
go test ./internal/store/onchainbtcprocessedtransaction/... ./internal/server/... -count=1
```

Do NOT run `go test ./...` bare (mainnet RPC hangs).

---

## 6. Pre-existing broken packages (out of scope)

`internal/telemetry` and `internal/btcrpc` (and `internal/btcrpc/blockstream`)
fail to BUILD their test binaries due to pre-existing broken tests
(`undefined: store.MockStore`, `IndexBTCTransactions`, stale `config` fields,
`NewMultiEndpoint`) from an unrelated multi-endpoint refactor. Confirmed identical
on untouched HEAD:

```
$ git stash push -u
$ go test ./internal/telemetry/... ./internal/btcrpc/... -count=1
telemetry   [build failed]
btcrpc      [build failed]
blockstream [build failed]
$ git stash pop
```

Not touched, per scope. Because the `internal/telemetry` test package does not
build, the settlement orchestrator (`ProcessPendingBtcTransactions`) is tested from
`internal/server` (the composition root that wires the cron → telemetry), using a
real telemetry instance + real store over sqlite + a Send-counting mock BTC RPC.

---

## 7. Rollback

- Code: `git revert` / drop the branch. No data migration on the code path.
- Migration 0013: `migrate down` one step runs
  `0013_…down.sql` → `DROP INDEX IF EXISTS uniq_btc_processed_swap_transaction_hash`.
  The index is additive and non-destructive; dropping it is safe.
- Note: after this ships, rows may exist in the new `processing` status. A rollback
  to pre-SG-05 code (which does not know `processing`) would leave those rows
  unpicked by `GetPendingTransactions` (status='pending' only), they need a
  manual `UPDATE … SET status='pending'` for any that are genuinely un-broadcast.
```
