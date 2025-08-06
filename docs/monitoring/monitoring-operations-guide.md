# ICY Backend Monitoring Operations Guide

## Overview

This guide provides comprehensive instructions for monitoring the ICY Backend cryptocurrency swap system using the implemented health endpoints, Prometheus metrics, and observability features.

## Table of Contents

1. [Health Check Endpoints](#health-check-endpoints)
2. [Prometheus Metrics](#prometheus-metrics)
3. [Alerting Configuration](#alerting-configuration)
4. [Dashboard Setup](#dashboard-setup)
5. [Operational Procedures](#operational-procedures)
6. [Troubleshooting Guide](#troubleshooting-guide)
7. [Performance Baselines](#performance-baselines)

---

## Health Check Endpoints

### 1. Basic System Health - `/healthz`

**Purpose**: Quick system availability check  
**SLA**: < 200ms response time, 99.9% availability  
**Authentication**: None required

#### Usage
```bash
curl http://localhost:8080/healthz
```

#### Expected Response
```json
{"message": "ok"}
```

#### Status Codes
- `200 OK`: System is healthy and operational
- `5xx`: System is experiencing issues

#### Monitoring Script
```bash
#!/bin/bash
# basic-health-check.sh
ENDPOINT="http://localhost:8080/healthz"
RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code};TIME:%{time_total}" $ENDPOINT)
HTTP_CODE=$(echo $RESPONSE | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
TIME=$(echo $RESPONSE | grep -o "TIME:[0-9.]*" | cut -d: -f2)

if [ "$HTTP_CODE" -eq 200 ]; then
    echo "✅ System healthy - Response time: ${TIME}s"
else
    echo "❌ System unhealthy - HTTP $HTTP_CODE"
    exit 1
fi
```

### 2. Database Health - `/api/v1/health/db`

**Purpose**: Database connectivity and performance validation  
**SLA**: < 500ms response time, 99.5% availability  
**Authentication**: None required

#### Usage
```bash
curl http://localhost:8080/api/v1/health/db
```

#### Expected Response
```json
{
  "status": "healthy",
  "timestamp": "2025-08-05T16:28:59.134262+07:00",
  "checks": {
    "database": {
      "status": "healthy",
      "latency_ms": 1,
      "metadata": {
        "connection_pool": {
          "idle": 2,
          "in_use": 0,
          "max_open": 0,
          "open_connections": 2
        },
        "driver": "postgres"
      }
    }
  },
  "duration_ms": 1
}
```

#### Key Metrics to Monitor
- **status**: `healthy` or `unhealthy`
- **latency_ms**: Database query response time
- **connection_pool.idle**: Available database connections
- **connection_pool.in_use**: Active database connections
- **duration_ms**: Total health check duration

#### Alert Conditions
- **Critical**: `status != "healthy"`
- **Warning**: `latency_ms > 100`
- **Warning**: `connection_pool.idle < 1`
- **Critical**: `duration_ms > 500`

### 3. External Dependencies Health - `/api/v1/health/external`

**Purpose**: External API connectivity and performance validation  
**SLA**: < 2000ms response time, 95% availability  
**Authentication**: None required

#### Usage
```bash
curl http://localhost:8080/api/v1/health/external
```

#### Expected Response
```json
{
  "status": "healthy|degraded|unhealthy",
  "timestamp": "2025-08-05T16:29:04.527731+07:00",
  "checks": {
    "base_rpc": {
      "status": "healthy",
      "latency_ms": 233,
      "metadata": {
        "endpoint": "base_rpc"
      }
    },
    "blockstream_api": {
      "status": "unhealthy",
      "error": "circuit breaker is open"
    }
  },
  "duration_ms": 233
}
```

#### Status Logic
- **healthy**: All external services responding within SLA
- **degraded**: One service failing or high latency (>2s)
- **unhealthy**: Multiple services failing or critical service down

#### Key Metrics to Monitor
- **Overall status**: System-wide external dependency health
- **Service-specific status**: Individual API health
- **latency_ms**: Response time for each service
- **Circuit breaker states**: Protection mechanism status

#### Alert Conditions
- **Critical**: `status == "unhealthy"`
- **Warning**: `status == "degraded"`
- **Warning**: Any service `latency_ms > 1000`
- **Critical**: Circuit breaker open for > 5 minutes

### 4. Background Jobs Health - `/api/v1/health/jobs`

**Purpose**: Background job execution status and performance  
**Authentication**: None required

#### Usage
```bash
curl http://localhost:8080/api/v1/health/jobs
```

#### Expected Response
```json
{
  "status": "healthy|degraded|unhealthy",
  "timestamp": "2025-08-05T16:29:10.035674+07:00",
  "jobs": {
    "btc_transaction_indexing": {
      "job_name": "btc_transaction_indexing",
      "status": "success|failed|running",
      "last_run_time": "2025-08-05T16:29:07.001158+07:00",
      "last_duration_ms": 5129542,
      "success_count": 2,
      "failure_count": 1,
      "consecutive_failures": 1,
      "last_error": "circuit breaker is open",
      "average_execution_ms": 3218649513,
      "max_execution_ms": 9453990958,
      "min_execution_ms": 5129542,
      "metadata": {
        "error_type": "unknown"
      }
    }
  },
  "summary": {
    "total_jobs": 5,
    "running_jobs": 1,
    "healthy_jobs": 2,
    "unhealthy_jobs": 2,
    "stalled_jobs": 0,
    "last_update_time": "2025-08-05T16:29:10.035673+07:00"
  },
  "duration_ms": 0
}
```

#### Key Jobs to Monitor
1. **btc_transaction_indexing**: Bitcoin blockchain transaction processing
2. **icy_transaction_indexing**: ICY token transaction processing
3. **icy_swap_transaction_indexing**: Swap transaction processing
4. **swap_request_processing**: Swap request handling
5. **btc_pending_transaction_processing**: Bitcoin transaction finalization

#### Alert Conditions
- **Critical**: `status == "unhealthy"`
- **Warning**: `consecutive_failures > 3`
- **Critical**: `stalled_jobs > 0`
- **Warning**: Job execution time > baseline + 200%
- **Critical**: No job execution in > 15 minutes

---

## Prometheus Metrics

### Accessing Metrics

```bash
curl http://localhost:8080/metrics
```

### Key Metric Categories

#### 1. HTTP Request Metrics

**Request Duration**
```prometheus
icy_backend_http_request_duration_seconds{method, path, status}
```
- **Type**: Histogram
- **Labels**: HTTP method, path, response status
- **Buckets**: 0.005s to 10s
- **Use**: Track API response times

**Request Count**
```prometheus
icy_backend_http_requests_total{method, path, status}
```
- **Type**: Counter
- **Labels**: HTTP method, path, response status
- **Use**: Track request volume and error rates

**Response Size**
```prometheus
icy_backend_http_response_size_bytes{method, path}
```
- **Type**: Histogram
- **Labels**: HTTP method, path
- **Use**: Track response payload sizes

**In-Flight Requests**
```prometheus
icy_backend_http_requests_in_flight
```
- **Type**: Gauge
- **Use**: Track concurrent request load

#### 2. Background Job Metrics

**Job Duration**
```prometheus
icy_backend_background_job_duration_seconds{job_name, status}
```
- **Type**: Histogram
- **Labels**: Job name, execution status
- **Use**: Track job execution performance

**Job Execution Count**
```prometheus
icy_backend_background_job_runs_total{job_name, status}
```
- **Type**: Counter
- **Labels**: Job name, execution status
- **Use**: Track job success/failure rates

#### 3. External API Metrics

**API Call Duration**
```prometheus
icy_backend_external_api_duration_seconds{api_name, status}
```
- **Type**: Histogram
- **Labels**: API name, response status
- **Use**: Track external API performance

**Circuit Breaker State**
```prometheus
icy_backend_circuit_breaker_state{api_name}
```
- **Type**: Gauge
- **Labels**: API name
- **Values**: 0 (closed), 1 (half-open), 2 (open)
- **Use**: Monitor circuit breaker status

#### 4. Business Logic Metrics

**Business Operations**
```prometheus
icy_backend_business_operations_total{operation, category, status}
```
- **Type**: Counter
- **Labels**: Operation type, category, status
- **Use**: Track business logic execution

---

## Operational Procedures

### Daily Monitoring Checklist

#### Morning Health Check (9:00 AM)
```bash
#!/bin/bash
# daily-health-check.sh

echo "🔍 ICY Backend Daily Health Check - $(date)"
echo "================================================"

# 1. Basic system health
echo "1. Basic System Health:"
curl -s http://localhost:8080/healthz | jq .
echo ""

# 2. Database health
echo "2. Database Health:"
curl -s http://localhost:8080/api/v1/health/db | jq '.checks.database | {status, latency_ms, connection_pool}'
echo ""

# 3. External dependencies
echo "3. External Dependencies:"
curl -s http://localhost:8080/api/v1/health/external | jq '{status, checks}'
echo ""

# 4. Background jobs summary
echo "4. Background Jobs Summary:"
curl -s http://localhost:8080/api/v1/health/jobs | jq '.summary'
echo ""

# 5. Failed jobs details
echo "5. Failed Jobs (if any):"
curl -s http://localhost:8080/api/v1/health/jobs | jq '.jobs | to_entries[] | select(.value.status == "failed") | {job: .key, error: .value.last_error, failures: .value.consecutive_failures}'
echo ""

echo "✅ Daily health check completed"
```

### Incident Response Procedures

#### 1. System Down (Critical)

**Detection**: Health check returns 5xx or no response

**Immediate Actions**:
1. Check system logs: `tail -f /var/log/icy-backend/app.log`
2. Verify database connectivity: `curl http://localhost:8080/api/v1/health/db`
3. Check resource usage: `top`, `df -h`, `free -m`
4. Restart service if necessary: `systemctl restart icy-backend`

**Investigation**:
1. Review recent deployments
2. Check external dependency status
3. Analyze error patterns in logs
4. Review resource utilization trends

#### 2. High Error Rate (Critical)

**Detection**: Error rate > 20% for > 1 minute

**Immediate Actions**:
1. Check specific failing endpoints: `curl http://localhost:8080/api/v1/health/external`
2. Review recent error logs: `grep ERROR /var/log/icy-backend/app.log | tail -50`
3. Check circuit breaker status
4. Verify database connectivity

**Investigation**:
1. Analyze error patterns by endpoint
2. Check external API response times
3. Review recent code changes
4. Monitor recovery after fixes

#### 3. Background Job Failures (High)

**Detection**: Multiple job failures or jobs not running

**Immediate Actions**:
1. Check job status: `curl http://localhost:8080/api/v1/health/jobs`
2. Review job-specific logs
3. Check external API connectivity (for indexing jobs)
4. Verify database connectivity

**Investigation**:
1. Identify root cause (timeout, external API, database)
2. Check job execution patterns
3. Review job configuration
4. Monitor job recovery

---

## Troubleshooting Guide

### Common Issues and Solutions

#### 1. Database Health Check Failing

**Symptoms**:
- `/api/v1/health/db` returns unhealthy status
- High database latency
- Connection pool exhaustion

**Diagnosis**:
```bash
# Check database connectivity
curl http://localhost:8080/api/v1/health/db | jq '.checks.database'

# Check connection pool status
curl http://localhost:8080/api/v1/health/db | jq '.checks.database.metadata.connection_pool'
```

**Solutions**:
1. **High Latency**:
   - Check database server performance
   - Review slow query logs
   - Optimize database queries
   - Consider connection pooling adjustments

2. **Connection Pool Issues**:
   - Increase max connections if needed
   - Check for connection leaks
   - Monitor connection usage patterns
   - Restart application if necessary

#### 2. Circuit Breaker Open

**Symptoms**:
- External API calls failing
- Circuit breaker status shows "open"
- Health check shows degraded status

**Diagnosis**:
```bash
# Check circuit breaker status
curl http://localhost:8080/api/v1/health/external | jq '.checks'

# Check circuit breaker metrics
curl http://localhost:8080/metrics | grep circuit_breaker
```

**Solutions**:
1. **Blockstream API Issues**:
   - Verify API endpoint accessibility
   - Check API rate limits
   - Review API key configuration
   - Consider alternative endpoints

2. **Base RPC Issues**:
   - Verify RPC endpoint configuration
   - Check network connectivity
   - Review authentication settings
   - Monitor provider status

#### 3. Background Job Performance Issues

**Symptoms**:
- Jobs taking longer than expected
- High failure rates
- Jobs timing out

**Diagnosis**:
```bash
# Check job performance
curl http://localhost:8080/api/v1/health/jobs | jq '.jobs | to_entries[] | {job: .key, avg_time: .value.average_execution_ms, failures: .value.failure_count}'

# Check recent job errors
curl http://localhost:8080/api/v1/health/jobs | jq '.jobs | to_entries[] | select(.value.failure_count > 0) | {job: .key, error: .value.last_error}'
```

**Solutions**:
1. **Timeout Issues**:
   - Increase job timeout configuration
   - Optimize job logic for performance
   - Break large jobs into smaller chunks
   - Review external API dependencies

2. **High Failure Rates**:
   - Review error patterns
   - Check external API connectivity
   - Verify database connectivity
   - Review job logic and error handling

#### 4. High Memory Usage

**Symptoms**:
- System memory usage increasing
- Potential memory leaks
- Performance degradation

**Diagnosis**:
```bash
# Check system memory
free -h

# Check Go memory stats via metrics
curl http://localhost:8080/metrics | grep go_memstats

# Monitor memory growth over time
watch -n 5 'curl -s http://localhost:8080/metrics | grep go_memstats_alloc_bytes'
```

**Solutions**:
1. **Memory Leaks**:
   - Review code for resource leaks
   - Check goroutine counts
   - Monitor garbage collection metrics
   - Consider memory profiling

2. **High Memory Usage**:
   - Optimize data structures
   - Implement proper caching strategies
   - Review metric cardinality
   - Consider increasing system memory

---

## Performance Baselines

### Expected Performance Metrics

#### Health Endpoints
- **Basic Health** (`/healthz`): < 5ms (target: < 200ms)
- **Database Health** (`/api/v1/health/db`): < 50ms (target: < 500ms)
- **External Health** (`/api/v1/health/external`): < 1000ms (target: < 2000ms)
- **Jobs Health** (`/api/v1/health/jobs`): < 10ms

#### API Performance
- **P50 Response Time**: < 100ms
- **P95 Response Time**: < 500ms
- **P99 Response Time**: < 1000ms
- **Error Rate**: < 1%

#### Background Jobs
- **BTC Transaction Indexing**: < 30s per run
- **ICY Transaction Indexing**: < 30s per run
- **Swap Request Processing**: < 60s per run
- **Job Success Rate**: > 95%

#### External APIs
- **Blockstream API**: < 2s response time
- **Base RPC**: < 1s response time
- **Circuit Breaker Trips**: < 1 per hour

### Performance Monitoring Queries

#### Prometheus Queries for Performance Analysis

**API Response Time Analysis**:
```promql
# P95 response time by endpoint
histogram_quantile(0.95, rate(icy_backend_http_request_duration_seconds_bucket[5m]))

# Response time by status code
histogram_quantile(0.95, rate(icy_backend_http_request_duration_seconds_bucket{status=~"2.."}[5m]))
```

**Error Rate Analysis**:
```promql
# Overall error rate
rate(icy_backend_http_requests_total{status=~"5.."}[5m]) / rate(icy_backend_http_requests_total[5m])

# Error rate by endpoint
rate(icy_backend_http_requests_total{status=~"5.."}[5m]) by (path)
```

**Background Job Analysis**:
```promql
# Average job execution time
rate(icy_backend_background_job_duration_seconds_sum[5m]) / rate(icy_backend_background_job_duration_seconds_count[5m])

# Job failure rate
rate(icy_backend_background_job_runs_total{status="failed"}[5m]) by (job_name)
```

---

## Security Monitoring

### Security Metrics to Monitor

1. **Authentication Failures**: Monitor failed API key validations
2. **Rate Limiting**: Track rate limit violations
3. **Error Patterns**: Monitor for potential attack patterns
4. **Resource Usage**: Watch for unusual resource consumption

### Security Alerts

```yaml
# Security-focused alert rules
- alert: HighAuthenticationFailures
  expr: rate(icy_backend_http_requests_total{status="401"}[5m]) > 10
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "High authentication failure rate"
    description: "Unusual number of authentication failures detected"

- alert: UnusualTrafficPattern
  expr: rate(icy_backend_http_requests_total[5m]) > 100
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "Unusual traffic pattern detected"
    description: "Request rate significantly higher than normal"
```

---

## Conclusion

This comprehensive monitoring guide provides all the tools and procedures needed to effectively monitor the ICY Backend cryptocurrency swap system. Regular use of these monitoring practices will help ensure system reliability, performance, and security.

For additional support or questions about monitoring procedures, refer to the session documentation in `/docs/sessions/2025-08-05-1456/` or contact the development team.

**Key Monitoring URLs**:
- Health Check: `http://localhost:8080/healthz`
- Database Health: `http://localhost:8080/api/v1/health/db`
- External Dependencies: `http://localhost:8080/api/v1/health/external`
- Background Jobs: `http://localhost:8080/api/v1/health/jobs`
- Prometheus Metrics: `http://localhost:8080/metrics`

**Next Steps**:
1. Set up Prometheus scraping configuration
2. Configure AlertManager with team notification channels
3. Create Grafana dashboards using provided configurations
4. Implement daily and weekly monitoring procedures
5. Train team on incident response procedures