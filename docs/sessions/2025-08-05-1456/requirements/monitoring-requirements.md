# Monitoring System Implementation Requirements

## Project Overview

Implement a comprehensive monitoring system for the ICY Backend cryptocurrency swap system with both synthetic (external black-box) and instrumentation (internal white-box) monitoring capabilities.

## High-Level Requirements

### Primary Objectives
1. **Synthetic Monitoring**: External health checks and business logic validation
2. **Instrumentation Monitoring**: Internal metrics collection for performance and business insights
3. **Alerting System**: Multi-level alerting based on severity and response time requirements
4. **Observability**: Complete visibility into system health, performance, and business metrics

### Scope of Implementation

#### Phase 1: Foundation (Current Focus)
**Task 1: Basic Health Endpoints (Synthetic)**
- Implement `/healthz` - Basic system availability
- Implement `/api/v1/health/db` - Database connectivity check  
- Implement `/api/v1/health/external` - External dependencies health check

**Task 2: External API Monitoring (Synthetic)**
- Monitor Blockstream API (Bitcoin blockchain)
- Monitor Base Chain RPC (Ethereum/ICY token network)
- Integration with health endpoints

**Task 3: Application-Level Metrics (Instrumentation)**
- HTTP request metrics (duration, rates, errors)
- Business logic metrics (Oracle data, Swap operations)
- External API call metrics

**Task 4: Background Job Monitoring (Instrumentation)**
- Cron job performance metrics
- Queue and processing metrics
- Job status tracking endpoint

## Functional Requirements

### 1. Health Check Endpoints

#### 1.1 Basic Health Endpoint
- **Endpoint**: `GET /healthz`
- **Purpose**: Basic system availability check
- **Response**: `{"message": "ok"}`
- **SLA**: < 200ms response time, 99.9% availability
- **No authentication required**

#### 1.2 Database Health Check
- **Endpoint**: `GET /api/v1/health/db`
- **Purpose**: Database connectivity and performance validation
- **Response**: `{"status": "healthy|unhealthy", "latency_ms": 15, "error": "optional"}`
- **SLA**: < 500ms response time, 99.5% availability
- **Timeout**: 5 seconds for database operations

#### 1.3 External Dependencies Health Check
- **Endpoint**: `GET /api/v1/health/external`
- **Purpose**: External API connectivity and performance validation
- **Response**: Complex JSON with service-specific health information
- **SLA**: < 2000ms response time, 95% availability
- **Timeout**: 10 seconds for external API calls
- **Services Monitored**:
  - Blockstream API (Bitcoin blockchain)
  - Base Chain RPC (Ethereum/ICY token network)

### 2. Metrics Collection (Prometheus)

#### 2.1 HTTP Request Metrics
- **Duration**: `icy_backend_http_request_duration_seconds{method, endpoint, status}`
- **Count**: `icy_backend_http_requests_total{method, endpoint, status}`
- **Middleware**: Gin middleware for automatic collection

#### 2.2 Business Logic Metrics
- **Oracle Data Age**: `icy_backend_oracle_data_age_seconds{data_type}`
- **Oracle Calculation Duration**: `icy_backend_oracle_calculation_duration_seconds{calculation_type}`
- **Swap Processing**: `icy_backend_swap_processing_duration_seconds{operation, status}`
- **Swap Counts**: `icy_backend_swap_operations_total{operation, status}`

#### 2.3 Background Job Metrics
- **Job Duration**: `icy_backend_background_job_duration_seconds{job_name, status}`
- **Job Counts**: `icy_backend_background_job_runs_total{job_name, status}`
- **Pending Transactions**: `icy_backend_pending_transactions_total{transaction_type}`

#### 2.4 External API Metrics
- **API Duration**: `icy_backend_external_api_duration_seconds{api_name, endpoint, status}`
- **API Calls**: `icy_backend_external_api_calls_total{api_name, status}`
- **Circuit Breaker**: `icy_backend_circuit_breaker_state{api_name}`

### 3. Background Job Status Tracking

#### 3.1 Job Status Endpoint
- **Endpoint**: `GET /api/v1/health/jobs`
- **Purpose**: Report background job health and status
- **Response**: Job-specific status information including last run, duration, success/failure counts

#### 3.2 Job Status Storage
- In-memory status tracking for job execution
- Thread-safe access to job metrics
- Periodic cleanup of old status records

## Non-Functional Requirements

### Performance Requirements
- Health endpoints must respond within specified SLA times
- Metrics collection must have minimal performance impact (<1ms overhead per request)
- Background job status tracking must not affect job execution performance

