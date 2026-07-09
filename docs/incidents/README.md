# Incident Documentation Index

**Incident**: Duplicate ICY Swap Transaction Investigation  
**Date**: 2026-04-08  
**User**: 0x5155b007D5C1AFe88912e52FABda1c7ED86baae0  
**Status**: Investigation Complete - Awaiting Database Verification  

---

## Documentation Files

### 1. 📄 [FINDING-not-a-duplicate.md](./FINDING-not-a-duplicate.md)

**Priority**: ⭐⭐⭐ HIGH - Read This First

**Summary**: Key finding that this is NOT a duplicate transaction bug. User submitted two intentional separate swaps with a 22-second gap.

**Key Points**:
- ✅ Two separate blockchain transactions (different nonces: 61 vs 62)
- ⏱️ 22-second gap between submissions (NOT rapid double-click)
- ✅ Both confirmed successfully on Base Mainnet
- ⏸️ BTC payout status pending database verification

---

### 2. 📊 [blockchain-verification-results.md](./blockchain-verification-results.md)

**Priority**: ⭐⭐ HIGH

**Summary**: Complete blockchain explorer verification of both transactions on Base Mainnet.

**Key Points**:
- Detailed transaction data from Basescan
- Confirmed timestamps and block numbers
- Function call parameters decoded
- Token transfer details verified

---

### 3. 🔍 [investigation-analysis.md](./investigation-analysis.md)

**Priority**: ⭐⭐ HIGH

**Summary**: Deep dive investigation into code flow, database schema, cron jobs, and root cause analysis.

**Key Points**:
- Code flow analysis of swap processing
- Database schema analysis
- Cron job execution timing
- Identified system gaps (no rate limiting, no balance monitoring)

---

### 4. 📋 [2026-04-01-duplicate-icy-swap-transactions.md](./2026-04-01-duplicate-icy-swap-transactions.md)

**Priority**: ⭐ MEDIUM

**Summary**: Comprehensive incident report with timeline, root cause, and recommendations.

**Key Points**:
- Complete incident timeline
- Customer impact assessment
- Recommended actions (P0, P1, P2)
- Resolution tracking checklist

---

### 5. 📝 [btc-verification-guide.md](./btc-verification-guide.md)

**Priority**: ⭐ MEDIUM

**Summary**: Guide for verifying BTC payout status using multiple methods.

**Key Points**:
- Database query verification
- Blockchain explorer verification
- Application logs verification
- User communication templates
- 3 possible scenarios and actions

---

### 6. 💾 [btc-payout-verification.sql](./btc-payout-verification.sql)

**Priority**: ⭐ MEDIUM

**Summary**: SQL queries for verifying BTC payout status when database is available.

**Key Points**:
- 8 investigation queries
- Check swap transactions
- Check BTC processed transactions
- Check pending transactions
- Check by BTC address

---

## Investigation Timeline

| Time | Action | Status |
|------|--------|--------|
| Initial | User report: "Duplicate transaction issue" | 📥 Received |
| Step 1 | Database query for swap transactions | ✅ Completed |
| Step 2 | Identified duplicate timestamps in DB | ✅ Completed |
| Step 3 | Code flow analysis | ✅ Completed |
| Step 4 | Blockchain explorer verification | ✅ Completed |
| Step 5 | **Critical Finding**: 22-second gap, different nonces | ✅ **Completed** |
| Step 6 | Database query for BTC payouts | ⏸️ **Pending (DB timeout)** |
| Step 7 | Check user's BTC wallet | ⏸️ **Pending user verification** |
| Step 8 | Determine payout status | ⏸️ **Pending** |
| Step 9 | Process refund if needed | ⏸️ **Pending** |

---

## Key Findings Summary

### ✅ Verified

1. **Not a Duplicate**: User submitted two intentional swaps
2. **Blockchain Confirmed**: Both transactions successful on Base Mainnet
3. **Timing**: 22-second gap between swaps (not double-click)
4. **Nonces**: Sequential nonces (61 then 62) = intentional submission
5. **ICY Transfer**: 100 ICY transferred successfully (50 + 50)

### ⏸️ Pending Verification

1. **BTC Payout Status**: Were both swaps processed for BTC?
2. **User's BTC Wallet**: How many transactions received?
3. **Database Query**: Check `onchain_btc_processed_transactions` table

### ❌ Identified Gaps

1. No rate limiting (user can submit rapid swaps)
2. No balance monitoring (no alert when contract low)
3. No transaction status tracking for users
4. No frontend double-click prevention

---

## Recommended Actions By Priority

### P0 (Immediate) - BLOCKING

1. ✅ ~~Blockchain verification~~ - DONE
2. ⏸️ Run database query for BTC payouts
3. ⏸️ Request user to verify BTC wallet transactions
4. ⏸️ Update user with findings

### P1 (Short-Term) - This Week

1. Add rate limiting (1-minute cooldown between swaps)
2. Add minimum balance check before processing
3. Create transaction status endpoint
4. Add database indexes for faster queries

### P2 (Long-Term) - Next Sprint

1. Implement swap queue with deduplication
2. Add frontend double-click prevention
3. Add idempotency keys for swap requests
4. Create user transaction history view

---

## Next Steps

### If Database Available:

```bash
psql -h <HOST> -U <USER> -d icy_backend_local -f docs/incidents/btc-payout-verification.sql
```

### If Database Still Unavailable:

1. Check application logs for BTC send operations
2. Request user to verify BTC wallet transaction history
3. Contact BTC treasury service provider

### After Verification:

- If both BTC sent: Close issue, no action needed
- If one BTC missing: Process manual payout
- If both pending: Debug processing issue

---

## Communication Templates

### For User (After DB Verification):

**If both BTC sent**:
```
We've verified both swaps. Both were processed successfully. 
You should have received 2 BTC transactions to your wallet. 
Please check and confirm.
```

**If one BTC missing**:
```
We found one missing BTC payout. Processing manual refund now. 
You'll receive ~102,450 satoshis within 2-5 minutes.
```

**If both pending**:
```
We're investigating why BTC payouts haven't processed. 
We'll update you within 24 hours.
```

---

## Related Files

- User wallet: `0x5155b007D5C1AFe88912e52FABda1c7ED86baae0`
- BTC address: `bc1q6wsfpvu70d5lh783mgl3z0z0dtjmz2g2yc0qmz`
- Transaction #1: `0x32486e9e44e9857b9f5a5ec52754a38106cfe89b1d6ee5fc4560f39832f33c35`
- Transaction #2: `0xe36ac188d3a539f431568b057db554ece6a6143afeeadfa9a3249c1904bd060a`

---

## Contact & Escalation

**Primary Investigator**: Engineering Team  
**Escalation**: If database remains unavailable for 24+ hours  
**User Communication**: Provide transaction hashes and request wallet verification