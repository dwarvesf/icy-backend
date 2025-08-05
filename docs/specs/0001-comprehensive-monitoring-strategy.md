# Comprehensive Monitoring Strategy for ICY Backend

## Overview

This document outlines a comprehensive monitoring strategy for the ICY Backend cryptocurrency swap system, covering both **Synthetic Monitoring** (external black-box testing) and **Instrumentation Monitoring** (internal white-box metrics). This dual approach ensures complete visibility into system health, performance, and business logic.

## System Architecture Context

### Core Components
- **HTTP API Server**: Gin-based REST API running on port 3000
- **Database**: PostgreSQL with GORM ORM
- **Background Processing**: Cron jobs running every 2 minutes (configurable via `INDEX_INTERVAL`)
- **External Dependencies**: 
  - Blockstream API (Bitcoin blockchain)
  - Base chain RPC (Ethereum/ICY token)
- **Configuration**: Environment-based with HashiCorp Vault for production secrets

### Critical Business Functions
- BTC ↔ ICY token swaps
- Real-time price oracle data
- Transaction indexing and monitoring
- Treasury management
- Swap request processing

---

## Part I: Synthetic Monitoring (External Black-Box Testing)

Synthetic monitoring tests the system from an external perspective, simulating real user interactions and validating system behavior without knowledge of internal implementation.

### 1. Health Check Endpoints (Synthetic)

#### 1.1 Basic Health Endpoint
```http
GET /healthz
Purpose: Basic system availability check
Expected Response: {"message": "ok"}
SLA: < 200ms response time, 99.9% availability
Test Frequency: Every 60 seconds
```

#### 1.2 Database Health Check
```http
GET /api/v1/health/db
Purpose: Database connectivity and performance validation
Expected Response: {"status": "healthy", "latency_ms": 15}
SLA: < 500ms response time, 99.5% availability
Test Frequency: Every 2 minutes
```

#### 1.3 External Dependencies Health Check
```http
GET /api/v1/health/external
Purpose: External API connectivity and performance validation
Expected Response: {
  "status": "healthy|degraded|unhealthy",
  "timestamp": "2025-01-25T10:30:00Z",
  "services": {
    "blockstream_api": {
      "status": "healthy",
      "latency_ms": 120,
      "details": {
        "latest_block": 820450,
        "block_age_minutes": 5,
        "endpoints_tested": [
          "https://blockstream.info/api/blocks/tip",
          "https://blockstream.info/api/address/{test-address}"
        ]
      }
    },
    "base_rpc": {
      "status": "healthy", 
      "latency_ms": 80,
      "details": {
        "latest_block": 15234567,
        "chain_id": 8453,
        "icy_contract_accessible": true
      }
    }
  }
}

External Dependencies Monitored:
1. Blockstream API (Bitcoin Blockchain):
   - Bitcoin network connectivity
   - Latest block height retrieval  
   - Block freshness validation (< 20 minutes)
   - Address lookup functionality
   - Circuit breaker status monitoring

2. Base Chain RPC (Ethereum/ICY Token Network):
   - Base chain RPC connectivity
   - Latest block number retrieval
   - ICY token contract accessibility
   - Balance query functionality

Status Logic:
- healthy: All external services responding within SLA
- degraded: One service failing or high latency (>2s)
- unhealthy: Multiple services failing or critical service down

SLA: < 2000ms response time, 95% availability
Test Frequency: Every 5 minutes
```

### 2. Business Logic Validation (Synthetic)

#### 2.1 Oracle Data Endpoints
```http
GET /api/v1/oracle/icy-btc-ratio
Purpose: Price oracle functionality and business logic validation
Business Logic Validation:
- Ratio must be between 0.0001 and 1.0
- Must be positive number
- Should not deviate >10% from cached version within 5 minutes
SLA: < 1000ms response time, 99% availability
Test Frequency: Every 5 minutes

GET /api/v1/oracle/icy-btc-ratio-cached
Purpose: Cache consistency validation
Validation Rules:
- Compare with non-cached version
- Variance should be <5% under normal conditions
- Cache should be fresher than 5 minutes
SLA: < 200ms response time, 99.9% availability
Test Frequency: Every 2 minutes
```

