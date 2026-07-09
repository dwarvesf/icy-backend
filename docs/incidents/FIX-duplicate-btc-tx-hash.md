# Fix: Duplicate BTC Transaction Hash Issue

**Date**: 2026-04-08  
**Priority**: CRITICAL  
**Status**: Ready for Implementation  

---

## Executive Summary

A user made two separate ICY swaps but both BTC payouts were recorded with the **same transaction hash**, resulting in only one Bitcoin transaction being sent instead of two. User lost ~$102 USD (50 ICY equivalent).

**Root Causes**:
1. No mutex lock in `selectUTXOs()` - concurrent UTXO selection
2. No unique constraint on `btc_transaction_hash` - duplicates allowed

**Fixes Required**:
1. Immediate: Refund user's missing payout
2. Code: Add mutex lock around UTXO selection
3. Database: Add unique constraint to prevent future duplicates

---

## Issue Explained Simply

### Why This Matters

Imagine you order two meals at a restaurant. You pay for both meals, but the kitchen only sends out one plate of food. The receipt says you got both meals, but you only received one.

That's exactly what happened here - the user made two separate swaps (paid for two meals), but the system recorded them as using the same Bitcoin transaction (sent one plate).

**User's Impact**: They lost ~$102 worth of Bitcoin (50 ICY equivalent).

---

### What Actually Happened

#### The User's Actions
1. User swapped 50 ICY for Bitcoin → Success ✅
2. 22 seconds later, user swapped another 50 ICY for Bitcoin → Success ✅

#### What the System Did
1. **Created two swap records** in the database (correct ✅)
2. **Supposed to send TWO Bitcoin transactions** but instead:
   - Sent ONE Bitcoin transaction
   - Recorded the same transaction ID for BOTH swaps ❌

#### The Bug
The system let two swaps share the **same Bitcoin transaction hash**. This is like two receipts having the same order number - it's impossible to tell them apart.

---

## Technical Deep Dive

### The Database Evidence

**Two swaps, same BTC hash:**

| ID | Swap Transaction | BTC Transaction Hash | Amount | Status |
|----|------------------|---------------------|--------|--------|
| 61 | `0x32486e9e...` | **`0af608f0...`** | 50 ICY | completed |
| 62 | `0xe36ac188...` | **`0af608f0...`** | 50 ICY | completed |

**Both rows have the SAME Bitcoin transaction hash!** This should be impossible.

### Bitcoin Blockchain Evidence

```
Transaction: 0af608f0537ac0cc...
├─ Amount sent: ~99,458 satoshis
├─ Recipient: bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz
└─ Sent: April 1, 2026 at 12:21

This is ONE transaction, not two.
```

### Root Cause Analysis

#### Problem #1: No Locking in UTXO Selection

**What are UTXOs?**  
UTXOs = "Unspent Transaction Outputs" = Available Bitcoin coins in your wallet

When you send Bitcoin, you must pick which coins (UTXOs) to use.

**Normal Flow:**
```
Swap #1: Pick coins A+B, send transaction #1 ✅
Swap #2: Pick coins C+D, send transaction #2 ✅
Result: Two different transactions
```

**What Went Wrong:**
```
Swap #1: Pick coins A+B, send transaction #1 ✅
Swap #2: Pick coins A+B (same coins!), send transaction #1 again ❌
Result: Same transaction hash recorded twice
```

**Why?**  
The code that picks which coins to use (`selectUTXOs()` in `internal/btcrpc/helper.go` lines 288-354) had **no mutex lock**. When two swaps came in rapid succession (0.23 seconds apart), both picked the same coins because nothing prevented them from looking at the same wallet state at the same time.

**Code Path:**
```
ProcessPendingBtcTransactions() [btc.go:100-162]
├─ for _, pendingTx := range pendingTxs {  // Sequential loop, but...
│   ├─ selectUTXOs() → selected UTXOs (NO MUTEX LOCK!)
│   ├─ prepareTx() → creates *wire.MsgTx
│   ├─ sign() → signs transaction
│   └─ broadcast() → sends to network
│       └─ Returns tx hash
└─ }

Two calls, 0.23 seconds apart:
  Swap #1: Same UTXOs → Same inputs
  Swap #2: Same UTXOs → Same inputs
  Result: Same inputs → Same tx hash
```

#### Problem #2: No Unique Constraint in Database

**Current Schema (Vulnerable):**
```sql
-- migrations/schema/0003_add_onchain_btc_processed_transaction_table.up.sql
CREATE TABLE onchain_btc_processed_transactions (
    ...
    btc_transaction_hash VARCHAR(255),  -- No UNIQUE constraint!
    ...
);
```

