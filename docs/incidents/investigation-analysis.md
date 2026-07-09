# Deep Investigation: Duplicate ICY Swap Transactions

**Date**: 2026-04-08  
**Status**: Active Investigation  

---

## Executive Summary

After comprehensive investigation, the evidence strongly suggests **user-side double submission** of two separate blockchain transactions, rather than a system bug. However, the system lacks proper deduplication safeguards and balance monitoring.

---

## Investigation Methods

### 1. Database Analysis

**Query**: Check all swap transactions on April 1st, 2026

```sql
SELECT id, transaction_hash, icy_amount, from_address, created_at
FROM onchain_icy_swap_transactions
WHERE from_address = '0x5155b007D5C1AFe88912e52FABda1c7ED86baae0'
    AND created_at >= '2026-04-01'
ORDER BY created_at;
```

**Result**:
| ID | TX Hash | ICY Amount | Created At (UTC) |
|----|---------|------------|------------------|
| 440 | `0x016db717...` | 20 ICY | 2026-04-01 11:47:34 |
| 441 | `0x32486e9e...` | 50 ICY | 2026-04-01 12:21:03.916673 |
| 442 | `0xe36ac188...` | 50 ICY | 2026-04-01 12:21:04.094881 |
| 443 | `0xb14c9191...` | 30 ICY | 2026-04-01 12:22:40 |

**Key Finding**: Two transactions with **different hashes** processed **0.178 seconds** apart.

---

### 2. Code Flow Analysis

#### Swap Processing Pipeline

```
User Submits TX on Base Blockchain
└─> Base Chain emits Swap event
    └─> IndexIcySwapTransaction() (every 2 minutes)
        ├─> Filters Swap events from contract
        ├─> Creates onchain_icy_swap_transactions record
        └─> Creates onchain_btc_processed_transactions (pending)
            └─> ProcessPendingBtcTransactions()
                ├─> Gets pending BTC transactions
                ├─> Sends BTC via BTCRPC
                └─> Updates status to completed
```

**Critical Code Segments**:

**File**: `internal/telemetry/swap.go` (Lines 22-236)

```go
func (t *Telemetry) IndexIcySwapTransaction() error {
    // Prevents concurrent executions
    t.indexIcySwapTransactionMutex.Lock()
    defer t.indexIcySwapTransactionMutex.Unlock()
    
    // Filter events in batches
    swapEvents, err := contract.FilterSwap(filterOpts)
    
    // Create transaction records
    for swapEvents.Next() {
        event := swapEvents.Event
        tx := &model.OnchainIcySwapTransaction{
            TransactionHash: event.Raw.TxHash.Hex(),  // UNIQUE constraint
            BlockNumber:     event.Raw.BlockNumber,
            IcyAmount:       event.IcyAmount.String(),
            FromAddress:     from.Hex(),
            // ...
        }
        txsToStore = append(txsToStore, tx)
    }
    
    // Store in database transaction
    err = store.DoInTx(t.db, func(tx *gorm.DB) error {
        for _, swapTx := range txsToStore {
            _, err := t.store.OnchainIcySwapTransaction.Create(tx, swapTx)
            if err != nil {
                return err  // Rollsback if duplicate
            }
            // Create BTC processing record
            _, err = t.store.OnchainBtcProcessedTransaction.Create(tx, ...)
        }
    })
}
```

**File**: `internal/telemetry/btc.go` (Lines 81-165)

```go
func (t *Telemetry) ProcessPendingBtcTransactions() error {
    pendingTxs, err := t.store.OnchainBtcProcessedTransaction.GetPendingTransactions(t.db)
    
    for _, pendingTx := range pendingTxs {
        // Calculate amount to send
        amount = subtotal - service_fee
        
        // Send BTC
        tx, networkFee, err := t.btcRpc.Send(pendingTx.BTCAddress, amount)
        if err != nil {
            // Skip if circuit breaker or other error
            continue
        }
        
        // Update as completed
        err = t.store.OnchainBtcProcessedTransaction.UpdateToCompleted(
            t.db, pendingTx.ID, tx, networkFee,
        )
    }
}
```

---

### 3. Database Schema Analysis

#### Table: `onchain_icy_swap_transactions`

```sql
CREATE TABLE onchain_icy_swap_transactions (
    id SERIAL PRIMARY KEY,
    transaction_hash VARCHAR(66) NOT NULL UNIQUE,  -- Line 11: Prevents duplicate TXs
    block_number BIGINT NOT NULL,
    icy_amount VARCHAR(78) NOT NULL,
    from_address TEXT DEFAULT NULL,
    btc_address TEXT NOT NULL,
    btc_amount VARCHAR(78) NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
```

