package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// XFXRequestsTotal tracks the number of outbound API calls to XFX.
	XFXRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "xfx_api_requests_total",
			Help: "Total number of XFX API requests made (by endpoint, method, and status).",
		},
		[]string{"endpoint", "method", "status"},
	)

	// XFXRequestDuration measures the duration of outbound XFX API calls.
	XFXRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "xfx_api_request_duration_seconds",
			Help:    "Duration of XFX API requests in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms → ~16s
		},
		[]string{"endpoint", "method"},
	)

	// NATSPublishErrors tracks NATS publish failures by subject.
	NATSPublishErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_publish_errors_total",
			Help: "Number of NATS publish failures by subject.",
		},
		[]string{"subject"},
	)
)

// IncXFXRequest increments the XFX API request counter.
func IncXFXRequest(endpoint, method, status string) {
	XFXRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
}

// ObserveDuration records elapsed time since start into a HistogramVec or SummaryVec.
func ObserveDuration(v any, start time.Time, labels ...string) {
	duration := time.Since(start).Seconds()
	switch metric := v.(type) {
	case *prometheus.HistogramVec:
		metric.WithLabelValues(labels...).Observe(duration)
	case *prometheus.SummaryVec:
		metric.WithLabelValues(labels...).Observe(duration)
	}
}

// IncNATSPublishError increments the NATS publish error counter for the given subject.
func IncNATSPublishError(subject string) {
	NATSPublishErrors.WithLabelValues(subject).Inc()
}
