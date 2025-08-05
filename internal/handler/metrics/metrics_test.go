package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestMetricsHandler_Handler(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	registry := prometheus.NewRegistry()
	
	// Register a test metric
	testCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "A test counter for metrics endpoint testing",
	})
	registry.MustRegister(testCounter)
	testCounter.Inc()
	
	handler := NewMetricsHandler(registry)
	
	router := gin.New()
	router.GET("/metrics", handler.Handler())

	// Act
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	
	responseBody := w.Body.String()
	
	// Check that the response contains Prometheus metrics format
	assert.Contains(t, responseBody, "# HELP test_counter A test counter for metrics endpoint testing")
	assert.Contains(t, responseBody, "# TYPE test_counter counter")
	assert.Contains(t, responseBody, "test_counter 1")
	
	// Check content type
	contentType := w.Header().Get("Content-Type")
	assert.True(t, 
		strings.Contains(contentType, "text/plain") || 
		strings.Contains(contentType, "application/openmetrics-text"), 
		"Expected Prometheus metrics content type, got: %s", contentType)
}

func TestMetricsHandler_EmptyRegistry(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	registry := prometheus.NewRegistry()
	handler := NewMetricsHandler(registry)
	
	router := gin.New()
	router.GET("/metrics", handler.Handler())

	// Act
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Even with empty registry, should return valid response (might be empty but valid)
	// Just verify we get a 200 OK response
	contentType := w.Header().Get("Content-Type")
	assert.True(t, 
		strings.Contains(contentType, "text/plain") || 
		strings.Contains(contentType, "application/openmetrics-text"), 
		"Expected Prometheus metrics content type, got: %s", contentType)
}

func TestMetricsHandler_MultipleMetrics(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	registry := prometheus.NewRegistry()
	
	// Register multiple test metrics
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_requests_total",
			Help: "Total test requests",
		},
		[]string{"method", "status"},
	)
	
	histogram := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "test_duration_seconds",
			Help: "Test duration in seconds",
		},
	)
	
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_active_connections",
			Help: "Number of active test connections",
		},
	)
	
	registry.MustRegister(counter, histogram, gauge)
	
	// Set some values
	counter.WithLabelValues("GET", "200").Inc()
	counter.WithLabelValues("POST", "201").Add(5)
	histogram.Observe(0.5)
	gauge.Set(42)
	
	handler := NewMetricsHandler(registry)
	
	router := gin.New()
	router.GET("/metrics", handler.Handler())

	// Act
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	
	responseBody := w.Body.String()
	
	// Check counter metrics
	assert.Contains(t, responseBody, "test_requests_total{method=\"GET\",status=\"200\"} 1")
	assert.Contains(t, responseBody, "test_requests_total{method=\"POST\",status=\"201\"} 5")
	
	// Check histogram metrics
	assert.Contains(t, responseBody, "test_duration_seconds_sum")
	assert.Contains(t, responseBody, "test_duration_seconds_count")
	
	// Check gauge metrics
	assert.Contains(t, responseBody, "test_active_connections 42")
}