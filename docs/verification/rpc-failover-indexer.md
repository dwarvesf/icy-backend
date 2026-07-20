# Incident + Proof of Done, Base RPC Failover and the Swap Indexer Outage (2026-07-20)

**Outcome:** the swap-event indexer and the rate endpoint survive a rate-limited
or capability-degraded Base RPC endpoint instead of wedging on it. Two root
causes fixed: an off-by-one that made every ranged `eth_getLogs` illegal, and a
swap-event scan pinned to a single endpoint with no failover.

**Tags:** `v0.4.11` (getLogs range + baserpc failover), `v0.4.14`
(swap-indexer failover). **Go:** 1.24.2.

---

## 1. What happened

`icy.so` showed "rate unavailable" and the swap indexer failed every tick. The
symptom chain:

1. The prod Base RPC list was `alchemy, mainnet.base.org`. The **Alchemy account
   had dropped to the Free tier**, which caps `eth_getLogs` to a **10-block**
   range. Any catch-up scan asks for up to 10,000 blocks, so every scan 400'd
   with `-32600 "Under the Free tier plan..."`.
2. Those failures held the `base_rpc` circuit breaker **open**, which also
   blocked the unrelated `totalSupply` read the oracle needs for the ICY rate.
   No supply figure means `/swap/info` returns `partial_data: true` and the
   frontend correctly pauses swapping.
3. Even after reordering to a public-RPC-primary list, a **second bug** surfaced:
   the chunk loop computed `end = start + 10000` (inclusive), a **10,001-block**
   span, which the public Base RPC rejects with `-32614` (exactly-10,000 cap).
4. And a **third**: the swap-event scan (`IndexIcySwapTransaction`) built its own
   contract binding from the shared `ethclient` and called `FilterSwap` raw, no
   retry. When other operations rotated the shared client onto the free-tier
   Alchemy fallback (where `eth_call` succeeds but ranged `eth_getLogs` 400s),
   the swap scan stayed pinned there and failed every tick while the rate looked
   healthy, because nothing forced a rotation back.

## 2. Fixes

| # | Fix | Location |
|---|-----|----------|
| 1 | Endpoint list reordered: `mainnet.base.org` primary (10k getLogs), Alchemy fallback (rescues `eth_call` when the public RPC 429s). Both are needed: the public RPC alone 429'd with no fallback. | Vault `kv/icy-backend/prod` `BLOCKCHAIN_BASE_RPC_ENDPOINTS` |
| 2 | `blockRanges(start, latest, maxRange)`: inclusive chunks of at most `maxRange` blocks (`end = start + maxRange - 1`). | `internal/baserpc/baserpc.go` |
| 3 | Same off-by-one existed inline in the swap indexer; fixed to `start + maxRange - 1`. | `internal/telemetry/swap.go` |
| 4 | `BaseRPC.FilterSwapEvents` wraps the scan in `withRetry` and **rebuilds the contract binding on every attempt**, so a scan that fails on one endpoint rotates and retries. The indexer no longer builds its own binding from `Client()`. | `internal/baserpc/baserpc.go`, `internal/telemetry/swap.go` |

## 3. Proof

- **RPC caps, verified live** (2026-07-20): a 10,000-block `eth_getLogs` on
  `mainnet.base.org` returns in ~1.2s; a 10,001-block span returns `-32614`. The
  free-tier Alchemy endpoint returns `-32600` (10-block cap) on the same query.
- **Recovery, prod:** after `v0.4.11` + the endpoint reorder, `/swap/info`
  returned `partial_data: false` with a whole rate; the swap indexer logged
  `Job completed successfully`.
- **Failover, prod (`v0.4.14`):** over five consecutive ticks,
  `icy_swap_transaction_indexing`, `icy_transaction_indexing` (BTC), and
  `btc_pending_transaction_processing` all reported `Job completed successfully`
  with **zero** `Job failed`, and the rate stayed whole.
- **Unit:** `internal/baserpc/failover_test.go` covers `switchEndpoint` rotation
  (skip-failed, retry-expired, all-failed-still-advances, single-endpoint-errors);
  `internal/baserpc/block_ranges_test.go` covers the inclusive-cap arithmetic
  with an exact-boundary negative control.

## 4. Standing risk / follow-ups

- **Data provider:** while the Alchemy account stays on Free tier it cannot serve
  the data path (getLogs). The public RPC carries current volume; restore Alchemy
  as primary only after a PAYG upgrade lifts the getLogs cap.
- **Sibling call sites (known gap):** `IndexIcySwapTransaction` still calls
  `TransactionReceipt` / `BlockNumber` / `TransactionByHash` directly on
  `Client()` with no retry (a `BlockNumber` failure returns before
  `FilterSwapEvents`'s failover engages). Wrapping those three and dropping
  `Client()` off `IBaseRPC` is the durable consolidation. Flagged by the
  architecture review; tracked, not yet done.
