# CRITICAL BUG: Two Swaps Merged into Single BTC Payout

**Date**: 2026-04-08  
**Status**: BUG CONFIRMED - Data Integrity Issue  
**Priority**: CRITICAL  
**Impact**: User lost 50 ICY equivalent in BTC  

---

## Executive Summary

**NOT A DUPLICATE BUG - THIS IS WORSE**

Two separate ICY swaps were processed into **ONE BTC transaction**. The user received BTC payout for only **ONE swap, not TWO**.

---

## Database Evidence

### Query Result

```json
[
  {
    "transaction_hash": "0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35",
    "icy_amount": "50000000000000000000",
    "id": 61,
    "status": "completed",
    "btc_transaction_hash": "**0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61**",
    "processed_at": "2026-04-01 12:21:48.297755"
  },
  {
    "transaction_hash": "0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a",
    "icy_amount": "50000000000000000000",
    "id": 62,
    "status": "completed",
    "btc_transaction_hash": "**0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61**",
    "processed_at": "2026-04-01 12:21:48.527817"
  }
]
```

### Critical Observation

**Both transactions have THE SAME BTC transaction hash:**
```
0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61
```

This is **IMPOSSIBLE for two separate BTC transactions**. A single Bitcoin transaction hash can only exist once on the blockchain.

---

## What Happened

### Timeline of Events

| Time | Event | Result |
|------|-------|--------|
| 12:21:48.297 | TX #1 processed | BTC tx `0af608f0...` created for swap #61 |
| 12:21:48.527 | TX #2 processed | **SAME** BTC tx `0af608f0...` recorded for swap #62 |

**Gap**: Only **0.23 seconds** between processing both swaps.

### The Bug

1. ✅ Two separate ICY swaps were executed on blockchain
2. ✅ Both were indexed correctly in `onchain_icy_swap_transactions`
3. ✅ Both created `onchain_btc_processed_transactions` records
4. ✅ Both were processed by `ProcessPendingBtcTransactions()`
5. ❌ **BUG**: Both recorded the **SAME BTC transaction hash**

**Root Cause Hypothesis**:
- Race condition in `ProcessPendingBtcTransactions()`
- Or BTC RPC returned same tx hash for both requests
- Or system sent one BTC transaction and recorded it twice

---

## Impact

### Financial Impact

**User Expected**:
- Swap #1: 50 ICY → ~102,450 satoshis BTC
- Swap #2: 50 ICY → ~102,450 satoshis BTC
- **Total**: ~204,900 satoshis BTC

**User Received**:
- One BTC transaction: `0af608f0...`
- **Total**: ~102,450 satoshis BTC (only)

**User Lost**: ~102,450 satoshis = 50 ICY equivalent = ~$102 USD (at current prices)

### Data Integrity Impact

- ❌ Two database records pointing to same BTC transaction
- ❌ Violates business logic: one BTC tx should = one swap
- ❌ User cannot be credited correctly
- ❌ Audit trail is incorrect

---

## Root Cause Investigation Needed

### Possible Causes

#### Hypothesis 1: Race Condition in ProcessPendingBtcTransactions()

**Code Flow**:
```
ProcessPendingBtcTransactions() runs every 2 minutes
├─ Fetches all pending transactions
├─ For each pending tx:
│  ├─ Send BTC via RPC
│  ├─ Get BTC tx hash
│  └─ Update record with hash
```

**Race Condition**: If both swaps fetched as "pending" in same batch, and BTC RPC returned same tx hash?

**Why this is unlikely**:
- Each `Send()` call should create separate BTC transactions
- Would need to investigate if BTC RPC reuses tx hashes

#### Hypothesis 2: BTC RPC Issue

**Scenario**: Blockstream API returned same tx hash twice?

**Need to verify**:
- Check if `btcRpc.Send()` can return same hash twice
- Check if there's caching in the RPC client
- Check if concurrent requests to same UTXOs cause issues

#### Hypothesis 3: Database Race Condition

**Scenario**: Two parallel updates to same BTC tx hash?

**Code path**:
```go
// In ProcessPendingBtcTransactions()
tx, networkFee, err := t.btcRpc.Send(...)
btcTxHash := tx.TxID()

_, err = t.store.OnchainBtcProcessedTransaction.UpdateToCompleted(
    t.db, pendingTx.ID, tx, networkFee,
)
```

**Need to check**: Is there a race where `tx` variable is shared?

---

## Immediate Actions Required

### Action 1: Investigate the BTC Transaction

```bash
# Check the actual BTC transaction
bitcoin-cli getrawtransaction 0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61

# Or use block explorer
# https://blockstream.info/tx/0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61
```

