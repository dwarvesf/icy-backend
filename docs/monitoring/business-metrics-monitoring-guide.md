# Business Metrics Monitoring Guide - ICY Backend

This guide provides comprehensive instructions for monitoring the business logic metrics implemented in the ICY Backend cryptocurrency swap system.

## Overview

The ICY Backend exposes detailed business metrics through Prometheus endpoints to provide visibility into:
- Oracle operations performance and reliability
- Swap operations success rates and timing
- Cache performance and hit/miss ratios
- Background job health and processing rates
- External API reliability and circuit breaker status

## Accessing Metrics

### Metrics Endpoint
```
GET /metrics
```

### Quick Health Check
```bash
curl http://localhost:8080/metrics | grep icy_backend_business
```

## Key Business Metrics

### 1. Oracle Operations Metrics

#### Oracle Operation Counts
```promql
# Total Oracle operations by type and status
icy_backend_business_operations_total{operation_type="oracle_operation"}

# Oracle operation rates (requests per second)
rate(icy_backend_business_operations_total{operation_type="oracle_operation"}[5m])
```

**Available Oracle Categories:**
- `circulated_icy` - Total ICY token supply queries
- `treasury_btc` - BTC treasury balance queries
- `icy_btc_ratio_realtime` - Real-time ICY/BTC price ratio
- `icy_btc_ratio_cached` - Cached ICY/BTC price ratio

#### Oracle Response Times
```promql
# Oracle operation duration percentiles
histogram_quantile(0.95, rate(icy_backend_business_operation_duration_seconds_bucket{operation_type="oracle_operation"}[5m]))

# Average Oracle response time
rate(icy_backend_business_operation_duration_seconds_sum{operation_type="oracle_operation"}[5m]) / 
rate(icy_backend_business_operation_duration_seconds_count{operation_type="oracle_operation"}[5m])
```

#### Oracle Error Rates
```promql
# Oracle error rate by category
rate(icy_backend_business_operations_total{operation_type="oracle_operation",status="error"}[5m]) / 
rate(icy_backend_business_operations_total{operation_type="oracle_operation"}[5m])
```

### 2. Swap Operations Metrics

#### Swap Operation Counts
```promql
# Total Swap operations by type and status
icy_backend_business_operations_total{operation_type="swap_operation"}

# Swap operation rates
rate(icy_backend_business_operations_total{operation_type="swap_operation"}[5m])
```

**Available Swap Categories:**
- `swap_info` - Swap information queries
- `generate_signature` - Signature generation for swaps
- `create_swap_request` - New swap request creation

#### Swap Response Times
```promql
# Swap operation duration percentiles
histogram_quantile(0.95, rate(icy_backend_business_operation_duration_seconds_bucket{operation_type="swap_operation"}[5m]))
```

#### Swap Success Rates
```promql
# Swap success rate
rate(icy_backend_business_operations_total{operation_type="swap_operation",status="success"}[5m]) / 
rate(icy_backend_business_operations_total{operation_type="swap_operation"}[5m])

# Swap partial success rate (degraded performance)
rate(icy_backend_business_operations_total{operation_type="swap_operation",status="partial_success"}[5m]) / 
rate(icy_backend_business_operations_total{operation_type="swap_operation"}[5m])
```

### 3. Cache Performance Metrics

#### Cache Hit Rate
```promql
# Overall cache hit rate
rate(icy_backend_cache_operations_total{operation="hit"}[5m]) / 
rate(icy_backend_cache_operations_total[5m])

# Cache hit rate by type
rate(icy_backend_cache_operations_total{cache_type="oracle_price",operation="hit"}[5m]) / 
rate(icy_backend_cache_operations_total{cache_type="oracle_price"}[5m])
```

#### Cache Operations
```promql
# Cache operations per second
rate(icy_backend_cache_operations_total[5m])

# Cache miss rate
rate(icy_backend_cache_operations_total{operation="miss"}[5m]) / 
rate(icy_backend_cache_operations_total[5m])
```

### 4. Background Job Metrics

#### Job Success Rates
```promql
# Background job success rate
rate(icy_backend_background_job_runs_total{status="success"}[5m]) / 
rate(icy_backend_background_job_runs_total[5m])

# Job failure rate by job type
rate(icy_backend_background_job_runs_total{status="error"}[5m]) by (job_name)
```

#### Job Duration
```promql
# Job duration by type
histogram_quantile(0.95, rate(icy_backend_background_job_duration_seconds_bucket[5m])) by (job_name)
```

### 5. External API Metrics

#### API Success Rates
```promql
# External API success rate
rate(icy_backend_external_api_calls_total{status="success"}[5m]) / 
rate(icy_backend_external_api_calls_total[5m])

# API failure rate by service
rate(icy_backend_external_api_calls_total{status="error"}[5m]) by (api_name)
```

