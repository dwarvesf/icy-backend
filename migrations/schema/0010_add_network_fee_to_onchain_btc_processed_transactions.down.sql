-- Remove network_fee column from onchain_btc_processed_transactions
ALTER TABLE onchain_btc_processed_transactions
DROP COLUMN IF EXISTS network_fee;