#### 2.2 Swap Operation Endpoints
```http
GET /api/v1/swap/info
Purpose: Swap information and fee calculation validation
Business Logic Validation:
- Fee calculation accuracy
- Minimum/maximum swap amounts
- Current exchange rates
- Service availability status
SLA: < 300ms response time, 99.8% availability
Test Frequency: Every 5 minutes
```

---

## Part II: Instrumentation Monitoring (Internal White-Box Metrics)

Instrumentation monitoring involves adding metrics collection directly into the application code to track internal performance, business metrics, and system health.

### 1. HTTP Request Metrics (Instrumentation)

#### 1.1 Request Duration Metrics
```go
// Prometheus histogram for HTTP request duration
icy_backend_http_request_duration_seconds{method, endpoint, status}

Tracks:
- Request response times by endpoint
- Success/error rates by status code
- Performance trends over time

Alerting:
- Warning: 95th percentile > 1 second
- Critical: 95th percentile > 2 seconds
```

#### 1.2 Request Rate Metrics
```go
// Prometheus counter for HTTP request counts
icy_backend_http_requests_total{method, endpoint, status}

Tracks:
- Request volume by endpoint
- Traffic patterns
- Error rates

Alerting:
- Warning: Error rate > 5%
- Critical: Error rate > 20%
```

### 2. Business Logic Metrics (Instrumentation)

#### 2.1 Oracle Data Metrics
```go
// Oracle data freshness
icy_backend_oracle_data_age_seconds{data_type}

// Oracle calculation duration
icy_backend_oracle_calculation_duration_seconds{calculation_type}

// Oracle data accuracy (deviation from external sources)
icy_backend_oracle_deviation_percentage{data_type}

Tracks:
- Oracle data staleness
- Calculation performance
- Data accuracy vs external sources

Alerting:
- Warning: Data age > 300 seconds (5 minutes)
- Critical: Data age > 600 seconds (10 minutes)
- Critical: Deviation > 10% from market rates
```

#### 2.2 Swap Operation Metrics
```go
// Swap request processing duration
icy_backend_swap_processing_duration_seconds{operation, status}

// Swap success/failure rates
icy_backend_swap_operations_total{operation, status}

// Swap amounts and fees
icy_backend_swap_amount_btc{operation}
icy_backend_swap_fee_collected{fee_type}

Tracks:
- Swap processing performance
- Success/failure rates
- Revenue metrics (fees collected)
- Transaction volumes

Alerting:
- Warning: Swap failure rate > 5%
- Critical: Swap failure rate > 15%
- Critical: Swap processing time > 60 seconds
```

### 3. Background Job Metrics (Instrumentation)

#### 3.1 Cron Job Performance Metrics
```go
// Background job execution duration
icy_backend_background_job_duration_seconds{job_name, status}

// Background job execution frequency
icy_backend_background_job_runs_total{job_name, status}

// Job-specific metrics
icy_backend_btc_transactions_indexed_total
icy_backend_icy_transactions_indexed_total
icy_backend_swap_requests_processed_total

Tracks:
- Job execution times
- Success/failure rates
- Processing volumes

Alerting:
- Warning: Job duration > 120 seconds
- Critical: Job failure rate > 10%
- Critical: No job execution in > 15 minutes
```

#### 3.2 Queue and Processing Metrics
```go
// Pending transaction counts
icy_backend_pending_transactions_total{transaction_type}

// Processing lag metrics
icy_backend_processing_lag_seconds{job_type}

Tracks:
- Queue depths
- Processing delays
- System throughput

Alerting:
- Warning: Pending transactions > 10
- Critical: Pending transactions > 50
- Critical: Processing lag > 300 seconds
```

### 4. External API Metrics (Instrumentation)

