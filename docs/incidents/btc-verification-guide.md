# BTC Payout Verification Guide

**Date**: 2026-04-08  
**User**: `0x5155b007D5C1AFe88912e52FABda1c7ED86baae0`  
**BTC Address**: `bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz`

---

## Duplicate Swap Transactions

| TX # | Transaction Hash | ICY Amount | Timestamp (UTC) | Gap |
|-----|------------------|------------|-----------------|-----|
| 1 | `0x016db717...` | 20 ICY | 2026-04-01 11:47:34 | - |
| 2 | `0x32486e9e...` | **50 ICY** | 2026-04-01 12:21:03.916673 | - |
| 3 | `0xe36ac188...` | **50 ICY** | 2026-04-01 12:21:04.094881 | **0.178s** |
| 4 | `0xb14c9191...` | 30 ICY | 2026-04-01 12:22:40 | - |

**Total**: 150 ICY attempted  
**Issue**: TXs #2 and #3 are only 0.178 seconds apart

---

## Verification Methods

### Method 1: Direct Database Query

Run the queries from `btc-payout-verification.sql`:

```bash
psql -h <DB_HOST> -U <DB_USER> -d icy_backend_local -f docs/incidents/btc-payout-verification.sql
```

**Key queries**:
1. Get swaps with BTC payouts (Query 1)
2. Check pending transactions (Query 2)
3. Verify by BTC address (Query 3)

### Method 2: Blockchain Explorer Verification

#### Base Mainnet Explorer (Basescan)

**TX #2 (50 ICY)**:
```
https://basescan.org/tx/0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35
```

**TX #3 (50 ICY)**:
```
https://basescan.org/tx/0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a
```

**What to check**:
1. ✅ Both transactions are confirmed
2. ✅ Both emitted `Swap` events
3. ✅ Both deduct 50 ICY from user's wallet
4. ✅ Both transfer ICY to swap contract

#### Bitcoin Blockchain Explorer

**User's BTC Address**:
```
https://blockstream.info/address/bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz
```

**What to check**:
1. Total BTC received on April 1st
2. Number of incoming transactions
3. Transaction amounts

### Method 3: Application Logs Check

Check application logs for April 1st:

```bash
# If logs stored in files
grep "0x32486e9e\|0xe36ac188" /var/log/icy-backend/*.log

# If using centralized logging (e.g., Elasticsearch)
# Search for transaction hashes
```

**Log patterns to find**:
```
[IndexIcySwapTransaction][SwapProcessed] tx_hash=0x32486e9e...
[IndexIcySwapTransaction][SwapProcessed] tx_hash=0xe36ac188...
[ProcessPendingBtcTransactions] BTC sent successfully btc_address=bc1q6wsf... amount=102458 tx=<btc_tx_hash>
[ProcessPendingBtcTransactions] BTC sent successfully btc_address=bc1q6wsf... amount=102458 tx=<btc_tx_hash>
```

---

## Expected Outcomes

### Scenario A: Both BTC Transactions Sent

**If** both swap transactions resulted in BTC payouts:

**Evidence**:
- ✅ Both have `status = 'completed'` in `onchain_btc_processed_transactions`
- ✅ Both have `btc_transaction_hash` populated
- ✅ Both have `processed_at` timestamps

**Interpretation**:
- User submitted two separate swaps
- Both were processed correctly
- User likely received BTC for both
- User confusion about "duplicate"

**Action**:
- Contact user to check BTC wallet for both transactions
- Provide transaction hashes for both BTC payments
- No refund needed

**User Communication**:
```
Hi,

We've verified your swap transactions and found that both 50 ICY swaps were processed:

1. TX: 0x32486e9e... → 102,458 satoshis (minus fees) sent to bc1q6wsf...
2. TX: 0xe36ac188... → 102,458 satoshis (minus fees) sent to bc1q6wsf...

Please check your BTC wallet for the receiving address. You should see two incoming 
transactions on April 1st, 2026.

If you only see one transaction, please provide the transaction IDs from your wallet 
so we can investigate further.
```

---

### Scenario B: Only One BTC Transaction Sent

**If** only one swap resulted in BTC payout:

**Evidence**:
- ❌ One has `status = 'completed'` with `btc_transaction_hash`
- ❌ Other has `status = 'pending'` with no `btc_transaction_hash`
- ❌ Or one has `status = 'failed'`

**Interpretation**:
- System processed first swap correctly
- System failed to process second swap
- User is owed 50 ICY equivalent in BTC

**Action**:
- Process manual payout for missed swap
- Create compensation transaction
- Refund 50 ICY equivalent in BTC

**Manual Payout Process**:

