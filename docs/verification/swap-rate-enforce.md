# Proof of done, enforce swap rate server-side + re-auth generate-signature

icy-swap-hardening SG-03 / DF-74. Closes CRIT-1 (verified-live treasury drain via the public signing endpoint).

## Acceptance criteria

| # | Criterion | Result |
|---|---|---|
| 1 | The signed BTC amount is derived server-side from the oracle, not the client | PASS |
| 2 | An inflated client `btc_amount` (≫ oracle) is rejected (400) or ignored | PASS |
| 3 | `generate-signature` can require auth (flag) → no-key = 401 | PASS |
| 4 | The public signing endpoint no longer fires a gas-paying on-chain `Approve` | PASS |

## Implementation

- `internal/handler/swap/swap.go` `GenerateSignature`: computes `serverSat = icy_wei × btcSupply / circulatedICY` from the 5-min cached oracle balances and signs only that; rejects a client `btc_amount` that exceeds the oracle amount by >1%.
- `internal/baserpc/baserpc.go`: moved the on-chain `Approve` out of `GenerateSignature` into `Swap()` (the settlement path that actually consumes the allowance).
- `internal/transport/http/http.go` + `internal/utils/config/config.go`: `/swap/generate-signature` removed from the unconditional skip-list; re-auth gated behind `REQUIRE_SWAP_SIGNATURE_AUTH` (default false, so the current frontend keeps working until it sends a key).
- Commit `564aacf`.

## Recorded run (gate format)

Command: go build ./...
Exit: 0

Command: go test ./internal/handler/swap/ratecheck/ ./internal/transport/http/ -count=1
Exit: 0
Output: ratecheck 4 specs PASS, http 4 specs PASS (8 total): signs oracle-derived amount; rejects a 1000× inflated client btc_amount (400); ignores within-tolerance client value; unauth 401 when flag on; correct key 200; wrong key 401.

NEGATIVE CONTROL: stashing `swap.go` back to the vulnerable base and re-running ratecheck → 2 of 4 FAIL (the inflated-reject spec and the sign-oracle-amount spec, the two drain-catching specs). Restored after. Verdict: PASS.

Toolchain: repo pins Go 1.24.2 via mise; ran with `GOROOT=~/.local/share/mise/installs/go/1.24.2` (brew go is 1.26.4). `make test-handler`/`make test-oracle` are stale-rotten on the clean base (pre-existing, unrelated), so the fix is proven in an isolated `ratecheck/` package + the `http` package.

## Out-of-scope findings surfaced

- **DF-85 (potential 100× overpay):** `ProcessSwapRequests` (`telemetry/swap.go:288`) derives sat at ~10,000,000 sat/ICY on the repo's 1000:1 fixtures vs `/swap/info` at 100,000 sat/ICY. Confirm before Flow-B settles anything.
- Approve-removal caveat: if a user-initiated on-chain swap relies on a per-call backend allowance, set a standing allowance (contract un-inspectable here). The settlement worker still approves per swap, so no regression.

## Reproduce

```
git checkout fix/swap-rate-enforce
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH GOROOT=$HOME/.local/share/mise/installs/go/1.24.2
go test ./internal/handler/swap/ratecheck/ ./internal/transport/http/ -count=1
```

## Rollback

`git revert 564aacf`, source-only, no migration/data change. Reverting restores the prior (vulnerable) behavior exactly; only re-do if the rate-enforcement causes a frontend regression (it mirrors `/swap/info`, which the frontend already uses).
