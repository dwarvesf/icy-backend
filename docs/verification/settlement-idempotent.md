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