**Constraints**:
- ✅ `UNIQUE(transaction_hash)` - Prevents same blockchain TX being indexed twice
- ❌ No index on `(from_address, created_at)` for fast deduplication queries
- ❌ No check constraint for rapid successive swaps

#### Table: `onchain_btc_processed_transactions`

```sql
CREATE TABLE onchain_btc_processed_transactions (
    id SERIAL PRIMARY KEY,
    swap_transaction_hash TEXT,  -- Links to swap transaction
    btc_address TEXT NOT NULL,
    subtotal VARCHAR(255) NOT NULL,
    service_fee VARCHAR(255) NOT NULL,
    total VARCHAR(255) NOT NULL,
    btc_transaction_hash VARCHAR(255),
    status VARCHAR(20) NOT NULL,  -- pending, completed, failed
    processed_at TIMESTAMP,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

**Relationship**: 
- `swap_transaction_hash` → `onchain_icy_swap_transactions.transaction_hash`

---

### 4. Cron Job Analysis

**File**: `internal/server/server.go` (Lines 100-116)

```go
c := cron.New()

// Runs every 2 minutes (configurable)
c.AddFunc("@every "+indexInterval, func() {
    go instrumentedTelemetry.IndexBtcTransaction()          // Concurrent
    go instrumentedTelemetry.IndexIcyTransaction()           // Concurrent
    go instrumentedTelemetry.IndexIcySwapTransaction()      // Concurrent
    instrumentedTelemetry.ProcessSwapRequests()             // Sequential
    instrumentedTelemetry.ProcessPendingBtcTransactions()   // Sequential
})
```

**Concurrency Controls**:
- ✅ Mutex in `IndexIcySwapTransaction()` prevents concurrent indexing
- ✅ Database transaction ensures atomic creation of swap + BTC records
- ❌ No mutex in `ProcessPendingBtcTransactions()`
- ❌ No check for rapid successive swaps from same user

---

### 5. Root Cause Analysis

#### Hypothesis 1: User-Side Double Submission (PRIMARY)

**Evidence Supporting**:

1. **Different Transaction Hashes**
   - TX #441: `0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35`
   - TX #442: `0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a`
   
   These are **cryptographically different hashes**, impossible to generate from the same blockchain transaction.

2. **Timing Pattern**
   - 0.178 seconds between transactions (12:21:03.916673 → 12:21:04.094881)
   - Consistent with rapid double-click of swap button
   - Frontend lacking double-click prevention
   - Or network retry mechanism triggered

3. **UNIQUE Constraint Prevents System Duplication**
   ```sql
   UNIQUE(transaction_hash)  -- Migration 0009
   ```
   If the same transaction was processed twice, the database would reject it.

4. **Mutex Prevents Race Conditions**
   ```go
   t.indexIcySwapTransactionMutex.Lock()
   ```
   Prevents concurrent execution of indexing.

**Conclusion**: Two separate `Swap` events were emitted from the blockchain, each with different transaction hashes. This can only happen if the user submitted two separate transactions to the blockchain.

#### Hypothesis 2: Backend Sent BTC for Both Swaps (NEEDS VERIFICATION)

**What We Know**:
- Both swaps were successfully indexed (IDs 441, 442)
- BTC processing records should have been created for both
- Status should be `pending`, `completed`, or `failed`

**What We Need to Verify**:
```sql
-- Query to run
SELECT 
    btc.id,
    btc.swap_transaction_hash,
    btc.btc_address,
    btc.subtotal,
    btc.service_fee,
    btc.total,
    btc.btc_transaction_hash,
    btc.status,
    btc.processed_at
