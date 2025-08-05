package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
)

// ExternalAPIMetrics contains all metrics for external API monitoring
type ExternalAPIMetrics struct {
	// API call duration histogram
	apiDuration *prometheus.HistogramVec
	
	// API call count counter
	apiCalls *prometheus.CounterVec
	
	// Circuit breaker state gauge
	circuitBreakerState *prometheus.GaugeVec
	
	// Timeout count counter
	timeouts *prometheus.CounterVec
}

// NewExternalAPIMetrics creates a new instance of external API metrics
func NewExternalAPIMetrics() *ExternalAPIMetrics {
	return &ExternalAPIMetrics{
		apiDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "icy_backend_external_api_duration_seconds",
				Help: "Duration of external API calls in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"api_name", "endpoint", "status"},
		),
		
		apiCalls: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "icy_backend_external_api_calls_total",
				Help: "Total number of external API calls",
			},
			[]string{"api_name", "status"},
		),
		
		circuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "icy_backend_circuit_breaker_state",
				Help: "Current state of circuit breakers (0=closed, 1=half-open, 2=open)",
			},
			[]string{"api_name"},
		),
		
		timeouts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "icy_backend_external_api_timeouts_total",
				Help: "Total number of external API timeouts",
			},
			[]string{"api_name", "timeout_type"},
		),
	}
}

// MustRegister registers all metrics with the provided registry
func (m *ExternalAPIMetrics) MustRegister(registry *prometheus.Registry) {
	registry.MustRegister(
		m.apiDuration,
		m.apiCalls,
		m.circuitBreakerState,
		m.timeouts,
	)
}

// RecordAPICall records an API call with duration and status
func (m *ExternalAPIMetrics) RecordAPICall(apiName, endpoint, status string, duration float64) {
	m.apiDuration.WithLabelValues(apiName, endpoint, status).Observe(duration)
	m.apiCalls.WithLabelValues(apiName, status).Inc()
}

// UpdateCircuitBreakerState updates the circuit breaker state metric
func (m *ExternalAPIMetrics) UpdateCircuitBreakerState(apiName string, state gobreaker.State) {
	m.circuitBreakerState.WithLabelValues(apiName).Set(float64(state))
}

// RecordTimeout records a timeout event
func (m *ExternalAPIMetrics) RecordTimeout(apiName, timeoutType string) {
	m.timeouts.WithLabelValues(apiName, timeoutType).Inc()
}