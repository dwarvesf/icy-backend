# Proof of done, med/low hardening sweep (SG-13 / DF-84)

icy-swap-hardening SG-13 / DF-84. Residual Med/Low sweep across icy-backend and
mochi-pay-api. This doc covers icy-backend; the sibling repo's doc is
`mochi-pay-api/docs/verification/cors-medlow-sweep.md`.

## Checklist

| # | Item | Status | Evidence |
|---|---|---|---|
| 1 | CORS: no wildcard-with-credentials (Med-8) | verified-present | `internal/transport/http/http.go:25,29,35` reads `cfg.ApiServer.AllowedOrigins` (config/env-sourced, `ALLOWED_ORIGINS`), never a `"*"` literal. `internal/utils/config/config.go:42,104,176` shows the field is sourced from `os.Getenv("ALLOWED_ORIGINS")` / Vault KV, no default wildcard. Actual prod origin value is a deploy-time env/Vault entry, out of code scope. |
| 2 | Precision: big.Int/decimal end-to-end (Med-6) | fixed | `internal/handler/swap/swap.go:171-187` (`GenerateSignature` fee-vs-amount gate): was `btcDecimal.Mul(...).InexactFloat64()` compared against a `float64` amount; now stays in `decimal.Decimal` (`svcFeeDecimal`, `minFeeDecimal`, `btcDecimal.Sub(...).IsNegative()`). The **signed** amount (`serverSatBig`, `swap.go:140`) was already `big.Int` end-to-end from SG-03 and is unchanged. Commit `49a9bee`. |
| 3 | Signature not logged at Info (Low-10) | fixed | `internal/baserpc/baserpc.go:792` (`GenerateSignature`): `b.logger.Info("Swap signature generated", ...)` → `b.logger.Debug(...)`. Commit `3e9571e`. |
| 4a | Constant-time API-key compare (SG-03) | verified-present | `internal/transport/http/http.go:96`: `subtle.ConstantTimeCompare([]byte(apiKey), []byte(appConfig.ApiServer.ApiKey)) != 1`. |
| 4b | `onchain_btc_processed_transactions.swap_transaction_hash` UNIQUE (SG-05) | verified-present | `migrations/schema/0013_add_unique_swap_transaction_hash_to_btc_processed.up.sql`: `CREATE UNIQUE INDEX IF NOT EXISTS uniq_btc_processed_swap_transaction_hash ON onchain_btc_processed_transactions (swap_transaction_hash) WHERE swap_transaction_hash IS NOT NULL AND swap_transaction_hash <> ''`. |

### Note on item 2 scope

`internal/handler/swap/swap.go`'s `Info` handler (`min_icy_to_swap` estimate,
lines ~357-372) also does float64 rate math, but it is a **display-only
estimate** for the public `/swap/info` response, not an enforcement gate; the
actual swap amount is always derived and signed via `big.Int`
(`serverSatBig`) regardless of what `Info` displays. Left as-is per the
surgical-changes rule; flagged here for completeness rather than silently
rewritten.

## Recorded run

Toolchain: `export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH` → `go version` = `go1.24.2 darwin/arm64`.

| # | Command | Exit | Result |
|---|---|---|---|
| 1 | `go build ./...` | 0 | builds clean |
| 2 | `go test ./internal/baserpc/...` | 0 | 7 specs pass |
| 3 | `go test ./internal/handler/swap/...` | 1 (build failed) | pre-existing, see negative control below |

**Pre-existing build-broken test package (out of scope):** `go test
./internal/handler/swap/...` fails to build on this branch
(`swap_info_integration_test.go`, `swap_info_timeout_test.go`: `undefined:
config.Bitcoin`, mock-interface mismatches, unused var). Confirmed via `git
stash` that the **identical** build failure exists on untouched
`fix/swap-medlow-sweep` HEAD before this sweep's commits (stashed, re-ran, same
errors, then `git stash pop` to restore). Not touched, per the task's hard
test constraint (pre-existing broken test packages are out of scope).

The `internal/baserpc` package (which owns the signature-log fix) builds and
passes all 7 specs clean.

## Reproduce

```bash
cd icy-backend
export PATH=$HOME/.local/share/mise/installs/go/1.24.2/bin:$PATH
go build ./...
go test ./internal/baserpc/...
# confirm the swap test package is unrelated/pre-existing:
git stash && go test ./internal/handler/swap/... ; git stash pop
```