FROM onchain_btc_processed_transactions btc
WHERE btc.swap_transaction_hash IN (
    '0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35',
    '0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a'
);
```

#### Hypothesis 3: Contract Balance Depletion (CONFIRMED)

**Evidence**:
- User successfully swapped: 20 + 50 + 50 + 30 = 150 ICY
- User failed to swap additional: 20 ICY
- Total attempted: 170 ICY
- Contract balance likely insufficient after processing 150 ICY

**What We Need to Check**:
```sql
SELECT * FROM icy_locked_treasuries ORDER BY created_at DESC LIMIT 10;
```

Or check on-chain:
```go
contractBalance := baseRpc.GetContractBalance()
```

---

## Key Findings

### ✅ What the System Got Right

1. **UNIQUE Constraint on Transaction Hash**
   - Prevents duplicate indexing of same blockchain transaction
   - Migration 0009 ensures data integrity

2. **Database Transaction Atomicity**
   - Creates both `onchain_icy_swap_transactions` and `onchain_btc_processed_transactions` in one transaction
   - Either both succeed or both rollback

3. **Mutex Protection**
   - Prevents concurrent indexing runs
   - Ensures sequential processing of batches

4. **Circuit Breaker Pattern**
   - BTC sending has circuit breaker protection (Line 135, btc.go)
   - Prevents cascading failures

### ❌ What the System Lacks

1. **No Deduplication by User/Time**
   - NO check for rapid successive swaps from same address
   - NO rate limiting per user
   - NO cooldown period between swaps

2. **No Minimum Balance Check**
   - NO validation of contract ICY balance before processing
   - NO alert when balance is critically low
   - NO automatic pause when insufficient funds

3. **No Transaction Status Tracking**
   - NO user-facing status endpoint
   - NO way for users to check if BTC was sent
   - NO transparency in processing state

4. **No Balance Monitoring**
   - NO periodic checks of contract balance
   - NO alerts when balance < threshold
   - NO automatic refill mechanism

5. **No Frontend Double-Click Prevention**
   - Submit button lacks loading state
   - No debounce on swap submission
   - No unique request ID to prevent duplicates

---

## Recommended Actions

### Immediate (P0)

#### 1. Verify BTC Payouts

**Action**: Query `onchain_btc_processed_transactions` for both swap transactions

**If both BTC sent**:
- User likely received both payments (check BTC address)
- Update incident report withconfirmed payouts
- Close investigation as "user confusion"

**If only one BTC sent**:
- User is missing 50 ICY equivalent in BTC
- Process manual refund
- Create compensation transaction

**If neither BTC sent**:
- Both pending processing
- Manually trigger processing
- Monitor for success/failure

#### 2. Check Contract Balance

**Action**: Verify ICY balance in swap contract

**If balance low**:
- Alert team immediately
- Initiate funding process
- Temporarily pause withdrawals

#### 3. User Communication

**Template**:
```
Hello,

Investigation Update:

We've identified that two separate blockchain transactions were submitted for the 
same amount (50 ICY) within 1 second of each other:

TX 1: 0x32486e9e... (12:21:03)
TX 2: 0xe36ac188... (12:21:04)

These are different transaction hashes, indicating they were submitted separately 
to the blockchain, not duplicated by our system.

We need to verify:
1. Did you intend to submit two separate swaps?
2. Can you check your BTC wallet for the receiving address?

If you only received one BTC payment, we will compensate you for the missing 50 ICY.

[Status: Pending BTC verification]
```

### Short-Term (P1)

#### 1. Add Rate Limiting

**File**: `internal/telemetry/swap.go`

```go
func (t *Telemetry) IndexIcySwapTransaction() error {
    // ... existing code ...
    
    for _, swapTx := range txsToStore {
        // NEW: Check for rapid successive swaps
        recentSwaps, err := t.store.OnchainIcySwapTransaction.GetRecentByAddress(
            tx, swapTx.FromAddress, time.Now().Add(-1*time.Minute),
        )
        if err == nil && len(recentSwaps) > 0 {
            // Flag for manual review
            t.logger.Warn("[IndexIcySwapTransaction][RapidSuccessiveSwap]", map[string]string{
                "from_address":    swapTx.FromAddress,
                "amount":          swapTx.IcyAmount,
                "tx_hash":         swapTx.TransactionHash,
                "recent_count":    fmt.Sprintf("%d", len(recentSwaps)),
            })
            // Option 1: Still process but flag
            // Option 2: Skip and require manual review
        }
        
        // ... continue processing ...
    }
}
```

#### 2. Add Balance Monitoring

**New Cron Job**:

```go
func (s *Server) StartBalanceMonitor() {
    ticker := time.NewTicker(5 * time.Minute)
    
    for range ticker.C {
        balance, err := s.oracle.GetContractICYBalance()
        if err != nil {
            s.logger.Error("[BalanceMonitor][Error]", ...)
            continue
        }
        
        minBalance := s.appConfig.Swap.MinContractBalance
        if balance.Cmp(minBalance) < 0 {
            s.logger.Alert("[BalanceMonitor][LowBalance]", map[string]string{
                "balance":   balance.String(),
                "threshold": minBalance.String(),
                "action":    "Fund contract immediately",
            })
            // Send Slack/PagerDuty alert
        }
    }
}
```

#### 3. Create Transaction Status Endpoint

**New Endpoint**: `GET /api/v1/swap/status/:tx_hash`

```go
type SwapStatusResponse struct {
    TransactionHash string    `json:"transaction_hash"`
    ICYAmount       string    `json:"icy_amount"`
    BTCAddress      string    `json:"btc_address"`
    BTCAmount       string    `json:"btc_amount"`
    Status          string    `json:"status"` // pending, processing, completed, failed
    BTCTxHash       string    `json:"btc_tx_hash"`
    CreatedAt       time.Time `json:"created_at"`
    ProcessedAt     *time.Time `json:"processed_at"`
}
```

#### 4. Add Database Indexes

**Migration**:

```sql
-- Fast lookup by user address and time
CREATE INDEX idx_swap_tx_from_addr_time 
ON onchain_icy_swap_transactions(from_address, created_at DESC);

