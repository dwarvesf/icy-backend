-- Drop otheraddress column and add to_address and from_address columns
ALTER TABLE onchain_icy_transactions
DROP COLUMN IF EXISTS other_address,
ADD COLUMN IF NOT EXISTS to_address VARCHAR(255),
ADD COLUMN IF NOT EXISTS from_address VARCHAR(255);
