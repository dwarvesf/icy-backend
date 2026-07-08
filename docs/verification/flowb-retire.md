# Proof of done, retire Flow B (request-based swap)

icy-swap-hardening SG-04. Contract DECISION: RETIRE Flow B entirely, an org-wide code scan proved
ZERO callers of `POST /api/v1/swap`. Retiring Flow B also moots **CRIT-2** (repeated BTC payout via
`ProcessSwapRequests`, status never advanced) and **DF-85** (100x settlement overpay, lived in the
same function): both bugs lived inside code that no longer exists. This PR **supersedes PR #34**
(`fix/flowb-status`, the CRIT-2 status-tracking fix) without closing it; Han decides that.

Flow A (`POST /swap/generate-signature`, `GET /swap/info`, the frontend calling the contract
directly) is untouched.

## Acceptance criteria

| # | Criterion | Result |
|---|---|---|
| 1 | `POST /api/v1/swap` route registration removed | PASS |
| 2 | `CreateSwapRequest` handler removed | PASS |
| 3 | `ProcessSwapRequests` processor + its cron wiring removed | PASS |
| 4 | Now-orphaned helpers (used only by the above) removed; pre-existing dead code left alone and flagged | PASS |
| 5 | Flow A (signing path) and BTC settlement idempotency untouched except where a shared symbol died only because Flow B is gone | PASS |
| 6 | `go build ./...` exit 0 | PASS |
| 7 | Route / handler / cron symbols grep-clean from live code | PASS |

## Implementation

**Removed (Flow B end to end):**

- `internal/transport/http/v1.go`: `swap.POST("", h.SwapHandler.CreateSwapRequest)` route.
- `internal/handler/swap/interface.go`: `CreateSwapRequest` from `IHandler`.
- `internal/handler/swap/swap.go`: the `CreateSwapRequest` handler (with its `TriggerSwap` swagger
  doc block), the `SwapRequest` request struct, and the now-dead `db *gorm.DB` and
  `btcProcessedTxStore` / `swapRequestStore` fields (used only by `CreateSwapRequest`). `swap.New(...)`
  dropped its `db` parameter; the two call sites in `internal/handler/handler.go` updated to match.
