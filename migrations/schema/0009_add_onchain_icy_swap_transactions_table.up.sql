CREATE TABLE IF NOT EXISTS onchain_icy_swap_transactions (
    id SERIAL PRIMARY KEY,
    transaction_hash VARCHAR(66) NOT NULL,
    block_number BIGINT NOT NULL,
    icy_amount VARCHAR(78) NOT NULL,
    from_address TEXT DEFAULT NULL,
    btc_address TEXT NOT NULL,
    btc_amount VARCHAR(78) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(transaction_hash)
);