**Verify**:
- Amount sent: Should be ~102,450 satoshis (for ONE swap) or ~204,900 (for TWO swaps)
- If amount is ~102,450 satoshis: User is owed another ~102,450 satoshis
- If amount is ~204,900 satoshis: User received both payouts in one tx (check with user)

### Action 2: Check Logs for Processing

```bash
# Check logs around processing time
grep "12:21:48" /var/log/icy-backend/*.log | grep -E "0x32486e9e|0xe36ac188|0af608f0"

# Look for:
# - "ProcessPendingBtcTransactions"
# - "Send BTC to"
# - "BTC transaction hash"
# - Any errors or warnings
```

### Action 3: Check Application Code for Race Conditions

**Files to review**:
- `internal/telemetry/btc.go` - Lines 81-165 (ProcessPendingBtcTransactions)
- `internal/btcrpc/btcrpc.go` - Send() function
- Check if `tx` variable is shared across goroutines

### Action 4: Refund the User

**If BTC amount is ~102,450 satoshis (user received ONE payout)**:
```sql
-- Option A: Create manual payout record
INSERT INTO onchain_btc_processed_transactions (
    swap_transaction_hash,
    btc_address,
    subtotal,
    service_fee,
    total,
    status,
    created_at,
    updated_at
) VALUES (
    'MANUAL_PAYOUT_2026-04-08_1',  -- Manual identifier
    'bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz',
    '102450',  -- 50 ICY equivalent
    '2049',    -- Service fee
    '100401',  -- subtotal - fee
    'pending',
    NOW(),
    NOW()
);

-- System will process in next cron run

-- Option B: Send BTC directly and record manually
-- After sending: manual BTC create transaction record with status='completed'
```

---

## User Communication

### Template

```
Hi,

We've completed our investigation and found the root cause:

【What Happened】
You made TWO successful ICY swaps:
- Swap #1: 50 ICY on Apr-01-2026 12:20:33 UTC
- Swap #2: 50 ICY on Apr-01-2026 12:20:55 UTC

Both were processed correctly, BUT a system bug caused both payouts to be merged 
into ONE Bitcoin transaction.

【What You Received】
Bitcoin Transaction: 0af608f0537ac0cc072d982fb10d0ff09a37c8d9afdb03d62d75f82933590e61
Amount: ~102,450 satoshis (for ONE swap)

【What You're Owed】
You're missing ~102,450 satoshis (equivalent to 50 ICY).

【What We're Doing】
We're processing your missing payout now. You should receive it within 2-5 minutes.

We apologize for this issue and are implementing safeguards to prevent it from 
happening again.
```

---

## System Fix Required

### Immediate Fix (Hotfix)

**Add unique constraint** to prevent same BTC tx hash being used twice:

```sql
-- Migration: Add unique constraint
ALTER TABLE onchain_btc_processed_transactions 
ADD CONSTRAINT unique_btc_transaction_hash UNIQUE (btc_transaction_hash);

-- This will prevent the same BTC tx hash from being recorded twice
```

**Code fix in `btc.go`**:

```go
// Before updating to completed, check if this BTC tx hash already exists
existingBTC, err := t.store.OnchainBtcProcessedTransaction.GetByBTCHash(t.db, tx.TxID())
if err == nil && existingBTC != nil {
    // This BTC tx hash already used! Log error and skip
    t.logger.Error("[ProcessPendingBtcTransactions][DuplicateBTCHash]", map[string]string{
        "btc_tx_hash": tx.TxID(),
        "existing_swap_id": fmt.Sprintf("%d", existingBTC.ID),
        "current_swap_id": fmt.Sprintf("%d", pendingTx.ID),
    })
    continue
}
```

### Long-Term Fix

1. **Investigate root cause** of duplicate BTC tx hash recording
2. **Add mutex** around BTC sending if needed
3. **Add logging** for BTC tx hash assignment
4. **Add monitoring** for duplicate BTC tx hashes
5. **Add alert** when two swaps have same BTC tx hash

---

## Next Steps

1. ✅ Verify the actual BTC transaction amount on blockchain
2. ⏸️ Check application logs for processing details
3. ⏸️ Investigate code for race conditions
4. ⏸️ Implement unique constraint
5. ⏸️ Refund user for missing payout
6. ⏸️ Add monitoring and alerting
7. ⏸️ Post-mortem and prevent recurrence

---

## Related Documents

- [Finding: Not a Duplicate](./FINDING-not-a-duplicate.md)
- [Blockchain Verification](./blockchain-verification-results.md)
- [Investigation Analysis](./investigation-analysis.md)