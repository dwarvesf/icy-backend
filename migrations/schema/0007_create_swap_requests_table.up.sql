CREATE TABLE swap_requests (
    id SERIAL PRIMARY KEY,
    icy_amount VARCHAR(255) NOT NULL,
    btc_address VARCHAR(255) NOT NULL,
    icy_tx VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_swap_requests_icy_tx ON swap_requests(icy_tx);
CREATE INDEX idx_swap_requests_status ON swap_requests(status);
