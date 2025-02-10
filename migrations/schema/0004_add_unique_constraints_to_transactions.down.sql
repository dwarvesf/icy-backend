-- Remove unique constraints from onchain_icy_transactions
ALTER TABLE onchain_icy_transactions
DROP CONSTRAINT IF EXISTS unique_icy_transaction_hash;

-- Remove unique constraints from onchain_btc_transactions
ALTER TABLE onchain_btc_transactions
DROP CONSTRAINT IF EXISTS unique_btc_transaction_hash;