- `internal/telemetry/swap.go`: `ProcessSwapRequests`, and its private helpers `validateBTCAddress`,
  `confirmLatestPrice`, `calculateSatAmount`, `convertICYToSat` (all had zero callers outside
  `ProcessSwapRequests` or each other). The `consts` import, now unused in this file, was dropped.
  `IndexIcySwapTransaction` (Flow A's on-chain event indexer, feeds BTC settlement) is untouched.
- `internal/telemetry/interface.go`: `ProcessSwapRequests()` from `ITelemetry`.
- `internal/monitoring/instrumented_telemetry.go`: the `ProcessSwapRequests` webhook wrapper.
- `internal/server/server.go`: the `instrumentedTelemetry.ProcessSwapRequests()` cron call.
- `internal/utils/config/config.go`: `ProcessSwapRequestsURL` (struct field + both env/vault reads),
  orphaned once its sole consumer (the monitoring wrapper) was gone.
- `.env.example`: `PROCESS_SWAP_REQUESTS_UPTIME_WEBHOOK_URL=` line, same reason.
- `internal/store/swaprequest/{swap_request.go,interface.go}`: `Create` (used only by
  `CreateSwapRequest`) and `FindPendingSwapRequests` (used only by `ProcessSwapRequests`).

**Deliberately NOT touched (flagged, not deleted):**

- `internal/store/swaprequest`: `GetByIcyTx` and `UpdateStatus` have zero callers anywhere in the
  repo, but that was already true before this change (not orphaned *by* this deletion), this is
  pre-existing scaffolding, unrelated to the two Flow B entry points. Left in place per the
  surgical-changes rule ("flag pre-existing dead code, don't delete it"). The `swap_requests` table
  and `model.SwapRequest` therefore also stay.
- `internal/store/onchainbtcprocessedtransaction.GetByIcyTransactionHash`: became dead once
  `CreateSwapRequest` (its only caller) was removed, but it lives in the shared BTC-settlement store
  package (SG-05 territory); left alone rather than risk touching that boundary for one dead method.
- `internal/handler/health/jobs.go`: `"swap_request_processing"` stays in the `criticalJobs` label
  list. It's a string, not a symbol reference to anything deleted; since the job is never started
  again it will simply never match in `jobs`, so the health check degrades gracefully. Flagged as
  residual, not fixed, since it's not a Go symbol orphaned by this deletion.
- `internal/monitoring/job_monitoring_test.go:182`: uses the string `"swap_request_processing"` as
  an arbitrary job-name label for a generic `JobStatusManager` unit test, unrelated to the real job.
  Left as is.

All non-listed whitespace-only diff noise (trailing-space trims in `server.go`, `handler.go`,
`config.go`) is the repo's own format-on-save hook re-touching lines adjacent to the edits, not a
deliberate change.

## Confirmation run-table

| Command | Exit | Result |
|---|---|---|
| `export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH && go version` | 0 | `go version go1.24.2 darwin/arm64` |
| `go build ./...` | 0 | (no output = success) |
| `go test ./internal/handler/health/...` | 0 | `Go test: 6 passed in 1 packages` |
| `go test ./internal/store/swaprequest/...` | 0 | `Go test: No tests found` (package has no test file; touched file, honestly noted) |
| `go test ./internal/transport/http/...` | 0 | `Go test: No tests found` (package has no test file) |
| `go test ./internal/utils/config/...` | 0 | `Go test: 1 passed in 1 packages` |
| `go test ./internal/monitoring/...` | 0 | `Go test: 50 passed in 1 packages` |
| `go test ./internal/handler/swap/...` | 1 | build fails: `swap_info_integration_test.go` / `swap_info_timeout_test.go` reference `config.Bitcoin` and an old 7-arg `swap.New(...)` signature. **Confirmed pre-existing**: identical failure reproduced on a `git stash` back to untouched `origin/develop` (ff49a48), before any edit in this PR. |
| `go test ./internal/telemetry/...` | 1 | build fails: `btc_multi_endpoint_test.go` references `store.MockStore`/`telemetry.New` with a stale signature, unrelated to `ProcessSwapRequests`. **Confirmed pre-existing**: identical failure reproduced the same way on untouched `origin/develop`. |

Aggregate: `Go test: 60 passed, 2 failed in 11 packages`, both failures are pre-existing build
breakage in unrelated test files, verified byte-for-byte reproducible on the clean base before this
branch touched anything (see Negative control).

## Negative control

```
git grep -n "CreateSwapRequest"      # 0 hits in .go source (only historical docs/sessions/*.md)
git grep -n "ProcessSwapRequests"    # 0 hits in .go source (only historical docs/sessions/*.md)
git grep -n "\"/swap\"" internal/    # 0 hits; only /swap/generate-signature and /swap/info remain
```

Flow A intact:

```
$ grep -n "generate-signature\|/swap/info\|GenerateSignature" internal/transport/http/v1.go internal/handler/swap/interface.go
internal/transport/http/v1.go:24:   swap.POST("/generate-signature", h.SwapHandler.GenerateSignature)
internal/transport/http/v1.go:26:   swap.GET("/info", h.SwapHandler.Info)
internal/handler/swap/interface.go:6:   GenerateSignature(c *gin.Context)
```

## Reproduce

```
git clone <repo> && cd icy-backend
git checkout fix/retire-flow-b
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH
go version   # must print go1.24.2
go build ./...
go test ./internal/handler/health/... ./internal/store/swaprequest/... ./internal/transport/http/... \
        ./internal/utils/config/... ./internal/monitoring/...
```

To reproduce the pre-existing-breakage claim: `git stash` on this branch (or check out
`origin/develop` directly) and re-run `go test ./internal/handler/swap/...` / `go test
./internal/telemetry/...`, the same compile errors appear, unchanged by this PR.

## Rollback

`git revert <merge commit>`. Source-only change, no migration, no data change. The `swap_requests`
table and its model are untouched (kept for the pre-existing, already-orphaned `GetByIcyTx` /
`UpdateStatus` methods), so no schema rollback is needed either. Reverting restores the
`POST /api/v1/swap` route, the `CreateSwapRequest` handler, and the `ProcessSwapRequests` cron
exactly as they were, including CRIT-2/DF-85's dormant bug (which is why PR #34 stays open, not
superseded-and-deleted, until Han decides Flow B is gone for good).