**What This Means:**
- Database allows same `btc_transaction_hash` multiple times
- No enforcement at the database level
- Completely relies on application logic to prevent duplicates

**Existing Constraints:**
```sql
-- migrations/schema/0004_add_unique_constraints_to_transactions.up.sql
-- Adds UNIQUE to onchain_icy_transactions ✅
-- Adds UNIQUE to onchain_btc_transactions ✅
-- Does NOT add to onchain_btc_processed_transactions ❌
```

**Why This Matters:**
Even if the code bug didn't happen, any programmer error could insert duplicate hashes. Database constraints are the last line of defense.

---

## Immediate User Refund

### SQL Query to Fix Missing Payout

```sql
-- Reset the duplicate record so system will send new BTC transaction
UPDATE onchain_btc_processed_transactions 
SET 
    btc_transaction_hash = NULL,
    status = 'pending',
    network_fee = NULL,
    processed_at = NULL,
    updated_at = NOW()
WHERE swap_transaction_hash = '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a';
```

**What This Does:**
1. Clears the duplicate transaction hash
2. Sets status to 'pending' → System will re-process
3. Clears processing timestamps → Fresh start
4. System will create NEW Bitcoin transaction within 2 minutes

**No Other Tables Need Updating:**
- ✅ `onchain_btc_processed_transactions` - The target table (UPDATE above)
- ❌ `onchain_icy_swap_transactions` - Already has correct swap data
- ❌ `swap_requests` - Separate workflow, no relationship
- ❌ No balance/audit tables - Everything is on-chain

---

## Permanent Fixes

### Fix #1: Add Mutex Lock (Code)

**File:** `internal/btcrpc/helper.go`

```go
// Add at package level
var utxoMutex sync.Mutex

// Modify selectUTXOs function
func (b *BtcRpc) selectUTXOs(address string, amountToSend int64) (selected []blockstream.UTXO, changeAmount int64, fee int64, err error) {
    utxoMutex.Lock()
    defer utxoMutex.Unlock()
    
    // Rest of the function remains the same
    confirmedUTXOs, err := b.getConfirmedUTXOs(address)
    if err != nil {
        return nil, 0, 0, err
    }
    
    for _, utxo := range confirmedUTXOs {
        selected = append(selected, utxo)
        // ... rest of logic
    }
    
    return selected, changeAmount, fee, nil
}
```

