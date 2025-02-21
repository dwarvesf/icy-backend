CREATE TABLE onchain_btc_processed_transactions (
    id SERIAL PRIMARY KEY,
    icy_transaction_hash VARCHAR(255) DEFAULT NULL,
    btc_transaction_hash VARCHAR(255),
    processed_at TIMESTAMP DEFAULT NULL,
    amount VARCHAR(255),
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_onchain_btc_processed_transactions_icy_hash 
ON onchain_btc_processed_transactions (icy_transaction_hash);
