-- Investigation Queries for Duplicate ICY Swap Issue
-- User: 0x5155b007D5C1AFe88912e52FABda1c7ED86baae0
-- BTC Address: bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz
-- Date: 2026-04-01

-- =====================================================
-- Query 1: Get all swap transactions for the user
-- =====================================================
SELECT 
    id,
    transaction_hash,
    icy_amount,
    btc_amount,
    from_address,
    btc_address,
    created_at
FROM onchain_icy_swap_transactions 
WHERE from_address = '0x5155b007D5C1AFe88912e52FABda1c7ED86baae0'
ORDER BY created_at DESC;

-- =====================================================
-- Query 2: Check if both swaps have BTC processed records
-- =====================================================
SELECT 
    swap.id as swap_id,
    swap.transaction_hash as swap_tx_hash,
    swap.icy_amount,
    swap.btc_amount as expected_btc,
    btc.id as btc_id,
    btc.swap_transaction_hash,
    btc.btc_transaction_hash,
    btc.btc_address,
    btc.subtotal,
    btc.service_fee,
    btc.total,
    btc.status,
    btc.created_at,
    btc.processed_at
FROM onchain_icy_swap_transactions swap
LEFT JOIN onchain_btc_processed_transactions btc 
    ON swap.transaction_hash = btc.swap_transaction_hash
WHERE swap.from_address = '0x5155b007D5C1AFe88912e52FABda1c7ED86baae0'
    AND swap.created_at >= '2026-04-01'
ORDER BY swap.created_at;

-- =====================================================
-- Query 3: Find all pending BTC transactions
-- =====================================================
SELECT 
    id,
    swap_transaction_hash,
    btc_address,
    subtotal,
    service_fee,
    total,
    status,
    created_at,
    processed_at
FROM onchain_btc_processed_transactions
WHERE status = 'pending'
    AND created_at >= '2026-04-01'
ORDER BY created_at DESC;

-- =====================================================
-- Query 4: Find all transactions with same BTC address
-- =====================================================
SELECT 
    btc.id,
    btc.swap_transaction_hash,
    btc.btc_address,
    btc.subtotal,
    btc.total,
    btc.status,
    btc.created_at,
    swap.from_address,
    swap.icy_amount
FROM onchain_btc_processed_transactions btc
JOIN onchain_icy_swap_transactions swap 
    ON btc.swap_transaction_hash = swap.transaction_hash
WHERE btc.btc_address = 'bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz'
ORDER BY btc.created_at DESC;

-- =====================================================
-- Query 5: Check for rapid successive swaps (within 1 minute)
-- =====================================================
WITH ranked_swaps AS (
    SELECT 
        id,
        transaction_hash,
        from_address,
        icy_amount,
        created_at,
        LAG(created_at) OVER (PARTITION BY from_address ORDER BY created_at) as prev_created_at,
        LAG(icy_amount) OVER (PARTITION BY from_address ORDER BY created_at) as prev_icy_amount,
        ROW_NUMBER() OVER (PARTITION BY from_address, DATE(created_at) ORDER BY created_at) as swap_order
    FROM onchain_icy_swap_transactions
    WHERE created_at >= '2026-04-01'
)
SELECT 
    from_address,
    id,
    transaction_hash,
    icy_amount,
    created_at,
    prev_created_at,
    prev_icy_amount,
    swap_order,
    EXTRACT(EPOCH FROM (created_at - prev_created_at)) as seconds_since_last_swap
FROM ranked_swaps
WHERE from_address = '0x5155b007D5C1AFe88912e52FABda1c7ED86baae0'
    AND swap_order > 1
ORDER BY created_at;

-- =====================================================
-- Query 6: Check contract balance/treasury
-- =====================================================
SELECT * FROM icy_locked_treasuries ORDER BY created_at DESC LIMIT 10;

-- =====================================================
-- Query 7: Find duplicate swap attempts by same user
-- =====================================================
SELECT 
    from_address,
    DATE(created_at) as swap_date,
    COUNT(*) as total_swaps,
    SUM(CAST(icy_amount AS BIGINT)) as total_icy_amount,
    array_agg(DISTINCT btc_address) as btc_addresses,
    array_agg(transaction_hash) as tx_hashes,
    array_agg(created_at) as timestamps
FROM onchain_icy_swap_transactions
WHERE created_at >= '2026-04-01'
GROUP BY from_address, DATE(created_at)
HAVING COUNT(*) > 1
ORDER BY swap_date DESC, total_swaps DESC;

-- =====================================================
-- Query 8: Check for any failed BTC transactions
-- =====================================================
SELECT 
    id,
    swap_transaction_hash,
    btc_address,
    subtotal,
    service_fee,
    total,
    status,
    created_at,
    processed_at
FROM onchain_btc_processed_transactions
WHERE status = 'failed'
    AND created_at >= '2026-04-01'
ORDER BY created_at DESC;