-- Investigate Duplicate BTC Transaction Hash
-- User: 0x5155b007D5C1AFe88912e52FABda1c7ED86baae0
-- Date: 2026-04-08

-- =====================================================
-- Query 1: Find all records with duplicate BTC tx hash
-- =====================================================
SELECT 
    btc.id,
    btc.swap_transaction_hash,
    btc.btc_address,
    btc.subtotal,
    btc.service_fee,
    btc.total,
    btc.btc_transaction_hash,
    btc.status,
    btc.created_at,
    btc.processed_at,
    swap.icy_amount,
    swap.from_address
FROM onchain_btc_processed_transactions btc
JOIN onchain_icy_swap_transactions swap 
    ON btc.swap_transaction_hash = swap.transaction_hash
WHERE btc.btc_transaction_hash = '0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61'
ORDER BY btc.created_at;

-- =====================================================
-- Query 2: Find all duplicates (same BTC tx hash used multiple times)
-- =====================================================
SELECT 
    btc.btc_transaction_hash,
    COUNT(*) as usage_count,
    array_agg(btc.swap_transaction_hash) as swap_hashes,
    array_agg(btc.subtotal) as amounts,
    SUM(CAST(btc.subtotal AS BIGINT)) as total_satoshis
FROM onchain_btc_processed_transactions btc
WHERE btc.btc_transaction_hash IS NOT NULL
    AND btc.status = 'completed'
    AND btc.btc_transaction_hash != ''
GROUP BY btc.btc_transaction_hash
HAVING COUNT(*) > 1
ORDER BY usage_count DESC;

-- =====================================================
-- Query 3: Get the actual BTC amount sent
-- =====================================================
SELECT 
    '0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61' as btc_tx_hash,
    COUNT(*) as number_of_swaps,
    SUM(CAST(subtotal AS BIGINT)) as sum_of_subtotals,
    SUM(CAST(service_fee AS BIGINT)) as sum_of_fees,
    SUM(CAST(total AS BIGINT)) as sum_of_totals
FROM onchain_btc_processed_transactions
WHERE btc_transaction_hash = '0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61';

-- =====================================================
-- Query 4: Timeline of processing
-- =====================================================
SELECT 
    'SWAP' as type,
    transaction_hash,
    icy_amount,
    created_at
FROM onchain_icy_swap_transactions
WHERE transaction_hash IN (
    '0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35',
    '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a'
)

UNION ALL

SELECT 
    'BTC' as type,
    swap_transaction_hash,
    subtotal,
    processed_at
FROM onchain_btc_processed_transactions
WHERE swap_transaction_hash IN (
    '0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35',
    '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a'
)
ORDER BY created_at;

-- =====================================================
-- Query 5: Check if this is a pattern (other duplicate BTC txs)
-- =====================================================
SELECT 
    btc.btc_transaction_hash,
    COUNT(*) as duplicate_count,
    array_agg(DISTINCT trunc(extract(epoch FROM (btc.processed_at - lag_processed_at)))) as time_gaps_seconds
FROM (
    SELECT 
        btc_transaction_hash,
        processed_at,
        LAG(processed_at) OVER (ORDER BY processed_at) as lag_processed_at
    FROM onchain_btc_processed_transactions
    WHERE status = 'completed'
        AND btc_transaction_hash IS NOT NULL
        AND btc_transaction_hash != ''
) btc
WHERE btc.btc_transaction_hash IS NOT NULL
GROUP BY btc.btc_transaction_hash
HAVING COUNT(*) > 1
ORDER BY duplicate_count DESC;

-- =====================================================
-- Query 6: Check processing logs ( timestamps around 12:21:48)
-- =====================================================
-- Run this in application logs:
-- grep "12:21:4" /var/log/icy-backend/*.log | grep -E "ProcessPendingBtcTransactions|Send BTC|processed_at"