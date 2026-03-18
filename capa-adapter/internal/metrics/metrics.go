package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CapaRequestsTotal tracks the number of outbound API calls to Capa.
	CapaRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capa_api_requests_total",
			Help: "Total number of Capa API requests made (by endpoint, method, and status).",
		},
		[]string{"endpoint", "method", "status"},
	)

	// CapaRequestDuration measures the duration of outbound Capa API calls.
	CapaRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "capa_api_request_duration_seconds",
			Help:    "Duration of Capa API requests in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms → ~16s
		},
		[]string{"endpoint", "method"},
	)

	// NATSPublishErrors tracks NATS publish failures by subject.
	NATSPublishErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capa_nats_publish_errors_total",
			Help: "Number of NATS publish failures by subject.",
		},
		[]string{"subject"},
	)
)

// IncCapaRequest increments the Capa API request counter.
func IncCapaRequest(endpoint, method, status string) {
	CapaRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
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