#### Circuit Breaker Status
```promql
# Circuit breaker state (1 = open, 0 = closed)
icy_backend_circuit_breaker_state

# Circuit breaker trips per minute
rate(icy_backend_circuit_breaker_state_changes_total{state="open"}[1m])
```

### Recommended Dashboard Layout

1. **Top Row**: High-level KPIs
   - Total operations per second
   - Overall success rate
   - Average response time
   - Cache hit rate

2. **Second Row**: Oracle Operations
   - Oracle operation rates by category
   - Oracle response time percentiles
   - Oracle error rates

3. **Third Row**: Swap Operations
   - Swap operation rates by category
   - Swap response time percentiles
   - Swap success/partial success rates

4. **Fourth Row**: Infrastructure
   - Background job status
   - External API health
   - Circuit breaker status


## Operational Runbooks

### High Error Rate Response

1. **Check External APIs**: Verify circuit breaker status and external API health
2. **Review Recent Changes**: Check recent deployments or configuration changes  
3. **Examine Error Details**: Use debug logs to identify specific error patterns
4. **Check Database Health**: Verify database connection and performance
5. **Scale if Needed**: Consider horizontal scaling if load-related

### Cache Performance Issues

1. **Check Cache Backend**: Verify Redis/cache backend health
2. **Review Cache Configuration**: Ensure TTL settings are appropriate
3. **Monitor Memory Usage**: Check if cache is being evicted due to memory pressure
4. **Analyze Access Patterns**: Review which endpoints have low hit rates

### Circuit Breaker Management

1. **Identify Root Cause**: Check external service status and logs
2. **Manual Override**: If needed, manually close circuit breaker for critical operations
3. **Update Timeouts**: Adjust timeout thresholds if external services are slow
4. **Failover Planning**: Activate backup services or degraded mode

## Testing and Validation

### Manual Testing Commands
```bash
# Test Oracle endpoints to generate metrics
curl -H "X-API-Key: your-api-key" http://localhost:8080/api/v1/oracle/circulated-icy
curl -H "X-API-Key: your-api-key" http://localhost:8080/api/v1/oracle/treasury-btc
curl -H "X-API-Key: your-api-key" http://localhost:8080/api/v1/oracle/icy-btc-ratio
curl -H "X-API-Key: your-api-key" http://localhost:8080/api/v1/oracle/icy-btc-ratio-cached

# Test Swap endpoints
curl http://localhost:8080/api/v1/swap/info

# Check metrics
curl http://localhost:8080/metrics | grep icy_backend_business
```

### Automated Testing Script
Use the provided `/test-business-metrics.sh` script for comprehensive testing:
```bash
API_KEY="your-api-key" ./test-business-metrics.sh
```

## Production Deployment

### Environment Variables
```bash
# Enable metrics collection
METRICS_ENABLED=true

# Set appropriate log level for production
GIN_MODE=release
LOG_LEVEL=INFO

# Configure monitoring endpoints
METRICS_PORT=8080
HEALTH_CHECK_PORT=8080
```

### Prometheus Configuration
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'icy-backend'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s
    scrape_timeout: 10s
```

### Security Considerations

1. **Metrics Endpoint**: Consider restricting `/metrics` access to monitoring systems only
2. **API Keys**: Protect API keys used for testing Oracle endpoints
3. **Network Security**: Ensure monitoring traffic is encrypted in production
4. **Data Sensitivity**: Review metrics labels to ensure no sensitive data exposure

## Troubleshooting

### Common Issues

#### No Metrics Appearing
- Verify server is running and `/metrics` endpoint is accessible
- Check that business operations are being called
- Ensure metrics registration is working correctly

#### Metrics Not Updating
- Confirm requests are reaching the instrumented endpoints
- Verify API key authentication for Oracle endpoints
- Check for errors in application logs

#### High Memory Usage
- Monitor Prometheus metrics cardinality
- Consider reducing metric label dimensions if needed
- Implement metric retention policies

### Debug Commands
```bash
# Check metric registration
curl -s http://localhost:8080/metrics | grep -E "TYPE.*icy_backend_business"

# Verify specific operations
curl -s http://localhost:8080/metrics | grep "oracle_operation" | sort

# Check recent activity
curl -s http://localhost:8080/metrics | grep "_total{.*}" | sort -k2 -nr
```

## Conclusion

This monitoring setup provides comprehensive visibility into the ICY Backend's business logic performance, enabling:

- **Proactive Issue Detection**: Early warning of performance degradation
- **Capacity Planning**: Understanding of usage patterns and scaling needs  
- **SLA Monitoring**: Tracking of response times and success rates
- **Operational Insights**: Clear visibility into cache performance and external API health

Regular review of these metrics and alerts will ensure the cryptocurrency swap system maintains high availability and performance for users.