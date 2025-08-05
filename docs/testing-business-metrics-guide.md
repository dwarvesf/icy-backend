# Testing Business Logic Metrics - ICY Backend

This guide provides comprehensive instructions for testing all newly implemented business logic metrics in the ICY Backend monitoring system.

## Overview

We've implemented the following new business metrics:
- **Oracle Operations**: 4 endpoints with timing and cache metrics
- **Swap Operations**: 3 operations with comprehensive error handling
- **Cache Performance**: Hit/miss ratio tracking

## Testing Environment Setup

### 1. Start the Server
```bash
# Start development server
make dev

# Or using devbox
devbox run server
```

### 2. Verify Metrics Endpoint
```bash
curl http://localhost:8080/metrics | grep icy_backend_business
```

Expected output:
```prometheus
# HELP icy_backend_business_operations_total Total number of business operations
# TYPE icy_backend_business_operations_total counter
# HELP icy_backend_business_operation_duration_seconds Duration of business operations in seconds
# TYPE icy_backend_business_operation_duration_seconds histogram
# HELP icy_backend_cache_operations_total Total number of cache operations
# TYPE icy_backend_cache_operations_total counter
```

## Testing Oracle Metrics

### Test 1: Oracle Operations (Requires API Key)

**Note**: Oracle endpoints require API key authentication. You'll need a valid API key to test these.

```bash
# Set your API key
API_KEY="your-api-key-here"

# Test Circulated ICY
curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/oracle/circulated-icy

# Test Treasury BTC  
curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/oracle/treasury-btc

# Test Real-time ICY/BTC Ratio
curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/oracle/icy-btc-ratio

# Test Cached ICY/BTC Ratio (this will test cache metrics)
curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/oracle/icy-btc-ratio-cached
```

### Test 2: Verify Oracle Metrics
```bash
# Check Oracle operation metrics
curl -s http://localhost:8080/metrics | grep "icy_backend_business_operations_total.*oracle_operation"

# Check Oracle duration metrics
curl -s http://localhost:8080/metrics | grep "icy_backend_business_operation_duration_seconds.*oracle_operation"

# Check cache hit/miss metrics
curl -s http://localhost:8080/metrics | grep "icy_backend_cache_operations_total"
```

Expected Oracle metrics:
```prometheus
icy_backend_business_operations_total{operation_type="oracle_operation",category="circulated_icy",status="success"} 1
icy_backend_business_operations_total{operation_type="oracle_operation",category="treasury_btc",status="success"} 1
icy_backend_business_operations_total{operation_type="oracle_operation",category="icy_btc_ratio_realtime",status="success"} 1
icy_backend_business_operations_total{operation_type="oracle_operation",category="icy_btc_ratio_cached",status="success"} 1
icy_backend_cache_operations_total{cache_type="oracle_price",operation="hit"} 1
```

### Test 3: Oracle Error Scenarios

Test without API key to trigger error metrics:
```bash
# This should trigger error metrics
curl http://localhost:8080/api/v1/oracle/circulated-icy

# Check error metrics
curl -s http://localhost:8080/metrics | grep "oracle_operation.*error"
```

## Testing Swap Metrics

### Test 1: Swap Info Endpoint (No Auth Required)

```bash
# Test swap info - this triggers multiple Oracle calls
curl http://localhost:8080/api/v1/swap/info
```

### Test 2: Generate Signature (Requires API Key)

```bash
# Test signature generation
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "icy_amount": "1000000000000000000",
    "btc_address": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
    "btc_amount": "100000"
  }' \
  http://localhost:8080/api/v1/swap/generate-signature
```

### Test 3: Create Swap Request (Requires API Key)

```bash
# Test swap request creation
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "icy_amount": "1000000000000000000",
    "btc_address": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
    "icy_tx": "0x1234567890abcdef1234567890abcdef12345678"
  }' \
  http://localhost:8080/api/v1/swap
```

### Test 4: Verify Swap Metrics

```bash
# Check swap operation metrics
curl -s http://localhost:8080/metrics | grep "icy_backend_business_operations_total.*swap_operation"

# Check swap duration metrics  
curl -s http://localhost:8080/metrics | grep "icy_backend_business_operation_duration_seconds.*swap_operation"
```

Expected Swap metrics:
```prometheus
icy_backend_business_operations_total{operation_type="swap_operation",category="swap_info",status="success"} 1
icy_backend_business_operations_total{operation_type="swap_operation",category="generate_signature",status="success"} 1
icy_backend_business_operations_total{operation_type="swap_operation",category="create_swap_request",status="success"} 1
```

## Testing Error Scenarios

### Test 1: Invalid Request Data

```bash
# Test invalid Oracle requests (should trigger validation errors)
curl -H "X-API-Key: $API_KEY" http://localhost:8080/api/v1/oracle/invalid-endpoint

# Test invalid Swap data
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"invalid": "data"}' \
  http://localhost:8080/api/v1/swap
```

### Test 2: Check Error Metrics

```bash
# Check for error status metrics
curl -s http://localhost:8080/metrics | grep "status=\"error\""
curl -s http://localhost:8080/metrics | grep "status=\"validation_error\""
```

## Automated Testing Script

Create a comprehensive test script:

