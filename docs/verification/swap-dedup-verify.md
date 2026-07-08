# Proof of Done, Dedup icy_tx + Verify ICY Deposit before BTC Payout (SG-12 / DF-83)

**Outcome:** BTC never leaves for an ICY swap that (a) was already paid out, or
(b) did not actually deposit the ICY it claims. The Base swap indexer now dedups
on the populated `swap_transaction_hash` key AND verifies the on-chain ICY
deposit landed in the treasury before it creates any payable row.

**Branch:** `fix/swap-dedup-verify` (base `develop`, which already has SG-03,
SG-04 Flow-B-retire, SG-05).
**Go:** 1.24.2. `go build ./...` exit 0.
**Run-table image:** `docs/proof/sg12-dedup-verify.png` (source `docs/verification/sg12-run-table.txt`).

---

## 1. Acceptance criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | **Dedup actually matches.** The dedup was dead: `IcyTransactionHash` was never populated, so `GetByIcyTransactionHash` never matched. Re-keyed the application dedup to `swap_transaction_hash` (the column that IS populated on every payout and already carries the migration-0013 partial UNIQUE index). The indexer skips a swap whose payout already exists. | PASS | `store.GetBySwapTransactionHash`; `Telemetry.CreateBtcPayoutForSwap` gate 1; `TestGetBySwapTransactionHash_FindsAndMisses`, `TestCreateBtcPayout_DuplicateSwap_NoSecondPayout` |
| 1b | **DB backstop.** A second payout row for the same swap tx is impossible even if the app guard were bypassed (migration 0013 partial UNIQUE index). | PASS | `TestSwapHashUniqueIndex_RejectsDuplicatePayout` |
| 1c | **`IcyTransactionHash` now populated too.** Set to the swap tx hash on create, reviving the previously-dead `GetByIcyTransactionHash` lookup and recording which ICY tx the payout settles. | PASS | `CreateBtcPayoutForSwap` create block; `TestCreateBtcPayout_VerifiedDeposit_CreatesOne` asserts the column is non-null |
| 1d | **Re-index no longer aborts the batch.** Duplicate swap rows (which inflate the ICY backing sum) are skipped via `GetByTransactionHash`, so a cursor-reset re-scan is idempotent instead of dying on the unique index. | PASS | `IndexIcySwapTransaction` swap-dedup guard |
| 2 | **Verify the ICY deposit before paying BTC.** Before creating the payout, the swap tx must have transferred at least `IcyAmount` of ICY to the treasury. Treasury address = `baseRpc.GetContractAddress()` (config `ICYSwapContractAddr`); ICY token = config `ICYContractAddr`; required amount = the Swap event's `IcyAmount`. All read from config, none hardcoded. | PASS | `baserpc.ICYTransferredTo` + pure `SumICYTransfersTo`; `Telemetry.verifyIcyDeposit`; `TestCreateBtcPayout_VerifiedDeposit_CreatesOne`, `TestCreateBtcPayout_OverDeposit_Accepted` |
| 2b | **Short / mismatched / absent ICY deposit is rejected (no payout row).** | PASS | `TestCreateBtcPayout_ShortDeposit_Rejected`, `TestCreateBtcPayout_AbsentDeposit_Rejected`, `TestSumICYTransfersTo_ShortDepositUndercounts`, `…_AbsentDepositIsZero` |
| 2c | **Wrong token / wrong recipient / non-Transfer logs do not satisfy the deposit.** A different token's Transfer, a transfer to a non-treasury address, or an Approval log are all ignored. | PASS | `TestSumICYTransfersTo_WrongTokenIgnored`, `…_WrongRecipientIgnored`, `…_NonTransferLogSkipped` |
| 2d | **Fail-closed on verify infra error.** If the ICY deposit cannot be verified (RPC failure), `CreateBtcPayoutForSwap` returns the error so the enclosing `DoInTx` rolls back and retries later, never paying an unverified deposit. | PASS | `verifyIcyDeposit` returns wrapped infra error; `CreateBtcPayoutForSwap` non-mismatch branch |

---

## 2. Implementation

