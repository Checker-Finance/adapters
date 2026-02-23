package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Tracks the number of outbound API calls to Braza.
	BrazaRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "braza_api_requests_total",
			Help: "Total number of Braza API requests made (by endpoint and method).",
		},
		[]string{"endpoint", "method", "status"},
	)

	// Measures duration of API requests to Braza.
	BrazaRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "braza_api_request_duration_seconds",
			Help:    "Duration of Braza API requests in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms → ~16s
		},
		[]string{"endpoint", "method"},
	)

	NATSPublishErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_publish_errors_total",
			Help: "Number of NATS publish failures",
		},
		[]string{"subject"},
	)
)

// ObserveDuration records the time taken for a function and updates the given histogram.
func ObserveDuration(v any, start time.Time, labels ...string) {
	duration := time.Since(start).Seconds()

	switch metric := v.(type) {
	case *prometheus.HistogramVec:
		metric.WithLabelValues(labels...).Observe(duration)
	case *prometheus.SummaryVec:
		metric.WithLabelValues(labels...).Observe(duration)
	default:
		// silently ignore counters; they're not meant for duration tracking
	}
}

func IncBrazaRequest(endpoint, method, status string) {
	BrazaRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
}

func StartServer(addr string) {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(addr, nil) //nolint:errcheck
	}()
}