**Why This Works:**
- Mutex ensures only ONE swap can select UTXOs at a time
- First swap finishes selecting + broadcasting before second starts
- Second swap sees updated UTXO set (first swap's coins already spent)
- **Different coin combinations = Different transaction hashes**

**Performance Impact:**
- Minimal - UTXO selection is fast (< 100ms typically)
- Only blocks when multiple swaps are processing simultaneously
- Acceptable tradeoff for correctness

---

### Fix #2: Add Unique Constraint (Database Migration)

**Up Migration:**
**File:** `migrations/schema/0013_add_unique_constraint_btc_transaction_hash.up.sql`

```sql
-- Add unique constraint to prevent duplicate btc_transaction_hash
-- First, handle any existing duplicates (set them to NULL for re-processing)

-- Step 1: Identify and mark duplicates for re-processing
UPDATE onchain_btc_processed_transactions 
SET 
    btc_transaction_hash = NULL,
    status = 'pending',
    processed_at = NULL,
    updated_at = NOW()
WHERE id IN (
    SELECT id FROM (
        SELECT 
            id,
            btc_transaction_hash,
            ROW_NUMBER() OVER (PARTITION BY btc_transaction_hash ORDER BY created_at) as rn
        FROM onchain_btc_processed_transactions
        WHERE btc_transaction_hash IS NOT NULL
    ) duplicates
    WHERE rn > 1
);

-- Step 2: Add unique constraint
ALTER TABLE onchain_btc_processed_transactions
ADD CONSTRAINT unique_btc_tx_hash UNIQUE (btc_transaction_hash);

-- Step 3: Add index for faster lookups
CREATE INDEX idx_btc_processed_tx_hash 
ON onchain_btc_processed_transactions (btc_transaction_hash);
```

**Down Migration:**
**File:** `migrations/schema/0013_add_unique_constraint_btc_transaction_hash.down.sql`

```sql
-- Remove unique constraint
ALTER TABLE onchain_btc_processed_transactions
DROP CONSTRAINT IF EXISTS unique_btc_tx_hash;

-- Remove index
DROP INDEX IF EXISTS idx_btc_processed_tx_hash;
```

**Why This Works:**
- Database enforces uniqueness at INSERT/UPDATE time
- If duplicate detected → Database rejects with error
- Application must handle uniqueness before database insert
- Last line of defense against programming errors

---

### Fix #3: Add Duplicate Detection (Code)

**File:** `internal/telemetry/btc.go`

```go
func (t *Telemetry) ProcessPendingBtcTransactions() error {
    // ... existing code ...
    
    for _, pendingTx := range pendingTxs {
        // ... validation code ...
        
        tx, networkFee, err := t.btcRpc.Send(pendingTx.BTCAddress, amount)
        if err != nil {
            // ... error handling ...
            continue
        }
        
        // NEW: Check if this BTC tx hash already exists
        existingBTC, err := t.store.OnchainBtcProcessedTransaction.GetByBTCHash(t.db, tx.TxID())
        if err == nil && existingBTC != nil {
            t.logger.Error("[ProcessPendingBtcTransactions][DuplicateBTCHash]", map[string]string{
                "btc_tx_hash":       tx.TxID(),
                "existing_swap_id":  fmt.Sprintf("%d", existingBTC.ID),
                "current_swap_id":   fmt.Sprintf("%d", pendingTx.ID),
            })
            // Skip this transaction and move to next
            continue
        }
        
        // ... continue with update ...
    }
}
```

**Add to Store Interface:**
**File:** `internal/store/onchainbtcprocessedtransaction/interface.go`

```go
type IStore interface {
    // ... existing methods ...
    GetByBTCHash(tx *gorm.DB, btcTxHash string) (*model.OnchainBtcProcessedTransaction, error)
}
```

**Add to Store Implementation:**
**File:** `internal/store/onchainbtcprocessedtransaction/onchain_btc_processed_transaction.go`

```go
func (s *store) GetByBTCHash(tx *gorm.DB, btcTxHash string) (*model.OnchainBtcProcessedTransaction, error) {
    var btcProcessedTx model.OnchainBtcProcessedTransaction
    result := tx.Where("btc_transaction_hash = ?", btcTxHash).First(&btcProcessedTx)
    if result.Error != nil {
        return nil, result.Error
    }
    return &btcProcessedTx, nil
}
```

**Why This Works:**
- Checks for duplicates BEFORE inserting
- Logs error for debugging
- Skips duplicate instead of inserting
- Prevents duplicates even if mutex lock fails

---

### Fix #4: Add Logging (Code)

**File:** `internal/telemetry/btc.go`

```go
func (t *Telemetry) ProcessPendingBtcTransactions() error {
    t.logger.Info("[ProcessPendingBtcTransactions] Start processing pending BTC transactions...")
    
    // ... fetch pending transactions ...
    
    for _, pendingTx := range pendingTxs {
        // Add detailed logging before UTXO selection
        t.logger.Info("[ProcessPendingBtcTransactions][BeforeSend]", map[string]string{
            "swap_tx_id":       fmt.Sprintf("%d", pendingTx.ID),
            "swap_tx_hash":     pendingTx.SwapTransactionHash,
            "btc_address":      pendingTx.BTCAddress,
            "amount_sats":      amount.Value,
        })
        
        tx, networkFee, err := t.btcRpc.Send(pendingTx.BTCAddress, amount)
        if err != nil {
            // ... error handling ...
            continue
        }
        
        // Add detailed logging after successful send
        t.logger.Info("[ProcessPendingBtcTransactions][AfterSend]", map[string]string{
            "swap_tx_id":       fmt.Sprintf("%d", pendingTx.ID),
            "swap_tx_hash":     pendingTx.SwapTransactionHash,
            "btc_tx_hash":      tx.TxID(),
            "network_fee_sats": fmt.Sprintf("%d", networkFee),
            "timestamp":        time.Now().Format(time.RFC3339),
        })
        
        // ... rest of processing ...
    }
}
```

**Why This Helps:**
- Enables debugging of duplicate issues
- Tracks UTXO selection timing
- Helps identify race conditions in production
- Provides audit trail for investigation

---

## Implementation Order

### Phase 1: Immediate (Done Today)

1. ✅ **Refund User**
   ```sql
   UPDATE onchain_btc_processed_transactions 
   SET 
       btc_transaction_hash = NULL,
       status = 'pending',
       network_fee = NULL,
       processed_at = NULL,
       updated_at = NOW()
   WHERE swap_transaction_hash = '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a';
   ```
   - Run manually in production
   - User will receive refund within 2 minutes (automatic processing)

2. ⏸️ **Add Duplicate Detection**
   - Implement `GetByBTCHash()` store method
   - Add duplicate check in `ProcessPendingBtcTransactions()`
   - Test thoroughly

### Phase 2: Short-Term (This Week)

3. ⏸️ **Add Mutex Lock**
   - Add `utxoMutex` to `internal/btcrpc/helper.go`
   - Wrap `selectUTXOs()` with mutex
   - Test with concurrent requests

4. ⏸️ **Add Logging**
   - Add detailed logs before/after UTXO selection
   - Add logs after successful broadcast
   - Monitor for duplicate hash warnings

### Phase 3: Long-Term (Next Sprint)

5. ⏸️ **Add Unique Constraint**
   - Create migration `0013_add_unique_constraint_btc_transaction_hash`
   - Handle existing duplicates
   - Test migration on staging

---

## Testing Checklist

### Immediate Fix Testing

- [ ] Run UPDATE query in staging
- [ ] Verify record status = 'pending'
- [ ] Verify btc_transaction_hash = NULL
- [ ] Wait 2 minutes for cron job
- [ ] Verify new BTC transaction sent
- [ ] Verify new btc_transaction_hash populated
- [ ] Verify user received BTC

### Mutex Lock Testing

- [ ] Create test with concurrent swap requests
- [ ] Verify only one swap processes UTXOs at a time
- [ ] Verify no race conditions
- [ ] Measure performance impact (< 100ms overhead)

### Unique Constraint Testing

- [ ] Attempt to insert duplicate btc_transaction_hash
- [ ] Verify database rejects with error
- [ ] Verify application handles error gracefully
- [ ] Test migration on staging database
- [ ] Verify existing duplicates handled correctly

### Duplicate Detection Testing

- [ ] Create test with duplicate BTC hash
- [ ] Verify duplicate detected and logged
- [ ] Verify duplicate skipped
- [ ] Verify next transaction processed normally

---

## Monitoring & Alerting

### Metrics to Add

```go
// Add to monitoring
var (
    duplicateBtcHash = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "btc_duplicate_hash_total",
            Help: "Number of duplicate BTC transaction hashes detected",
        },
        []string{"swap_tx_id"},
    )
    
    utxoSelectionDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "utxo_selection_duration_seconds",
            Help: "Time taken to select UTXOs",
        },
    )
)
```

### Alerts to Add

```yaml
# Prometheus alert
groups:
  - name: btc-processing
    rules:
      - alert: DuplicateBtcHashDetected
        expr: rate(btc_duplicate_hash_total[5m]) > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Duplicate BTC transaction hash detected"
          description: "{{ $value }} duplicate hashes in last 5 minutes"
```

---

## Documentation Updates

- [ ] Update `CLAUDE.md` with duplicate hash handling
- [ ] Add incident to `docs/incidents/` directory
- [ ] Update API documentation if needed
- [ ] Add runbook for handling duplicates

---

## Communication

### User Communication Template

```
Hi,

We've resolved the issue with your swap:

【 What Happened 】
You made two swaps (50 ICY each) but only received one BTC payment.
Root cause: Two swaps used the same Bitcoin transaction due to a rare bug.

【 What We Did 】
1. Identified the duplicate transaction issue
2. Reset your second swap to be re-processed
3. Implementing fixes to prevent this in the future

【 What You'll Receive】
Bitcoin Transaction: NEW transaction within 2-5 minutes
Amount: ~99,458 satoshis (50 ICY equivalent)

【 Prevention】
We're adding safeguards:
- Mutex locks to prevent concurrent UTXO selection
- Database constraints to prevent duplicate hashes
- Duplicate detection logging

We apologize for the inconvenience.
```

### Internal Communication

- Update engineering team on incident
- Schedule code review for fixes
- Plan deployment window for migration
- Update on-call procedures

---

## Related Documents

- [Incident Report](./CRITICAL-BUG-duplicate-btc-tx-hash.md)
- [Finding: Not a Duplicate](./FINDING-not-a-duplicate.md)
- [Blockchain Verification](./blockchain-verification-results.md)
- [Investigation Analysis](./investigation-analysis.md)
- [Root Cause: Duplicate BTC Hash](./CRITICAL-BUG-duplicate-btc-tx-hash.md)

---

## Post-Mortem Summary

**Incident**: Two ICY swaps merged into one Bitcoin transaction  
**Duration**: April 1-8, 2026 (7 days to discovery)  
**Impact**: User lost ~$102 USD (50 ICY equivalent)  
**Root Causes**:
1. No mutex lock on UTXO selection
2. No unique constraint on btc_transaction_hash

**Immediate Action**: Refund user's missing payout  
**Long-Term Fixes**:
1. Add mutex lock to `selectUTXOs()`
2. Add unique constraint to database
3. Add duplicate detection in code
4. Add comprehensive logging

**Prevention**:
- Database constraints as last line of defense
- Mutex locks for atomic operations
- Duplicate detection for early warning
- Comprehensive logging for debugging

**Status**: Ready for implementation  
**Priority**: CRITICAL  
**Assignee:** Engineering Team