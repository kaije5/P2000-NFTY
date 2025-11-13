package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the application
type Metrics struct {
	MessagesReceived    prometheus.Counter
	MessagesFiltered    prometheus.Counter
	NotificationsSent   prometheus.Counter
	NotificationsFailed prometheus.Counter
	NotificationDuration prometheus.Histogram
	WebsocketConnected  prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		MessagesReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "p2000_messages_received_total",
			Help: "Total number of P2000 messages received from WebSocket",
		}),
		MessagesFiltered: promauto.NewCounter(prometheus.CounterOpts{
			Name: "p2000_messages_filtered_total",
			Help: "Total number of P2000 messages that matched capcode filters",
		}),
		NotificationsSent: promauto.NewCounter(prometheus.CounterOpts{
			Name: "p2000_notifications_sent_total",
			Help: "Total number of notifications successfully sent to ntfy",
		}),
		NotificationsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "p2000_notifications_failed_total",
			Help: "Total number of notifications that failed to send",
		}),
		NotificationDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "p2000_notification_duration_seconds",
			Help:    "Duration of notification sending in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		WebsocketConnected: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "p2000_websocket_connected",
			Help: "WebSocket connection status (1 = connected, 0 = disconnected)",
		}),
	}
}

// RecordMessageReceived increments the messages received counter
func (m *Metrics) RecordMessageReceived() {
	m.MessagesReceived.Inc()
}

// RecordMessageFiltered increments the filtered messages counter
func (m *Metrics) RecordMessageFiltered() {
	m.MessagesFiltered.Inc()
}

// RecordNotificationSent increments the sent notifications counter
func (m *Metrics) RecordNotificationSent() {
	m.NotificationsSent.Inc()
}

// RecordNotificationFailed increments the failed notifications counter
func (m *Metrics) RecordNotificationFailed() {
	m.NotificationsFailed.Inc()
}

// SetWebsocketConnected sets the WebSocket connection status
func (m *Metrics) SetWebsocketConnected(connected bool) {
	if connected {
		m.WebsocketConnected.Set(1)
	} else {
		m.WebsocketConnected.Set(0)
	}
}
