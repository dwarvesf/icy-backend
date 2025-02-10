-- Add block_number column to onchain_icy_transactions
ALTER TABLE onchain_icy_transactions
ADD COLUMN block_number BIGINT;
