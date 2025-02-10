-- Remove block_number column from onchain_icy_transactions
ALTER TABLE onchain_icy_transactions
DROP COLUMN IF EXISTS block_number;
