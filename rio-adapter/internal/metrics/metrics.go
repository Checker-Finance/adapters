package metrics

import (
	"time"

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

	// Tracks NATS messages processed by subject and result.
	NATSMessageCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_messages_total",
			Help: "Total number of NATS messages processed.",
		},
		[]string{"subject", "result"}, // result = "ok" | "error"
	)

	NATSMessageLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nats_message_latency_seconds",
			Help:    "Time taken to publish NATS messages",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"subject"},
	)

	// Tracks cache hits and misses for secrets / credentials.
	SecretsCacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "secrets_cache_access_total",
			Help: "Number of cache hits/misses in secret cache.",
		},
		[]string{"result"}, // hit | miss
	)

	// Tracks total errors (aggregated).
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adapter_errors_total",
			Help: "Count of adapter-level errors by component.",
		},
		[]string{"component", "reason"},
	)

	// Gauges the last successful poll time (seconds since epoch).
	LastPollTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "adapter_last_poll_timestamp",
			Help: "Timestamp (unix seconds) of the last successful balance or quote poll.",
		},
		[]string{"component"},
	)
)

// ObserveDuration records the time taken for a function and updates the given histogram.
func ObserveDuration(v interface{}, start time.Time, labels ...string) {
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

func IncRioRequest(endpoint, method, status string) {
	RioRequestsTotal.WithLabelValues(endpoint, method, status).Inc()
}

func IncNATSMessage(subject, result string) {
	NATSMessageCount.WithLabelValues(subject, result).Inc()
}

func IncCacheHit(result string) {
	SecretsCacheHits.WithLabelValues(result).Inc()
}

func IncError(component, reason string) {
	ErrorsTotal.WithLabelValues(component, reason).Inc()
}

func SetLastPoll(component string, t time.Time) {
	LastPollTimestamp.WithLabelValues(component).Set(float64(t.Unix()))
}