#### 4.1 External API Call Metrics
```go
// External API call duration
icy_backend_external_api_duration_seconds{api_name, endpoint, status}

// External API success/failure rates
icy_backend_external_api_calls_total{api_name, status}

// Circuit breaker metrics
icy_backend_circuit_breaker_state{api_name}
icy_backend_circuit_breaker_trips_total{api_name}

Tracks:
- External API performance
- Reliability metrics
- Circuit breaker effectiveness

Alerting:
- Warning: API latency > 5 seconds
- Critical: API error rate > 20%
- Critical: Circuit breaker open for > 5 minutes
```

---

## Implementation Plan

### Phase 1: Foundation (Week 1)
**Synthetic Monitoring:**
- [ ] **Task 1**: Implement basic health endpoints (`/healthz`, `/health/db`, `/health/external`)
- [ ] **Task 2**: Add external API monitoring (Blockstream, Base RPC)

**Instrumentation Monitoring:**
- [ ] **Task 3**: Implement application-level metrics for business logic (oracle data, swap operations, transaction processing)
- [ ] **Task 4**: Add background job status tracking and health endpoint

### Phase 2: Advanced Monitoring (Week 2-3)
- [ ] Set up Prometheus metrics collection and alerting rules
- [ ] Implement oracle data validation synthetic tests
- [ ] Set up basic alerting notifications

### Phase 3: End-to-End Testing (Week 4-5)
- [ ] Develop complete transaction flow tests
- [ ] Implement error condition testing
- [ ] Create performance baseline measurements
- [ ] Full incident response procedures

### Phase 4: Production Deployment (Week 6)
- [ ] Deploy monitoring stack to production environment
- [ ] Fine-tune alerting thresholds based on production data
- [ ] Implement automated remediation for common issues
- [ ] Documentation and runbook creation

---

## Alerting Strategy

### Alert Severity Levels

**Critical (P0) - Immediate Response Required**
- API endpoints down (health check failing)
- Background jobs stopped (no runs in >15 minutes)
- External API complete failure (>50% error rate)
- Price oracle deviation >25% from market rates
- Database connectivity lost

**High (P1) - Response within 30 minutes**
- API response time >2 seconds sustained
- Background job failures >50% rate
- External API degraded (>20% error rate)
- Price oracle deviation >10% from market rates
- Pending transactions stuck >30 minutes

**Medium (P2) - Response within 2 hours**
- API response time >1 second sustained
- Background job duration increased >200% from baseline
- External API latency >5 seconds
- Transaction processing lag >10 minutes

**Low (P3) - Response within 24 hours**
- API response time >500ms sustained
- Minor deviations in oracle data (<5%)
- Non-critical endpoint issues

---

## Success Metrics and KPIs

### Availability Targets
- **API Endpoints**: 99.9% uptime
- **Background Jobs**: 99.5% success rate
- **End-to-End Swaps**: 99% completion rate within SLA

### Performance Targets
- **API Response Time**: 95th percentile < 500ms
- **Oracle Data Freshness**: < 2 minutes lag
- **Transaction Processing**: < 5 minutes end-to-end

### Quality Metrics
- **False Positive Rate**: < 5% of alerts
- **Mean Time to Detection (MTTD)**: < 2 minutes
- **Mean Time to Recovery (MTTR)**: < 15 minutes for P0 issues

---

## Technology Stack

### Monitoring Tools
- **Synthetic Monitoring**: Datadog Synthetics or New Relic Synthetics
- **Metrics Collection**: Prometheus
- **Visualization**: Grafana
- **Alerting**: AlertManager + PagerDuty
- **Incident Management**: PagerDuty + Slack

### Custom Implementation
- **Health Endpoints**: Go HTTP handlers
- **Metrics**: Prometheus client library
- **Middleware**: Gin middleware for HTTP metrics
- **Background Job Instrumentation**: Direct metric collection in cron jobs

---

**Document Version**: 2.0  
**Last Updated**: January 25, 2025  
**Next Review**: February 25, 2025  
**Owner**: DevOps Team  
**Stakeholders**: Backend Engineering, SRE, Product Management
