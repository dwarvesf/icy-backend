-- Defense-in-depth for exactly-once BTC settlement: at most one processed BTC
-- payout row may exist per swap event. Application-level idempotency (the atomic
-- claim in ProcessPendingBtcTransactions) is the primary guard; this UNIQUE
-- index is the database backstop.
--
-- Partial index: legacy rows with a NULL or empty swap_transaction_hash
-- (pre-swap-flow rows, or rows never linked to a swap) are exempt, so the
-- migration applies cleanly on existing data and does not force a value onto
-- unrelated rows. Cheap: a single index build on a low-cardinality column.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_btc_processed_swap_transaction_hash
ON onchain_btc_processed_transactions (swap_transaction_hash)
WHERE swap_transaction_hash IS NOT NULL AND swap_transaction_hash <> '';
