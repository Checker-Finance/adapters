package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Tracks the number of outbound API calls to Rio.
	RioRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rio_api_requests_total",
			Help: "Total number of Rio API requests made (by endpoint and method).",
		},
		[]string{"endpoint", "method", "status"},
	)

	// Measures duration of API requests to Rio.
	RioRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rio_api_request_duration_seconds",
			Help:    "Duration of Rio API requests in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms → ~16s
		},
		[]string{"endpoint", "method"},
	)
)

func IncRioRequest(endpoint, method, status string) {
	RioRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
}