```bash
#!/bin/bash
# test-business-metrics.sh

API_KEY="${API_KEY:-your-api-key-here}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
METRICS_URL="$BASE_URL/metrics"

echo "🔍 Testing ICY Backend Business Metrics"
echo "========================================"

# Function to check metrics
check_metrics() {
    local metric_pattern="$1"
    local description="$2"
    
    echo "📊 Checking: $description"
    curl -s "$METRICS_URL" | grep "$metric_pattern" | head -5
    echo ""
}

# Test Swap Info (no auth required)
echo "🔄 Testing Swap Info endpoint..."
curl -s "$BASE_URL/api/v1/swap/info" > /dev/null
check_metrics "swap_operation.*swap_info" "Swap Info Metrics"

# Test Oracle endpoints (if API key provided)
if [ "$API_KEY" != "your-api-key-here" ]; then
    echo "🔮 Testing Oracle endpoints..."
    
    curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/v1/oracle/circulated-icy" > /dev/null
    curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/v1/oracle/treasury-btc" > /dev/null
    curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/v1/oracle/icy-btc-ratio" > /dev/null
    curl -s -H "X-API-Key: $API_KEY" "$BASE_URL/api/v1/oracle/icy-btc-ratio-cached" > /dev/null
    
    check_metrics "oracle_operation" "Oracle Operation Metrics"
    check_metrics "cache_operations" "Cache Metrics"
else
    echo "⚠️  Skipping Oracle tests (no API key provided)"
    echo "   Set API_KEY environment variable to test Oracle endpoints"
fi

# Test error scenarios
echo "❌ Testing error scenarios..."
curl -s "$BASE_URL/api/v1/oracle/circulated-icy" > /dev/null  # Should fail without API key
check_metrics "status=\"error\"" "Error Metrics"

# Summary
echo "📈 Business Metrics Summary:"
echo "============================"
check_metrics "icy_backend_business_operations_total" "All Business Operations"
check_metrics "icy_backend_business_operation_duration_seconds_count" "Operation Counts"

echo "✅ Testing completed!"
echo ""
echo "💡 Tips:"
echo "   - Set API_KEY environment variable for full testing"
echo "   - Monitor metrics at: $METRICS_URL"
echo "   - Use Grafana dashboards for visualization"
```

### Run the Test Script

```bash
# Make it executable
chmod +x test-business-metrics.sh

# Run with API key
API_KEY="your-api-key" ./test-business-metrics.sh

# Or run without API key (limited testing)
./test-business-metrics.sh
```

## Production Testing on api.icy.so

### Test Production Metrics

```bash
# Test Swap Info on production (no auth required)
curl https://api.icy.so/api/v1/swap/info

# Check production metrics
curl https://api.icy.so/metrics | grep icy_backend_business
```

### Production Test Results

Since the production server is running, you should see:
```prometheus
icy_backend_business_operations_total{operation_type="swap_operation",category="swap_info",status="success"} N
icy_backend_business_operation_duration_seconds_count{operation_type="swap_operation",category="swap_info",status="success"} N
```

## Monitoring Integration

### Prometheus Queries for Testing

```promql
# Business operation rates
rate(icy_backend_business_operations_total[5m])

# Business operation duration percentiles
histogram_quantile(0.95, rate(icy_backend_business_operation_duration_seconds_bucket[5m]))

# Cache hit rate
rate(icy_backend_cache_operations_total{operation="hit"}[5m]) / 
rate(icy_backend_cache_operations_total[5m])

# Error rates by operation
rate(icy_backend_business_operations_total{status="error"}[5m]) by (operation_type, category)
```

### Grafana Dashboard Panels

Create panels to visualize:
1. **Business Operation Rates**: Line chart of operations per second
2. **Response Time Distribution**: Histogram of operation durations  
3. **Cache Hit Rate**: Gauge showing cache performance
4. **Error Rate by Operation**: Bar chart of error rates
5. **Top Operations**: Table of most frequently used operations

## Troubleshooting

### Common Issues

1. **No metrics appearing**:
   - Verify server is running on correct port
   - Check `/metrics` endpoint accessibility
   - Ensure business metrics are registered

2. **Oracle metrics missing**:
   - API key required for Oracle endpoints
   - Check authentication headers
   - Verify endpoints return success responses

3. **Cache metrics not updating**:
   - Cache hit detection based on response time (<10ms)
   - May need multiple requests to see patterns
   - Check Oracle cached endpoint specifically

### Debug Commands

```bash
# Check if business metrics are registered
curl -s http://localhost:8080/metrics | grep -E "TYPE.*icy_backend_business"

# Verify specific metric labels
curl -s http://localhost:8080/metrics | grep "oracle_operation" | sort

# Check metric counts
curl -s http://localhost:8080/metrics | grep "_total{.*}" | wc -l
```

## Conclusion

This comprehensive testing approach ensures all business logic metrics are working correctly:

✅ **Oracle Operations**: Timing, success/error rates, cache performance  
✅ **Swap Operations**: Complete pipeline monitoring  
✅ **Error Handling**: Comprehensive error classification  
✅ **Cache Metrics**: Hit/miss ratio tracking  
✅ **Production Ready**: Tested on live environment

The metrics provide complete visibility into business logic performance and enable proactive monitoring of the cryptocurrency swap system.