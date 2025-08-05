package monitoring

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics contains all metrics for HTTP request monitoring
type HTTPMetrics struct {
	// HTTP request duration histogram
	requestDuration *prometheus.HistogramVec
	
	// HTTP request count counter
	requestsTotal *prometheus.CounterVec
	
	// HTTP response size histogram
	responseSize *prometheus.HistogramVec
	
	// HTTP requests currently in flight
	inFlightRequests *prometheus.GaugeVec
	
	// Business logic metrics
	businessOperations *prometheus.CounterVec
	businessDuration   *prometheus.HistogramVec
	
	// Cache metrics
	cacheHitRate *prometheus.CounterVec
}

// NewHTTPMetrics creates a new instance of HTTP metrics
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "icy_backend_http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"method", "path", "status"},
		),
		
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "icy_backend_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "icy_backend_http_response_size_bytes",
				Help:    "Size of HTTP responses in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 2, 8), // 100B to 12KB
			},
			[]string{"method", "path", "status"},
		),
		
		inFlightRequests: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "icy_backend_http_requests_in_flight",
				Help: "Current number of HTTP requests being served",
			},
			[]string{"method", "path"},
		),
		
		businessOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "icy_backend_business_operations_total",
				Help: "Total number of business operations",
			},
			[]string{"operation_type", "category", "status"},
		),
		
		businessDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "icy_backend_business_operation_duration_seconds",
				Help:    "Duration of business operations in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0},
			},
			[]string{"operation_type", "category", "status"},
		),
		
		cacheHitRate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "icy_backend_cache_operations_total",
				Help: "Total number of cache operations",
			},
			[]string{"cache_type", "operation"}, // operation: hit, miss
		),
	}
}

// MustRegister registers all HTTP metrics with the provided registry
func (m *HTTPMetrics) MustRegister(registry *prometheus.Registry) {
	registry.MustRegister(
		m.requestDuration,
		m.requestsTotal,
		m.responseSize,
		m.inFlightRequests,
		m.businessOperations,
		m.businessDuration,
		m.cacheHitRate,
	)
}

// RecordBusinessMetric records a business operation metric
func (m *HTTPMetrics) RecordBusinessMetric(operationType, category, status string, duration float64) {
	m.businessOperations.WithLabelValues(operationType, category, status).Inc()
	if duration > 0 {
		m.businessDuration.WithLabelValues(operationType, category, status).Observe(duration)
	}
}

// HTTPMetricsMiddleware creates a Gin middleware for HTTP metrics collection
func HTTPMetricsMiddleware(metrics *HTTPMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		method := c.Request.Method
		
		// Handle cases where FullPath might be empty (404s, etc.)
		if path == "" {
			path = c.Request.URL.Path
		}
		
		// Increment in-flight requests
		metrics.inFlightRequests.WithLabelValues(method, path).Inc()
		
		// Process request
		c.Next()
		
		// Calculate response metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		responseSize := float64(c.Writer.Size())
		
		// Record metrics
		metrics.requestDuration.WithLabelValues(method, path, status).Observe(duration)
		metrics.requestsTotal.WithLabelValues(method, path, status).Inc()
		if responseSize > 0 {
			metrics.responseSize.WithLabelValues(method, path, status).Observe(responseSize)
		}
		
		// Decrement in-flight requests
		metrics.inFlightRequests.WithLabelValues(method, path).Dec()
	}
}

// BusinessMetricsRecorder provides methods to record business logic metrics
type BusinessMetricsRecorder struct {
	metrics *HTTPMetrics
}

// NewBusinessMetricsRecorder creates a new business metrics recorder
func NewBusinessMetricsRecorder(metrics *HTTPMetrics) *BusinessMetricsRecorder {
	return &BusinessMetricsRecorder{
		metrics: metrics,
	}
}

// RecordSwapRequest records a swap request operation
func (r *BusinessMetricsRecorder) RecordSwapRequest(swapType, status string, duration float64) {
	r.metrics.RecordBusinessMetric("swap_request", swapType, status, duration)
}

// RecordSwapOperation records a swap operation (generic swap operations)
func (r *BusinessMetricsRecorder) RecordSwapOperation(operationType, status string, duration float64) {
	r.metrics.RecordBusinessMetric("swap_operation", operationType, status, duration)
}

// RecordOracleOperation records an oracle operation
func (r *BusinessMetricsRecorder) RecordOracleOperation(operationType, status string, duration float64) {
	r.metrics.RecordBusinessMetric("oracle_operation", operationType, status, duration)
}

// RecordTransactionIndexing records a transaction indexing operation
func (r *BusinessMetricsRecorder) RecordTransactionIndexing(chain, status string, duration float64) {
	r.metrics.RecordBusinessMetric("transaction_indexing", chain, status, duration)
}

// RecordDatabaseOperation records a database operation
func (r *BusinessMetricsRecorder) RecordDatabaseOperation(operationType, status string, duration float64) {
	r.metrics.RecordBusinessMetric("database_operation", operationType, status, duration)
}

// RecordCacheOperation records a cache hit or miss
func (r *BusinessMetricsRecorder) RecordCacheOperation(cacheType, operation string) {
	r.metrics.cacheHitRate.WithLabelValues(cacheType, operation).Inc()
}