**The two money-gates live in one exported, testable unit.** The indexer's inline
payout-create block was extracted into `Telemetry.CreateBtcPayoutForSwap(tx, swapTx)`,
which runs both gates before any row is written. Exported (like SG-05's settlement
methods) so it is exercisable from a package whose test binary builds
(`internal/server`), because the `internal/telemetry` test binary is pre-existing
build-broken (see §5).

- **`internal/telemetry/swap.go`**
  - `IndexIcySwapTransaction` inner loop: added a **swap-row dedup** via
    `OnchainIcySwapTransaction.GetByTransactionHash` (skip an already-indexed swap;
    keeps a cursor-reset re-scan idempotent instead of aborting on the unique
    index), then delegates payout creation to `CreateBtcPayoutForSwap`.
  - New `CreateBtcPayoutForSwap`: **gate 1** dedup on
    `OnchainBtcProcessedTransaction.GetBySwapTransactionHash` (skip if a payout
    already exists); **gate 2** `verifyIcyDeposit`; then the unchanged fee math and
    the create, now also setting `IcyTransactionHash`.
  - New `verifyIcyDeposit`: parses `IcyAmount`, reads treasury from
    `baseRpc.GetContractAddress()`, calls `baseRpc.ICYTransferredTo`, and compares
    `deposited >= required`. Returns `ErrIcyDepositMismatch` (business rejection,
    drop this payout, keep the batch) vs a wrapped infra error (abort + retry).
  - New sentinel `ErrIcyDepositMismatch`.
- **`internal/baserpc/interface.go` + `baserpc.go`**
  - New `ICYTransferredTo(txHash, to) (*big.Int, error)`: fetches the tx receipt
    (with the existing retry wrapper) and sums ICY Transfer logs to `to`.
  - New exported pure helper `SumICYTransfersTo(logs, icyToken, to)`: RPC-free
    log-summing so the money-critical amount math is unit-testable with hand-built
    logs. Counts a log only when `log.Address == icyToken` and it parses as a
    `Transfer` to `to`.
- **`internal/store/onchainbtcprocessedtransaction/` (interface + impl)**
  - New `GetBySwapTransactionHash(tx, swapTxHash)`: the re-keyed dedup lookup on
    the populated, DB-unique column.
- **`internal/monitoring/circuit_breaker.go`**
  - `CircuitBreakerBaseRPC.ICYTransferredTo` wrapper (the decorator must satisfy
    the widened `IBaseRPC`).
- **`internal/handler/swap/ratecheck/generate_signature_test.go`**
  - `stubBaseRPC.ICYTransferredTo` stub, so this previously-building test package
    still satisfies `IBaseRPC` after the interface widened. (Not a fix of unrelated
    breakage; a required consequence of the interface change.)

**Why dedup on `swap_transaction_hash` and not `icy_transaction_hash`:** in Flow A
the Swap event's tx hash IS the on-chain ICY tx. `swap_transaction_hash` is already
written on every payout and already backed by the 0013 UNIQUE index; keying the app
guard to the same column gives one coherent dedup surface (fast app-level skip +
DB backstop). `IcyTransactionHash` is now populated too for completeness, but the
dedup key is `swap_transaction_hash`.

---

## 3. Run table

Full output: `docs/verification/sg12-run-table.txt` (rendered `docs/proof/sg12-dedup-verify.png`).

| Command | Exit | Result |
|---|---|---|
| `go build ./...` | 0 | builds |
| `go test ./internal/store/onchainbtcprocessedtransaction/... -count=1` | 0 | ok (dedup + claim) |
| `go test ./internal/baserpc/... -count=1` | 0 | ok (deposit-sum math) |
| `go test ./internal/server/... -count=1` | 0 | ok (payout money-gates + SG-05) |
| `go test ./internal/handler/swap/ratecheck/... -count=1` | 0 | ok (stub in sync) |

Money-gate tests (verbose), all PASS:

