-- Add network_fee column to onchain_btc_processed_transactions
ALTER TABLE onchain_btc_processed_transactions
ADD COLUMN network_fee VARCHAR(255) DEFAULT '0';
