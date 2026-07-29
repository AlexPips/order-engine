package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the order engine.
type Metrics struct {
	OrdersReceived  prometheus.Counter
	OrdersSubmitted prometheus.Counter
	TradesExecuted  prometheus.Counter
	OrderLatency    prometheus.Histogram
	BookDepth       *prometheus.GaugeVec
	ActiveStreams   prometheus.Gauge
}

// NewMetrics registers and returns all metrics. Call once at startup.
func NewMetrics() *Metrics {
	return &Metrics{
		OrdersReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "orders_received_total",
			Help: "Total number of orders received by the service.",
		}),
		OrdersSubmitted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "orders_submitted_total",
			Help: "Total number of orders successfully submitted to the matching engine.",
		}),
		TradesExecuted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "trades_executed_total",
			Help: "Total number of trades executed by the matching engine.",
		}),
		OrderLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "order_submit_duration_seconds",
			Help:    "Time spent submitting an order to the matching engine.",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 20), // 1us to ~1s
		}),
		BookDepth: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "order_book_depth",
			Help: "Number of price levels in the order book per symbol.",
		}, []string{"symbol"}),
		ActiveStreams: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "active_trade_streams",
			Help: "Number of active TradeFeed streams.",
		}),
	}
}
