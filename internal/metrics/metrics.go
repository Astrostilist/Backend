package metrics

import (
	"astroapi/config"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry *prometheus.Registry

	// Counter metrics
	dlqMessagesTotal *prometheus.CounterVec

	// Histogram metrics
	httpRequestDuration *prometheus.HistogramVec
	// Gauge metrics
	natsConsumerLag     *prometheus.GaugeVec
	circuitBreakerState *prometheus.GaugeVec

	initOnce sync.Once
)

func Initialize(cfg *config.Config) {
	initOnce.Do(func() {
		initializeMetrics(cfg)
	})
}

func initializeMetrics(cfg *config.Config) {
	registry = prometheus.NewRegistry()

	// init Global lables
	staticLabels := prometheus.Labels{
		"environment": cfg.Environment,
		"instance":    cfg.LogServiceName,
	}

	// init Counter metrics
	rawDlqMessagesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dlq_messages_total",
			Help: "DLQ messages total",
		},
		[]string{"instance", "environment"})
	dlqMessagesTotal = rawDlqMessagesTotal.MustCurryWith(staticLabels)

	// init Histogram metrics
	rawHTTPRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Time spent processing http request",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"instance", "environment", "method", "endpoint", "status"},
	)
	httpRequestDuration = rawHTTPRequestDuration.MustCurryWith(staticLabels).(*prometheus.HistogramVec)

	// init Gauge metrics
	rawNatsConsumerLag := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nats_consumer_lag",
			Help: "NATS consumer lag",
		}, []string{"instance", "environment", "stream", "consumer"})
	natsConsumerLag = rawNatsConsumerLag.MustCurryWith(staticLabels)

	// 0=closed, 1=open, 2=half-open
	rawCircuitBreakerState := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit Breaker state",
		}, []string{"instance", "environment", "service"})

	circuitBreakerState = rawCircuitBreakerState.MustCurryWith(staticLabels)

	// metrics register
	registry.MustRegister(rawDlqMessagesTotal)
	registry.MustRegister(rawHTTPRequestDuration)
	registry.MustRegister(rawNatsConsumerLag)
	registry.MustRegister(rawCircuitBreakerState)

}

func NewHandler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

func IncdlqMessagesTotal() {
	dlqMessagesTotal.WithLabelValues().Inc()
}

func ObserveMessageProcessingDuration(method, endpoint, status string, seconds float64) {
	httpRequestDuration.WithLabelValues(method, endpoint, status).Observe(seconds)
}

func SetNatsConsumerLag(stream, consumer string, lag float64) {
	natsConsumerLag.WithLabelValues(stream, consumer).Set(lag)
}

func SetCircuitBreakerState(service string, state int) {
	circuitBreakerState.WithLabelValues(service).Set(float64(state))
}
