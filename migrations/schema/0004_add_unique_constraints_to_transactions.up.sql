-- Add unique constraint to onchain_icy_transactions
ALTER TABLE onchain_icy_transactions
ADD CONSTRAINT unique_icy_transaction_hash UNIQUE (transaction_hash);

-- Add unique constraint to onchain_btc_transactions
ALTER TABLE onchain_btc_transactions
ADD CONSTRAINT unique_btc_transaction_hash UNIQUE (transaction_hash);
