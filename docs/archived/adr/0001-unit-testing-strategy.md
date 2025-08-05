# ADR: Unit Testing Strategy for ICY Backend

## Status
Proposed

## Context
The ICY Backend is a cryptocurrency swap service handling Bitcoin and ICY token transactions. Following a comprehensive codebase analysis, we identified critical gaps in unit test coverage across core business logic packages. The system handles financial operations, multi-endpoint API integrations, and complex failover mechanisms that require robust testing to ensure reliability and maintainability.

## Current Test Coverage Analysis
- **Utils packages** (config, logger) - Well tested ✅
- **Model package** - Basic web3_bigint_test.go exists ⚠️
- **Oracle package** - Only util_test.go, missing core logic ❌
- **BTC RPC package** - Blockstream tests exist, missing main logic ❌
- **Store packages** - No unit tests ❌
- **Handler packages** - No unit tests ❌
- **Telemetry package** - No comprehensive tests ❌

## Decision
We will implement a prioritized unit testing strategy focusing on business-critical packages with complex logic and high financial impact.

## Implementation Plan

### Critical Priority (Immediate - Sprint 1)
1. **internal/btcrpc/btcrpc.go**
   - Multi-endpoint failover logic and circuit breaker behavior
   - UTXO selection algorithms and fee calculations
   - Transaction signing and broadcasting
   - Network failure error handling

2. **internal/oracle/oracle.go**
   - ICY/BTC rate calculations and treasury management
   - Price oracle integration with external APIs
   - Error scenarios with price feed failures

3. **internal/handler/swap/swap.go**
   - Signature generation with nonce/deadline validation
   - Swap request validation and duplicate detection
   - Concurrent goroutine handling in Info endpoint
   - Database transaction rollback scenarios

### High Priority (Sprint 2)
4. **internal/model/web3_bigint.go**
   - Mathematical operations (Add/Sub) with different decimals
   - ToFloat conversion accuracy and precision
   - Edge cases with very large numbers

5. **internal/btcrpc/helper.go**
   - Fee calculation algorithms and dust limit detection
   - UTXO selection strategies for different scenarios

### Medium Priority (Sprint 3)
6. **internal/store/store.go** and related packages
   - Database CRUD operations with error scenarios
   - Transaction isolation and constraint violations

7. **internal/telemetry/telemetry.go**
   - Concurrent transaction processing and mutex handling
   - Multi-RPC service integration

## Testing Standards

### Test Framework
- **Ginkgo/Gomega** for BDD-style testing (already in use)
- **Testify** for assertions and mocking where needed
- **Database testing** with test containers or in-memory databases

### Test Categories
1. **Unit Tests** - Individual functions with mocked dependencies
2. **Integration Tests** - Database operations and external API calls
3. **Error Scenario Tests** - Network failures and invalid inputs
4. **Financial Precision Tests** - All monetary calculations with edge cases
5. **Concurrent Tests** - Thread-safe operations and race conditions

### Coverage Goals
- **Critical packages**: 90%+ coverage
- **High priority packages**: 80%+ coverage
- **Medium priority packages**: 70%+ coverage

### Test Structure
```go
var _ = Describe("PackageName", func() {
    Context("when testing core functionality", func() {
        It("should handle normal cases", func() {
            // Test implementation
        })
        
        It("should handle error cases", func() {
            // Error scenario testing
        })
        
        It("should handle edge cases", func() {
            // Edge case testing
        })
    })
})
```

## Consequences

### Positive
- **Increased reliability** for financial operations and multi-endpoint failover
- **Faster development cycles** with confidence in refactoring
- **Better documentation** through test cases showing expected behavior
- **Reduced production issues** through comprehensive error scenario testing
- **Easier onboarding** for new developers with clear behavioral examples

### Negative
- **Initial development time** investment for writing comprehensive tests
- **Maintenance overhead** for keeping tests updated with code changes
- **Potential over-testing** if not balanced with development velocity

## Implementation Timeline
- **Week 1-2**: Critical priority packages (btcrpc, oracle, swap handler)
- **Week 3-4**: High priority packages (web3_bigint, btcrpc helper)
- **Week 5-6**: Medium priority packages (store, telemetry)

## Success Metrics
- All critical packages achieve 90%+ test coverage
- Zero production incidents related to untested code paths
- Developer confidence score in making changes increases
- Time to identify and fix bugs decreases by 50%

## Review and Updates
This ADR will be reviewed monthly and updated based on:
- New package additions requiring testing
- Changes in business priorities
- Lessons learned from testing implementation
- Performance impact of test suite execution time