# Proof of done, enforce swap rate server-side + re-auth generate-signature

icy-swap-hardening SG-03 / DF-74. Closes CRIT-1 (verified-live treasury drain via the public signing endpoint).

## Acceptance criteria

| # | Criterion | Result |
|---|---|---|
| 1 | The signed BTC amount is derived server-side from the oracle, not the client | PASS |
| 2 | An inflated client `btc_amount` (≫ oracle) is rejected (400) or ignored | PASS |
| 3 | `generate-signature` **always** requires auth (no flag) → no-key = 401 unconditionally | PASS |
| 4 | The public signing endpoint no longer fires a gas-paying on-chain `Approve` | PASS |

## Implementation

- `internal/handler/swap/swap.go` `GenerateSignature`: computes `serverSat = icy_wei × btcSupply / circulatedICY` from the 5-min cached oracle balances and signs only that; rejects a client `btc_amount` that exceeds the oracle amount by >1%.
- `internal/baserpc/baserpc.go`: moved the on-chain `Approve` out of `GenerateSignature` into `Swap()` (the settlement path that actually consumes the allowance).
- `internal/transport/http/http.go` + `internal/utils/config/config.go`: `/swap/generate-signature` goes through the same `apiKeyMiddleware` API-key check as every other authenticated route. Commit `564aacf` (initial), auth-enforcement follow-up below.

### Auth follow-up (fail-closed, replaces the default-off flag)

The rung-4 adversarial review found the original re-auth was gated behind `REQUIRE_SWAP_SIGNATURE_AUTH`, which **defaulted to false**, so in the prod/default config the endpoint was still reachable with no auth (Done criterion #1 not met). Fix:

- `internal/transport/http/http.go`: **removed** the `RequireSwapSignatureAuth` bypass branch for `/api/v1/swap/generate-signature`. The endpoint now falls through to the standard `apiKeyMiddleware` Authorization-header check, a missing/invalid `ApiKey` returns `401` in prod, matching every other authed route. (Non-prod `AppEnv` still short-circuits `c.Next()` as before; that is the codebase's existing local-dev convention, unchanged.)
- `internal/utils/config/config.go`: **removed** the `RequireSwapSignatureAuth` config field, its `envVarAsBool("REQUIRE_SWAP_SIGNATURE_AUTH")` read, and the now-orphaned `envVarAsBool` helper (its only caller). No flag = no footgun; the fragile `valueStr == "true"` parse (MEDIUM finding) is eliminated by removal rather than patched.
- `internal/transport/http/middleware_auth_test.go`: the "allows an unauthenticated request when auth is not required" spec is deleted; the no-key spec now asserts `401` **unconditionally** with no env flag set.

Grep proof the flag had no other consumer: `grep -rn "RequireSwapSignatureAuth\|REQUIRE_SWAP_SIGNATURE_AUTH" --include="*.go"` returns **zero** matches after the change.

## Recorded run (gate format)

Command: go build ./...
Exit: 0

Command: go test ./internal/handler/swap/ratecheck/ ./internal/transport/http/ -count=1
Exit: 0
Output: ratecheck 4 specs PASS, http 4 specs PASS (8 total): signs oracle-derived amount; rejects a 1000× inflated client btc_amount (400); ignores within-tolerance client value; unauth 401 when flag on; correct key 200; wrong key 401.

NEGATIVE CONTROL: stashing `swap.go` back to the vulnerable base and re-running ratecheck → 2 of 4 FAIL (the inflated-reject spec and the sign-oracle-amount spec, the two drain-catching specs). Restored after. Verdict: PASS.

Toolchain: repo pins Go 1.24.2 via mise; ran with `GOROOT=~/.local/share/mise/installs/go/1.24.2` (brew go is 1.26.4). `make test-handler`/`make test-oracle` are stale-rotten on the clean base (pre-existing, unrelated), so the fix is proven in an isolated `ratecheck/` package + the `http` package.

## Recorded run, auth-enforcement follow-up (2026-07-08)

Toolchain: `export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH` → `go version` = `go1.24.2 darwin/arm64`.

| # | Command | Exit | Result |
|---|---|---|---|
| 1 | `go build ./...` | 0 | builds clean |
| 2 | `go test ./internal/transport/http/... ./internal/handler/swap/ratecheck/... ./internal/utils/config/... -count=1` | 0 | 3 packages pass |
| 3 | `go test ./internal/transport/http/ -run TestHTTPMiddlewareAuth -count=1 -v` | 0 | `HTTP apiKeyMiddleware Auth Suite`: Ran 3 of 3, **SUCCESS!** 3 Passed |
| 4 | grep `RequireSwapSignatureAuth\|REQUIRE_SWAP_SIGNATURE_AUTH` over `*.go` | 1 (no match) | flag fully removed, zero remaining consumers |

The three passing auth specs (no env flag set anywhere):
- `returns 401 for a no-key request in prod (auth always required, no flag)`, the Done-criterion #1 assertion, holds unconditionally.
- `allows a correctly-keyed request in prod`, `ApiKey secret` → 200.
- `rejects a wrong-key request in prod`, `ApiKey wrong` → 401.

**NEGATIVE CONTROL:** temporarily re-inserted an unconditional bypass branch for `/api/v1/swap/generate-signature` into `apiKeyMiddleware` (the pre-fix vulnerable state), re-ran `go test ./internal/transport/http/ -run TestHTTPMiddlewareAuth`:

```
• [FAILED] ... returns 401 for a no-key request in prod (auth always required, no flag)
  Expected
      <int>: 200
  to equal
      <int>: 401
FAIL! -- 1 Passed | 2 Failed
```

The no-key request returned **200** (bypass) instead of 401, proving the assertion actually exercises the auth path. Bypass reverted; suite green again (exit 0). Verdict: PASS.

### 08 deploy dependency (frontend API key)

Making `generate-signature` always-auth is fail-closed in `develop`, but the `dwarvesf/icy-swap` frontend currently calls the endpoint with **no** API key. Before icy-backend `develop` is promoted to **prod**, the frontend must be issued and start sending `Authorization: ApiKey <key>`, or every real swap-signature request will 401. The code is deliberately not weakened to accommodate the current keyless frontend; the frontend-key rollout is coordinated at the prod deploy gate (sub-goal 08), not by softening this control. (Note: non-prod `AppEnv` still bypasses the check via the pre-existing `c.Next()` short-circuit, so local/staging is unaffected.)

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
