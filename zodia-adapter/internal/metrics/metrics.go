package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ZodiaRequestsTotal tracks the number of outbound API calls to Zodia.
	ZodiaRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zodia_api_requests_total",
			Help: "Total number of Zodia API requests made (by endpoint, method, and status).",
		},
		[]string{"endpoint", "method", "status"},
	)

	// ZodiaRequestDuration measures the duration of outbound Zodia API calls.
	ZodiaRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zodia_api_request_duration_seconds",
			Help:    "Duration of Zodia API requests in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms → ~16s
		},
		[]string{"endpoint", "method"},
	)

	// ZodiaWSReconnectsTotal tracks WebSocket reconnect events per client.
	ZodiaWSReconnectsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zodia_ws_reconnects_total",
			Help: "Number of WebSocket reconnect events by client ID.",
		},
		[]string{"client_id"},
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

// IncZodiaRequest increments the Zodia API request counter.
func IncZodiaRequest(endpoint, method, status string) {
	ZodiaRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
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

// IncWSReconnect increments the WebSocket reconnect counter for a client.
func IncWSReconnect(clientID string) {
	ZodiaWSReconnectsTotal.WithLabelValues(clientID).Inc()
}
