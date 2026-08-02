package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP request metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of HTTP requests being processed.",
		},
	)
)

// MetricsMiddleware returns middleware that records Prometheus metrics for each request.
// Tracks request count by method/path/status, request duration, and in-flight count.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := normalizePath(c.FullPath(), c.Request.URL.Path)

		// Track in-flight requests
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		// Process request
		c.Next()

		// Record metrics
		status := strconv.Itoa(c.Writer.Status())
		duration := time.Since(start).Seconds()
		method := c.Request.Method

		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}

// MetricsHandler returns an http.Handler for the /metrics endpoint.
// Register it with: r.GET("/metrics", gin.WrapH(handler.MetricsHandler()))
func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// normalizePath returns the route pattern (e.g., "/api/repos/:id") instead of the
// actual path (e.g., "/api/repos/abc-123") to keep metric cardinality bounded.
func normalizePath(fullPath, actualPath string) string {
	if fullPath != "" {
		return fullPath
	}
	return actualPath
}
