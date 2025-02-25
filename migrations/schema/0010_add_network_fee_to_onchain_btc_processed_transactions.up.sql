-- Add network_fee column to onchain_btc_processed_transactions
ALTER TABLE onchain_btc_processed_transactions
ADD COLUMN network_fee VARCHAR(255) DEFAULT '0',
ADD CONSTRAINT fk_btc_processed_icy_swap_transaction
    FOREIGN KEY (swap_transaction_hash)
    REFERENCES onchain_icy_swap_transactions(transaction_hash)
    ON DELETE SET NULL;