```sql
-- Check pending transaction
SELECT * FROM onchain_btc_processed_transactions 
WHERE swap_transaction_hash = '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a';

-- If pending, the system will auto-process on next cron run
-- If failed, create manual payout record
INSERT INTO onchain_btc_processed_transactions (
    swap_transaction_hash,
    btc_address,
    subtotal,
    service_fee,
    total,
    status,
    created_at
) VALUES (
    '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a',
    'bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz',
    '102458',  -- Same as other swap
    '<service_fee>',
    '<total>',
    'pending',
    NOW()
);

-- System will process in next cron run (every 2 minutes)
```

**User Communication**:
```
Hi,

We've identified the issue. You submitted two swaps, but only one was processed for BTC payout.

Missing payout:
- TX: 0xe36ac188...
- Amount: 50 ICY (≈102,458 satoshis after fees)

We're processing the manual payout now. You should receive the BTC within 2-5 minutes.

We apologize for the inconvenience.
```

---

### Scenario C: Neither TX Processed (Both Pending)

**If** both swaps are still pending:

**Evidence**:
- ❌ Both have `status = 'pending'`
- ❌ No `btc_transaction_hash`
- ❌ No `processed_at`

**Possible Causes**:
1. Circuit breaker triggered
2. BTC RPC connection issues
3. Insufficient BTC in treasury
4. Processing job not running

**Action**:
- Check application logs for errors
- Verify BTC treasury balance
- Restart processing if needed

**Debug Commands**:
```bash
# Check if cron job is running
# Look for: [ProcessPendingBtcTransactions] Start processing...

# Check BTC treasury balance
# Use Base RPC to check contract balance

# Check for circuit breaker status
# Look for: "circuit breaker is open" in logs
```

---

## Verification Checklist

### Database Checks

- [ ] Run Query 1: Get swaps with BTC payouts
- [ ] Run Query 2: Check pending BTC transactions
- [ ] Run Query 3: Check BTC address history
- [ ] Run Query 4: Get all user swaps with status
- [ ] Run Query 5: Verify total ICY amounts

### Blockchain Checks

- [ ] Verify TX #2 on Base explorer
- [ ] Verify TX #3 on Base explorer
- [ ] Check BTC address for incoming transactions
- [ ] Count number of BTC transactions on April 1st
- [ ] Verify transaction amounts match expected

### Application Checks

- [ ] Check application logs for processing
- [ ] Verify no errors in processing logs
- [ ] Check circuit breaker status
- [ ] Verify cron job is running
- [ ] Check BTC treasury balance

---

## Investigation Findings Template

### If Both BTC Sent:

```
✅ Investigation completed: Both BTC transactions sent

User: 0x5155b007D5C1AFe88912e52FABda1c7ED86baae0
BTC Address: bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz

Swap TX #1: 0x32486e9e...
- Status: Completed
- BTC TX: <hash>
- Amount: <satoshis> satoshis
- Processed: <timestamp>

Swap TX #2: 0xe36ac188...
- Status: Completed
- BTC TX: <hash>
- Amount: <satoshis> satoshis
- Processed: <timestamp>

User received BTC for both swaps. No action needed.
```

### If One BTC Missing:

```
❌ Investigation completed: One BTC transaction missing

User: 0x5155b007D5C1AFe88912e52FABda1c7ED86baae0
BTC Address: bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz

Swap TX #1: 0x32486e9e...
- Status: Completed
- BTC TX: <hash>
- Amount: <satoshis> satoshis
- Processed: <timestamp>

Swap TX #2: 0xe36ac188...
- Status: <pending/failed>
- BTC TX: <none>
- Amount: <satoshis> satoshis
- Processed: <not processed>

ACTION REQUIRED:
- Manual payout needed for TX #2
- Create pending BTC transaction record
- System will auto-process in next cron run
```

---

## Follow-up Actions

### After Verification

1. **Update Incident Report** (`2026-04-01-duplicate-icy-swap-transactions.md`)
   - Add verification results
   - Update status (resolved/in-progress/needs-action)
   - Document outcome

2. **User Communication**
   - Send findings to user
   - Provide transaction hashes
   - Request wallet verification if needed

3. **Compensation** (if applicable)
   - Process manual BTC payout
   - Create transaction record
   - Notify user

4. **System Improvements**
   - Implement rate limiting (P1)
   - Add balance monitoring (P1)
   - Create transaction status endpoint (P2)

---

## Contact & Escalation

**Database Issues**: If database queries timeout, use blockchain explorer verification as primary method.

**Need Manual Payout**: Contact engineering team to create manual BTC transaction record.

**User Disputes**: If user disputes findings, request:
1. Screenshot of BTC wallet showing transaction history
2. Transaction IDs from their wallet
3. Confirmation of receiving address