-- Remove network_fee column and foreign key from onchain_btc_processed_transactions
ALTER TABLE onchain_btc_processed_transactions
DROP CONSTRAINT IF EXISTS fk_btc_processed_icy_swap_transaction,
DROP COLUMN IF EXISTS network_fee;
