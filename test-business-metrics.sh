#!/bin/bash
# test-business-metrics.sh - Comprehensive Business Metrics Testing

API_KEY="${API_KEY:-your-api-key-here}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
METRICS_URL="$BASE_URL/metrics"

echo "🔍 Testing ICY Backend Business Metrics"
echo "========================================"
echo "Base URL: $BASE_URL"
echo "Metrics URL: $METRICS_URL"
echo ""

# Function to check metrics
check_metrics() {
    local metric_pattern="$1"
    local description="$2"
    
    echo "📊 Checking: $description"
    local metrics=$(curl -s "$METRICS_URL" 2>/dev/null | grep "$metric_pattern" | head -5)
    if [ -n "$metrics" ]; then
        echo "$metrics"
    else
        echo "   ❌ No metrics found for pattern: $metric_pattern"
    fi
    echo ""
}

# Function to test endpoint
test_endpoint() {
    local method="$1"
    local url="$2"  
    local data="$3"
    local headers="$4"
    local description="$5"
    
    echo "🔄 Testing: $description"
    echo "   URL: $method $url"
    
    if [ "$method" = "GET" ]; then
        if [ -n "$headers" ]; then
            response=$(curl -s -w "HTTP_CODE:%{http_code}" $headers "$url" 2>/dev/null)
        else
            response=$(curl -s -w "HTTP_CODE:%{http_code}" "$url" 2>/dev/null)
        fi
    else
        response=$(curl -s -w "HTTP_CODE:%{http_code}" -X "$method" $headers -d "$data" "$url" 2>/dev/null)
    fi
    
    http_code=$(echo "$response" | grep -o "HTTP_CODE:[0-9]*" | cut -d: -f2)
    echo "   Status: $http_code"
    
    if [ "$http_code" = "200" ]; then
        echo "   ✅ Success"
    elif [ "$http_code" = "401" ]; then
        echo "   ⚠️  Authentication required (expected for Oracle endpoints)"
    else
        echo "   ❌ Error (HTTP $http_code)"
    fi
    echo ""
}

# Verify metrics endpoint is accessible
echo "🏥 Testing Metrics Endpoint..."
metrics_response=$(curl -s -w "HTTP_CODE:%{http_code}" "$METRICS_URL" 2>/dev/null)
metrics_http_code=$(echo "$metrics_response" | grep -o "HTTP_CODE:[0-9]*" | cut -d: -f2)

if [ "$metrics_http_code" = "200" ]; then
    echo "✅ Metrics endpoint accessible"
    echo ""
else
    echo "❌ Metrics endpoint not accessible (HTTP $metrics_http_code)"
    echo "   Make sure the server is running on $BASE_URL"
    exit 1
fi

# Test 1: Swap Info (no auth required)
test_endpoint "GET" "$BASE_URL/api/v1/swap/info" "" "" "Swap Info"
check_metrics "swap_operation.*swap_info" "Swap Info Metrics"

# Test 2: Oracle endpoints (with/without API key)
if [ "$API_KEY" != "your-api-key-here" ] && [ -n "$API_KEY" ]; then
    echo "🔮 Testing Oracle endpoints with API key..."
    
    test_endpoint "GET" "$BASE_URL/api/v1/oracle/circulated-icy" "" "-H 'X-API-Key: $API_KEY'" "Oracle Circulated ICY"
    test_endpoint "GET" "$BASE_URL/api/v1/oracle/treasury-btc" "" "-H 'X-API-Key: $API_KEY'" "Oracle Treasury BTC"
    test_endpoint "GET" "$BASE_URL/api/v1/oracle/icy-btc-ratio" "" "-H 'X-API-Key: $API_KEY'" "Oracle ICY/BTC Ratio"
    test_endpoint "GET" "$BASE_URL/api/v1/oracle/icy-btc-ratio-cached" "" "-H 'X-API-Key: $API_KEY'" "Oracle ICY/BTC Ratio (Cached)"
    
    check_metrics "oracle_operation" "Oracle Operation Metrics"
    check_metrics "cache_operations" "Cache Metrics"
else
    echo "🔮 Testing Oracle endpoints without API key (will show auth errors)..."
    
    test_endpoint "GET" "$BASE_URL/api/v1/oracle/circulated-icy" "" "" "Oracle Circulated ICY (No Auth)"
    
    echo "   💡 To test Oracle metrics with authentication:"
    echo "      export API_KEY='your-actual-api-key'"
    echo "      ./test-business-metrics.sh"
    echo ""
fi

# Test 3: Swap operations (require API key)
if [ "$API_KEY" != "your-api-key-here" ] && [ -n "$API_KEY" ]; then
    echo "💱 Testing Swap operations with API key..."
    
    # Test signature generation
    signature_data='{"icy_amount":"1000000000000000000","btc_address":"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh","btc_amount":"100000"}'
    test_endpoint "POST" "$BASE_URL/api/v1/swap/generate-signature" "$signature_data" "-H 'Content-Type: application/json' -H 'X-API-Key: $API_KEY'" "Generate Signature"
    
    check_metrics "swap_operation.*generate_signature" "Signature Generation Metrics"
else
    echo "💱 Skipping authenticated Swap operations (no API key)"
    echo ""
fi

# Test 4: Error scenarios
echo "❌ Testing error scenarios..."
test_endpoint "GET" "$BASE_URL/api/v1/oracle/circulated-icy" "" "" "Oracle without Auth (Should Error)"
test_endpoint "POST" "$BASE_URL/api/v1/swap" '{"invalid":"data"}' "-H 'Content-Type: application/json'" "Invalid Swap Request"

check_metrics "status=\"error\"" "Error Metrics"

# Summary
echo "📈 Business Metrics Summary:"
echo "============================"
check_metrics "icy_backend_business_operations_total" "All Business Operations"
check_metrics "icy_backend_business_operation_duration_seconds_count" "Operation Duration Counts"
check_metrics "icy_backend_cache_operations_total" "Cache Operations"

# Check if any business metrics exist
business_metrics_count=$(curl -s "$METRICS_URL" 2>/dev/null | grep -c "icy_backend_business_operations_total")
cache_metrics_count=$(curl -s "$METRICS_URL" 2>/dev/null | grep -c "icy_backend_cache_operations_total")

echo "🎯 Metrics Verification:"
echo "========================"
echo "Business Operations: $business_metrics_count metrics found"
echo "Cache Operations: $cache_metrics_count metrics found"

if [ "$business_metrics_count" -gt 0 ]; then
    echo "✅ Business logic metrics are working!"
else
    echo "❌ No business logic metrics found"
    echo "   This might indicate the new instrumentation is not active"
fi

echo ""
echo "✅ Testing completed!"
echo ""
echo "💡 Tips:"
echo "   - Set API_KEY environment variable for full testing"
echo "   - Monitor metrics at: $METRICS_URL"
echo "   - Use 'curl $METRICS_URL | grep icy_backend_business' for quick check"
echo "   - Check the testing guide: docs/testing-business-metrics-guide.md"