### Availability Requirements
- Health endpoints: 99.9% availability
- Metrics collection: 99.99% reliability (should not fail requests)
- Background job monitoring: Must survive job failures gracefully

### Security Requirements
- Health endpoints accessible without authentication (except where noted)
- Metrics endpoint (`/metrics`) should be accessible for Prometheus scraping
- No sensitive data exposed in health check responses
- Proper error handling to prevent information disclosure

### Scalability Requirements
- Metrics collection must scale with request volume
- Memory usage for metrics must be bounded and predictable
- Background job status tracking must handle multiple concurrent jobs

## Technical Constraints

### Existing System Integration
- Must integrate with existing Gin HTTP framework
- Must work with current GORM database layer
- Must not interfere with existing API routes
- Must integrate with existing cron job system

### Technology Stack
- **Language**: Go
- **HTTP Framework**: Gin
- **Database**: PostgreSQL with GORM
- **Metrics**: Prometheus client library
- **Background Jobs**: robfig/cron/v3

### Dependencies
- Prometheus Go client library
- Existing btcrpc and baserpc interfaces
- Existing store interfaces
- Existing logger implementation

## Success Criteria

### Functional Success Criteria
1. All health endpoints respond correctly and within SLA
2. Metrics are collected and exposed on `/metrics` endpoint
3. Background job status is tracked and reportable
4. External API monitoring provides accurate health status
5. Integration with existing system without breaking changes

### Performance Success Criteria
1. Health endpoints meet response time SLA requirements
2. Metrics collection adds <1ms overhead per HTTP request
3. Memory usage for metrics remains stable under load
4. Background job monitoring doesn't affect job execution time

### Quality Success Criteria
1. All functionality covered by comprehensive tests
2. Code follows existing project conventions
3. Proper error handling and logging
4. Documentation for all new endpoints and metrics
5. Zero security vulnerabilities introduced

## Out of Scope

### Not Included in Phase 1
- Grafana dashboard setup
- AlertManager configuration
- External synthetic testing setup (Datadog/New Relic)
- Complete end-to-end transaction flow testing
- Production deployment configuration

### Future Phases
- Advanced alerting rules
- Custom dashboards
- External monitoring service integration
- Performance optimization based on production data
- Automated remediation capabilities

## Assumptions

1. Existing btcrpc and baserpc interfaces provide necessary methods for health checking
2. Database store interface provides a Ping() method or equivalent
3. Existing logger can be used for monitoring-related logging
4. Prometheus metrics will be scraped by external Prometheus server
5. Health endpoints will be called by external monitoring systems
6. Background jobs are executed via the existing cron system

## Dependencies and Prerequisites

### Code Dependencies
- Review existing interfaces: btcrpc.Interface, baserpc.Interface, store.Store
- Understand existing handler structure and routing
- Review existing middleware patterns
- Understand existing cron job implementation

### Infrastructure Dependencies
- Prometheus server for metrics collection (external)
- Monitoring system for health check validation (external)
- Network access for external API health checks

## Risk Assessment

### Technical Risks
- **Performance Impact**: Metrics collection could affect request latency
- **Memory Usage**: Unbounded metrics could cause memory issues
- **External Dependencies**: Health checks depend on external API availability
- **Concurrency**: Background job status tracking needs thread safety

### Mitigation Strategies
- Use efficient Prometheus metrics with proper labeling
- Implement bounded metrics with appropriate cleanup
- Implement timeouts and circuit breaker patterns for external calls
- Use thread-safe data structures for job status tracking

## Acceptance Criteria

### Phase 1 Completion Criteria
1. **Health Endpoints**:
   - `/healthz` returns 200 OK with correct JSON response
   - `/api/v1/health/db` validates database connectivity
   - `/api/v1/health/external` checks external APIs with proper status logic

2. **Metrics Collection**:
   - HTTP requests are instrumented with duration and count metrics
   - Business logic metrics are collected for oracle and swap operations
   - External API calls are instrumented
   - Background jobs report execution metrics

3. **Integration**:
   - All endpoints integrated into existing route structure
   - Metrics middleware applied to relevant routes
   - Background job monitoring integrated with existing cron jobs
   - `/metrics` endpoint exposes all collected metrics

4. **Testing**:
   - Unit tests for all health handlers
   - Integration tests for metrics collection
   - Test coverage >80% for new code
   - All tests pass in CI/CD pipeline

5. **Documentation**:
   - API documentation for new endpoints
   - Metrics documentation with labels and descriptions
   - Integration guide for operations team
   - Runbook for common monitoring scenarios