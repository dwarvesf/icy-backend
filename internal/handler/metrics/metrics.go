package metrics

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler handles Prometheus metrics endpoint
type MetricsHandler struct {
	registry *prometheus.Registry
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(registry *prometheus.Registry) *MetricsHandler {
	return &MetricsHandler{
		registry: registry,
	}
}

// Handler returns a Gin handler function for the /metrics endpoint
func (h *MetricsHandler) Handler() gin.HandlerFunc {
	handler := promhttp.HandlerFor(h.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	
	return gin.WrapH(handler)
}