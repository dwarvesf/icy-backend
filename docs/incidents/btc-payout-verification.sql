-- BTC Payout Verification for Duplicate Swaps
-- User: 0x5155b007D5C1AFe88912e52FABda1c7ED86baae0
-- Date: 2026-04-08

-- =====================================================
-- Query 1: Get swaps with corresponding BTC payouts
-- =====================================================
SELECT 
    swap.id as swap_id,
    swap.transaction_hash as swap_tx_hash,
    swap.icy_amount,
    swap.btc_amount as expected_btc_sats,
    swap.from_address,
    swap.btc_address,
    swap.created_at as swap_created_at,
    btc.id as btc_id,
    btc.btc_transaction_hash,
    btc.subtotal as btc_subtotal_sats,
    btc.service_fee as btc_service_fee_sats,
    btc.total as btc_total_sats,
    btc.status,
    btc.processed_at
FROM onchain_icy_swap_transactions swap
LEFT JOIN onchain_btc_processed_transactions btc 
    ON swap.transaction_hash = btc.swap_transaction_hash
WHERE swap.transaction_hash IN (
    '0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35',
    '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a'
)
ORDER BY swap.created_at;

-- =====================================================
-- Query 2: Check all pending BTC transactions
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
-- Query 3: Check all BTC transactions for user's BTC address
-- =====================================================
SELECT 
    id,
    swap_transaction_hash,
    btc_address,
    subtotal,
    service_fee,
    total,
    btc_transaction_hash,
    status,
    created_at,
    processed_at
FROM onchain_btc_processed_transactions
WHERE btc_address = 'bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz'
ORDER BY created_at DESC;

-- =====================================================
-- Query 4: Get all swaps and their BTC payout status for this user
-- =====================================================
SELECT 
    swap.id as swap_id,
    swap.transaction_hash,
    swap.icy_amount,
    swap.btc_amount,
    swap.created_at as swap_time,
    CASE 
        WHEN btc.id IS NULL THEN 'NO_BTC_RECORD'
        WHEN btc.status = 'pending' THEN 'PENDING'
        WHEN btc.status = 'completed' THEN 'COMPLETED'
        WHEN btc.status = 'failed' THEN 'FAILED'
        ELSE 'UNKNOWN'
    END as payout_status,
    btc.btc_transaction_hash,
    btc.total as btc_paid,
    btc.processed_at
FROM onchain_icy_swap_transactions swap
LEFT JOIN onchain_btc_processed_transactions btc 
    ON swap.transaction_hash = btc.swap_transaction_hash
WHERE swap.from_address = '0x5155b007D5C1AFe88912e52FABda1c7ED86baae0'
ORDER BY swap.created_at DESC;

-- =====================================================
-- Query 5: Verify ICY amounts
-- =====================================================
SELECT 
    from_address,
    COUNT(*) as total_swaps,
    SUM(CAST(icy_amount AS BIGINT)) as total_icy_wei,
    SUM(CAST(btc_amount AS BIGINT)) as total_btc_sats,
    array_agg(DISTINCT status) as payout_statuses
FROM onchain_icy_swap_transactions swap
LEFT JOIN onchain_btc_processed_transactions btc 
    ON swap.transaction_hash = btc.swap_transaction_hash
WHERE from_address = '0x5155b007D5C1AFe88912e52FABda1c7ED86baae0'
    AND swap.created_at >= '2026-04-01'
    AND swap.created_at < '2026-04-02'
GROUP BY from_address;