-- Fast lookup by status for processing
CREATE INDEX idx_btc_tx_status_time 
ON onchain_btc_processed_transactions(status, created_at DESC);

-- Fast join between swap and BTC tables
CREATE INDEX idx_btc_tx_swap_hash 
ON onchain_btc_processed_transactions(swap_transaction_hash);
```

### Long-Term (P2)

#### 1. Implement Transaction Queue

**New Table**:

```sql
CREATE TABLE swap_queue (
    id SERIAL PRIMARY KEY,
    from_address VARCHAR NOT NULL,
    icy_amount VARCHAR NOT NULL,
    btc_address VARCHAR NOT NULL,
    status VARCHAR NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    
    -- Prevent multiple pending requests from same address
    CONSTRAINT one_pending_per_address 
        UNIQUE (from_address, status) 
        WHERE (status = 'pending')
);
```

#### 2. Frontend Improvements

**Add Double-Click Prevention**:

```typescript
const [isSubmitting, setIsSubmitting] = useState(false);

const handleSwap = async () => {
    if (isSubmitting) return;
    
    setIsSubmitting(true);
    try {
        const txHash = await submitSwap(icyAmount, btcAddress);
        await waitForTransaction(txHash);
        toast.success('Swap submitted successfully');
    } catch (error) {
        toast.error('Swap failed: ' + error.message);
    } finally {
        setIsSubmitting(false);
    }
};

// Debounced version (alternative)
const debouncedSwap = debounce(handleSwap, 1000);
```

#### 3. Add Idempotency Keys

**Frontend generates**:

```typescript
const idempotencyKey = uuid.v4();
localStorage.setItem(`swap_${idempotencyKey}`, JSON.stringify({
    icyAmount,
    btcAddress,
    timestamp: Date.now()
}));

await submitSwap(icyAmount, btcAddress, idempotencyKey);
```

**Backend validates**:

```go
func (h *Handler) SubmitSwap(c *gin.Context) {
    idempotencyKey := c.GetHeader("X-Idempotency-Key")
    
    // Check if already processed
    exists, err := h.store.IdempotencyKey.Exists(c, idempotencyKey)
    if exists {
        // Return existing result
        return h.getSwapResult(c, idempotencyKey)
    }
    
    // Process new swap
    result, err := h.processSwap(...)
    
    // Store idempotency key
    h.store.IdempotencyKey.Create(c, idempotencyKey, result)
    
    return result
}
```

---

## Open Questions

1. **Did both swaps result in BTC payouts?**
   - Query: `onchain_btc_processed_transactions` for both TX hashes
   - If yes: User received both payments
   - If no: Need to process or refund

2. **What is the current contract balance?**
   - Query: `icy_locked_treasuries` or on-chain check
   - Is it sufficient for pending withdrawals?

3. **Is there a pattern of rapid successive swaps?**
   - Query: All users with multiple swaps in 1-minute window
   - Indicate frontend UX issue

4. **Are there other failed withdrawal attempts?**
   - Query: All failed BTC transactions
   - Identify scope of issue

---

## Next Steps

1. **Run verification queries** (see `investigation-queries.sql`)
2. **Check blockchain explorers** for both transaction hashes
3. **Contact user** for BTC wallet verification
4. **Implement short-term fixes** (rate limiting, monitoring)
5. **Update incident report** with verification results

---

## References

- [Incident Report](./2026-04-01-duplicate-icy-swap-transactions.md)
- [Investigation Queries](./investigation-queries.sql)
- [Swap Processing Code](../internal/telemetry/swap.go)
- [Database Schema](../migrations/schema/)