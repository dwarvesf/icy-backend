# Circuit Breaker Guide - ICY Backend

This guide explains how the circuit breaker pattern is implemented in the ICY Backend to protect against external API failures and provides operational guidance for monitoring and managing circuit breaker states.

## Overview

The ICY Backend uses the [sony/gobreaker](https://github.com/sony/gobreaker) library to implement circuit breakers for external API calls. Circuit breakers protect the system from cascading failures when external services become unavailable or unresponsive.

## Protected Services

Circuit breakers are implemented for:
- **Blockstream API** (`btc_rpc`) - Bitcoin blockchain operations
- **Base RPC** (`base_rpc`) - Ethereum/Base blockchain operations

## Circuit Breaker States

### 1. Closed State (Normal Operation)
- All requests pass through to the external service
- Failures are counted and monitored
- System operates normally

### 2. Open State (Protecting System)
- All requests are immediately rejected with "circuit breaker is open" error
- No calls are made to the external service
- System is protected from further failures

### 3. Half-Open State (Testing Recovery)
- Limited number of test requests are allowed through
- If test requests succeed → circuit breaker closes
- If test requests fail → circuit breaker opens again

## State Transitions

### When Circuit Breaker Opens

The circuit breaker **opens** when consecutive failures reach the configured threshold:

**Blockstream API (btc_rpc)**:
```go
ConsecutiveFailureThreshold: 3  // Opens after 3 consecutive failures
```

**Base RPC (base_rpc)**:
```go
ConsecutiveFailureThreshold: 5  // Opens after 5 consecutive failures
```

### Automatic Recovery Process

**Open → Half-Open Transition**:
- **Blockstream API**: After **60 seconds** timeout
- **Base RPC**: After **120 seconds** timeout

**Half-Open Testing**:
- **Blockstream API**: Allows up to **5 test requests**
- **Base RPC**: Allows up to **3 test requests**

### Complete State Flow
```
Closed → [3/5 consecutive failures] → Open
  ↑                                    ↓
  ← [test requests succeed] ← Half-Open ← [60s/120s timeout]
                                ↓
                              Open ← [test requests fail]
```

## Configuration Details

### Blockstream API Configuration
```go
"blockstream_api": {
    MaxRequests:               5,              // Max requests in half-open
    Interval:                  30 * time.Second, // Stats reset interval
    Timeout:                   60 * time.Second, // Time before half-open
    ConsecutiveFailureThreshold: 3,            // Failures to open
}
```

### Base RPC Configuration
```go
"base_rpc": {
    MaxRequests:               3,               // Max requests in half-open
    Interval:                  45 * time.Second, // Stats reset interval  
    Timeout:                   120 * time.Second, // Time before half-open
    ConsecutiveFailureThreshold: 5,             // Failures to open
}
```

## Monitoring Circuit Breaker State

### Log Monitoring

All state changes are logged at **INFO** level:
```bash
# Monitor state changes in real-time
tail -f /var/log/icy-backend.log | grep "Circuit breaker state change"

# Example log output:
{"level":"info","msg":"Circuit breaker state change","service":"btc_rpc","from":"Closed","to":"Open"}
{"level":"info","msg":"Circuit breaker state change","service":"base_rpc","from":"Open","to":"HalfOpen"}
```

### Metrics Monitoring

**Circuit Breaker State Metric**:
```promql
# Current state (1 = open, 0 = closed/half-open)
icy_backend_circuit_breaker_state{api_name="btc_rpc"}
icy_backend_circuit_breaker_state{api_name="base_rpc"}

# State change frequency (trips per minute)
rate(icy_backend_circuit_breaker_state_changes_total{state="open"}[1m])

# Time since last state change
time() - icy_backend_circuit_breaker_last_state_change_timestamp_seconds
```

**API Call Metrics**:
```promql
# Failed API calls leading to circuit breaker trips
rate(icy_backend_external_api_calls_total{status="error"}[5m]) by (api_name)

# Success rate before/after circuit breaker events
rate(icy_backend_external_api_calls_total{status="success"}[5m]) / 
rate(icy_backend_external_api_calls_total[5m]) by (api_name)
```

## Operational Procedures

### When Circuit Breaker Opens

1. **Immediate Actions**:
   ```bash
   # Check current circuit breaker states
   curl http://localhost:8080/metrics | grep icy_backend_circuit_breaker_state
   
   # Review recent API call errors
   curl http://localhost:8080/metrics | grep external_api_calls_total
   ```

2. **Root Cause Investigation**:
   - Check external service status (Blockstream API, Base RPC endpoints)
   - Review network connectivity and DNS resolution
   - Examine recent error patterns in logs
   - Verify API rate limits and quotas

3. **System Impact Assessment**:
   - BTC operations will fail when `btc_rpc` circuit breaker is open
   - ICY token operations will fail when `base_rpc` circuit breaker is open
   - Background transaction processing will be affected

### Manual Recovery (Emergency)

If automatic recovery is not working or taking too long:

1. **Identify the Issue**:
   ```bash
   # Check which services are failing
   grep "circuit breaker is open" /var/log/icy-backend.log | tail -20
   ```

2. **Test External Services Manually**:
   ```bash
   # Test Blockstream API
   curl -s "https://blockstream.info/api/address/[test-address]/txs" | head -100
   
   # Test Base RPC (if accessible)
   curl -X POST [base-rpc-endpoint] -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
   ```

3. **Force Circuit Breaker Recovery** (if services are healthy):
   - Restart the application to reset circuit breaker states
   - Or implement manual circuit breaker reset endpoint (future enhancement)

### Preventing Circuit Breaker Trips

1. **Proactive Monitoring**:
   ```promql
   # Alert when error rate increases (before circuit breaker opens)
   rate(icy_backend_external_api_calls_total{status="error"}[5m]) / 
   rate(icy_backend_external_api_calls_total[5m]) > 0.3
   ```

2. **Service Health Checks**:
   - Monitor `/health/external` endpoint regularly
   - Set up external service availability monitoring
   - Configure alerts for external service degradation

3. **Graceful Degradation**:
   - Implement fallback mechanisms where possible
   - Cache critical data to reduce external API dependency
   - Provide user-friendly error messages when services are unavailable

## Error Handling

### Circuit Breaker Errors

When circuit breaker is open, operations return:
```
Error: "circuit breaker is open"
```

**Application Behavior**:
- BTC send operations will fail immediately
- Oracle price fetches will fail
- Swap operations may be degraded or fail
- Background indexing will be paused

### Error Classification

The system classifies external API errors for better monitoring:

- **Timeout**: Network timeouts, deadline exceeded
- **Network Error**: Connection issues, DNS failures
- **Server Error**: 5xx HTTP responses, internal server errors
- **Client Error**: 4xx HTTP responses, rate limiting
- **Unknown**: Unclassified errors

## Alerting Configuration

### Critical Alerts

```yaml
# Prometheus alerting rules
groups:
  - name: circuit_breaker_alerts
    rules:
      - alert: CircuitBreakerOpen
        expr: icy_backend_circuit_breaker_state == 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Circuit breaker is open for {{ $labels.api_name }}"
          description: "{{ $labels.api_name }} circuit breaker has been open for more than 1 minute"

      - alert: CircuitBreakerFlapping
        expr: rate(icy_backend_circuit_breaker_state_changes_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Circuit breaker flapping detected"
          description: "Circuit breaker state changes too frequently for {{ $labels.api_name }}"
```

### Warning Alerts

```yaml
      - alert: HighExternalAPIErrorRate
        expr: |
          rate(icy_backend_external_api_calls_total{status="error"}[5m]) / 
          rate(icy_backend_external_api_calls_total[5m]) > 0.3
        for: 3m
        labels:
          severity: warning
        annotations:
          summary: "High external API error rate"
          description: "{{ $labels.api_name }} error rate is {{ $value | humanizePercentage }}"
```

## Troubleshooting

### Common Issues

1. **Circuit Breaker Opens Frequently**
   - **Cause**: External service instability
   - **Solution**: Increase `ConsecutiveFailureThreshold` or implement retry with backoff

2. **Circuit Breaker Never Recovers**
   - **Cause**: External service is completely down
   - **Solution**: Check service status, implement alternative endpoints

3. **Performance Degradation**
   - **Cause**: Half-open state testing is too aggressive
   - **Solution**: Adjust `MaxRequests` in half-open state

### Debug Commands

```bash
# Check current circuit breaker states
curl -s http://localhost:8080/metrics | grep icy_backend_circuit_breaker_state

# Monitor circuit breaker events in real-time
tail -f /var/log/icy-backend.log | grep -E "(circuit breaker|Circuit breaker)"

# Check external API health
curl http://localhost:8080/health/external

# Review error patterns
grep "External API call failed" /var/log/icy-backend.log | tail -20
```

### Performance Tuning

If circuit breakers are too sensitive or not responsive enough:

1. **Adjust Failure Threshold**:
   ```go
   ConsecutiveFailureThreshold: 5  // More tolerant
   ConsecutiveFailureThreshold: 2  // More sensitive
   ```

2. **Modify Recovery Time**:
   ```go
   Timeout: 30 * time.Second   // Faster recovery
   Timeout: 180 * time.Second  // Slower recovery
   ```

3. **Change Test Request Limits**:
   ```go
   MaxRequests: 10  // More aggressive testing
   MaxRequests: 1   // Conservative testing
   ```

## Integration with Business Metrics

Circuit breaker state affects business operations:

- **Oracle Operations**: Price fetches may fail when circuit breakers are open
- **Swap Operations**: Transaction processing is blocked
- **Background Jobs**: Transaction indexing is paused
- **User Experience**: API endpoints may return degraded responses

Monitor business impact with:
```promql
# Business operations affected by circuit breaker
rate(icy_backend_business_operations_total{status="error"}[5m]) by (operation_type)
```

## Future Enhancements

Potential improvements to consider:

1. **Manual Reset Endpoint**: Allow operators to manually reset circuit breakers
2. **Adaptive Thresholds**: Adjust failure thresholds based on service patterns
3. **Circuit Breaker Dashboard**: Dedicated UI for circuit breaker monitoring
4. **Notification Integration**: Slack/email notifications for state changes
5. **Service-Specific Timeouts**: Different timeout values per operation type

## Conclusion

Circuit breakers provide essential protection against external service failures in the ICY Backend. Proper monitoring and operational procedures ensure system reliability while maintaining service availability. Regular review of circuit breaker configurations and thresholds helps optimize the balance between protection and availability.