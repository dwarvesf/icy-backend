# Proof of done, Flow-B swap-request status tracking

icy-swap-hardening SG-04 / DF-75. Closes CRIT-2 (repeated BTC payout: `ProcessSwapRequests` never advanced status, so every cron tick re-swapped the same request with a fresh nonce and the on-chain replay guard could not dedupe).

## Acceptance criteria

| # | Criterion | Result |
|---|---|---|
| 1 | A settled swap_request is never re-processed on a later tick | PASS |
| 2 | Status advances pending → processing (claim) → completed / failed | PASS |
| 3 | A failed swap is marked `failed` (terminal), not silently retried | PASS |
| 4 | The nonce is deterministic per request (retry reuses it) | PASS |

## Implementation

- `internal/model/swap_request.go`: added `Processing` and `Failed` status constants.
- `internal/telemetry/swap.go` `ProcessSwapRequests`: claims the row (`pending → processing` via `UpdateStatus`) immediately before the on-chain `Swap`, then settles `completed`/`failed`. Transient pre-swap validation failures still leave it `pending` (retry preserved). Nonce = `keccak256(icyTx)` via `deriveSwapNonce`, replacing the time-based nonce.
- `internal/baserpc/{interface,baserpc}.go`: `Swap()` takes a `nonce *big.Int` (falls back to time-based if nil).
- `internal/monitoring/circuit_breaker.go`: forwards the nonce through the wrapper.
- Commit `a484346`.

Deliberate deviation: the on-chain RPC is NOT wrapped in a SQL transaction (holding a DB tx across a network call is an anti-pattern); the claim-write commits atomically just before the swap and the settle-write just after, still guaranteeing a settled request is never re-picked.

## Recorded run (gate format)

Command: go build ./...
Exit: 0

Command: go test ./internal/telemetry/... -count=1 -v   (isolated; the pre-existing broken sibling test file set aside)
Exit: 0
Output: 4 specs PASS, settles a pending request at most once across two ProcessSwapRequests() calls; never touches processing/completed/failed rows; marks failed swaps failed (terminal); reuses the nonce across a manual retry.

NEGATIVE CONTROL: stripping the status-claim logic (keeping the test file) → 1 Passed, 3 FAILED, exactly the three specs that assert the CRIT-2 fix (at-most-once, failed-terminal, nonce-reuse). Verdict: PASS.

Pre-existing (not from this change): `internal/telemetry/btc_multi_endpoint_test.go` fails to compile on the clean base `ff49a48` (confirmed byte-identical via `cmp`); it is excluded from the run above and was not touched.

## Reproduce

```
git checkout fix/flowb-status
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH GOROOT=$HOME/.local/share/mise/installs/go/1.24.2
go test ./internal/telemetry/... -count=1   # with the pre-existing broken sibling file set aside
```

## Rollback

`git revert a484346`, source-only (plus a new status constant), no migration/data change. Reverting restores prior behavior. Residual to note: nonce is deterministic but `deadline` is still fresh per attempt, so full replay-guard determinism across arbitrary-time retries is not achieved (flagged for review; the bug report only flagged the nonce).
