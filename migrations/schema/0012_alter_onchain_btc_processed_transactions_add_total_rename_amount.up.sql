ALTER TABLE onchain_btc_processed_transactions
RENAME COLUMN amount TO subtotal;

ALTER TABLE onchain_btc_processed_transactions
ADD COLUMN total VARCHAR(255) DEFAULT '0';

UPDATE onchain_btc_processed_transactions SET total = subtotal;