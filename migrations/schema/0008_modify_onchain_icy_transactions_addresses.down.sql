-- Revert changes to onchain_icy_transactions table
ALTER TABLE onchain_icy_transactions
ADD COLUMN IF NOT EXISTS other_address VARCHAR(255),
DROP COLUMN IF EXISTS to_address,
DROP COLUMN IF EXISTS from_address;