```
TestCreateBtcPayout_VerifiedDeposit_CreatesOne      (verified deposit -> one pending payout, icy_transaction_hash populated)
TestCreateBtcPayout_OverDeposit_Accepted            (deposit >= required accepted)
TestCreateBtcPayout_DuplicateSwap_NoSecondPayout    (DEDUP: same swap twice -> exactly 1 payout)
TestCreateBtcPayout_ShortDeposit_Rejected           (VERIFY: 500 < 1000 -> 0 payouts)
TestCreateBtcPayout_AbsentDeposit_Rejected          (VERIFY: 0 deposited -> 0 payouts)
TestGetBySwapTransactionHash_FindsAndMisses         (dedup key populated + queryable)
TestSwapHashUniqueIndex_RejectsDuplicatePayout      (DB backstop: dup swap hash rejected)
TestSumICYTransfersTo_ExactDepositCounted / _MultipleDepositsSummed / _ShortDepositUndercounts
TestSumICYTransfersTo_AbsentDepositIsZero / _WrongRecipientIgnored / _WrongTokenIgnored / _NonTransferLogSkipped
```

---

## 4. Negative controls

Each money-gate has a passing negative control (same shape, opposite input, opposite outcome):

| Gate | Positive | Negative control | Proves |
|---|---|---|---|
| Dedup | `TestCreateBtcPayout_VerifiedDeposit_CreatesOne` (1 payout) | `TestCreateBtcPayout_DuplicateSwap_NoSecondPayout` (2nd call → still 1) | the second index of a swap creates no second BTC payout |
| Dedup DB | first insert accepted | `TestSwapHashUniqueIndex_RejectsDuplicatePayout` (dup rejected, a *distinct* hash still inserts) | the unique index blocks only the duplicate |
| Verify amount | `_OverDeposit_Accepted` (1500 ≥ 1000 → paid) | `_ShortDeposit_Rejected` (500 < 1000 → 0), `_AbsentDeposit_Rejected` (0 → 0) | a short/absent deposit is never paid |
| Verify target | `_ExactDepositCounted` (to treasury → counted) | `_WrongRecipientIgnored`, `_WrongTokenIgnored` (→ 0) | only ICY-to-treasury counts |
| Verify parse | real Transfer counted | `_NonTransferLogSkipped` (Approval ignored) | non-Transfer logs never inflate the deposit |

**Manual revert check (conceptual):** deleting the gate-2 call in
`CreateBtcPayoutForSwap` makes `_ShortDeposit_Rejected` and `_AbsentDeposit_Rejected`
create a payout row (count 1), the exact double/false-payout these gates remove.
Deleting the gate-1 dedup makes `_DuplicateSwap_NoSecondPayout` create a second row
(then the DB index would reject it, aborting the batch), the pre-fix behaviour.

---

## 5. Pre-existing out-of-scope breakage (unchanged by this branch)

The `internal/telemetry` and `internal/handler/swap` (parent) **test binaries do not
build on `develop`** for reasons unrelated to this work:

- `internal/telemetry/btc_multi_endpoint_test.go`: stale `telemetry.New` arity,
  `store.MockStore`/`NewMockStore` undefined, `IndexBTCTransactions` undefined,
  `generateLargeBTCResponse()` non-constant.
- `internal/handler/swap/swap_info_{integration,timeout}_test.go`: `config.Bitcoin`
  undefined, mock types not implementing current interfaces, unused vars.

Confirmed **identical on untouched HEAD via `git stash`** (byte-for-byte the same
compiler errors before and after this branch). Not fixed here (out of scope). The
touched logic is therefore tested from packages that DO build (`internal/server`,
`internal/store/...`, `internal/baserpc`), mirroring how SG-05 tested the settlement
orchestrator from `internal/server`. The one test-package this branch *changed the
status of* is `internal/handler/swap/ratecheck`, which built before, would have
broken from the interface widening, and was kept building by adding the one-line
`stubBaseRPC.ICYTransferredTo` stub.

---

## 6. Reproduce

```
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH   # go1.24.2
cd <repo>
git switch fix/swap-dedup-verify
go build ./...
# Do NOT run `go test ./...` (mainnet RPC hangs). Touched packages only:
go test ./internal/store/onchainbtcprocessedtransaction/... \
        ./internal/baserpc/... \
        ./internal/server/... \
        ./internal/handler/swap/ratecheck/... -count=1
```

---

## 7. Rollback

Pure additive change; revert the branch's commits. New store method
(`GetBySwapTransactionHash`), interface method (`ICYTransferredTo`) + its wrapper,
and the two telemetry gates are all new surface. No schema migration was added
(migration 0013's UNIQUE index already existed on `develop` from SG-05). Reverting
restores the prior blind-trust behaviour; no data migration needed.
