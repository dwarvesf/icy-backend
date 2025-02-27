ALTER TABLE onchain_btc_processed_transactions
DROP COLUMN IF EXISTS total;

ALTER TABLE onchain_btc_processed_transactions
RENAME COLUMN subtotal TO amount;
