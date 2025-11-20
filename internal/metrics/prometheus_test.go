package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	require.NotNil(t, m)
	assert.NotNil(t, m.MessagesReceived)
	assert.NotNil(t, m.MessagesFiltered)
	assert.NotNil(t, m.NotificationsSent)
	assert.NotNil(t, m.NotificationsFailed)
	assert.NotNil(t, m.NotificationDuration)
	assert.NotNil(t, m.WebsocketConnected)
}

func TestRecordMessageReceived(t *testing.T) {
	// Create a new registry to isolate this test
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_messages_received_total",
		Help: "Test counter",
	})
	registry.MustRegister(counter)

	m := &Metrics{
		MessagesReceived: counter,
	}

	// Initial value should be 0
	initialValue := testutil.ToFloat64(m.MessagesReceived)
	assert.Equal(t, 0.0, initialValue)

	// Record one message
	m.RecordMessageReceived()
	value := testutil.ToFloat64(m.MessagesReceived)
	assert.Equal(t, 1.0, value)

	// Record multiple messages
	m.RecordMessageReceived()
	m.RecordMessageReceived()
	m.RecordMessageReceived()
	value = testutil.ToFloat64(m.MessagesReceived)
	assert.Equal(t, 4.0, value)
}

func TestRecordMessageFiltered(t *testing.T) {
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_messages_filtered_total",
		Help: "Test counter",
	})
	registry.MustRegister(counter)

	m := &Metrics{
		MessagesFiltered: counter,
	}

	initialValue := testutil.ToFloat64(m.MessagesFiltered)
	assert.Equal(t, 0.0, initialValue)

	m.RecordMessageFiltered()
	value := testutil.ToFloat64(m.MessagesFiltered)
	assert.Equal(t, 1.0, value)

	for i := 0; i < 10; i++ {
		m.RecordMessageFiltered()
	}
	value = testutil.ToFloat64(m.MessagesFiltered)
	assert.Equal(t, 11.0, value)
}

func TestRecordNotificationSent(t *testing.T) {
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_notifications_sent_total",
		Help: "Test counter",
	})
	registry.MustRegister(counter)

	m := &Metrics{
		NotificationsSent: counter,
	}

	initialValue := testutil.ToFloat64(m.NotificationsSent)
	assert.Equal(t, 0.0, initialValue)

	m.RecordNotificationSent()
	value := testutil.ToFloat64(m.NotificationsSent)
	assert.Equal(t, 1.0, value)

	for i := 0; i < 5; i++ {
		m.RecordNotificationSent()
	}
	value = testutil.ToFloat64(m.NotificationsSent)
	assert.Equal(t, 6.0, value)
}

func TestRecordNotificationFailed(t *testing.T) {
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_notifications_failed_total",
		Help: "Test counter",
	})
	registry.MustRegister(counter)

	m := &Metrics{
		NotificationsFailed: counter,
	}

	initialValue := testutil.ToFloat64(m.NotificationsFailed)
	assert.Equal(t, 0.0, initialValue)

	m.RecordNotificationFailed()
	value := testutil.ToFloat64(m.NotificationsFailed)
	assert.Equal(t, 1.0, value)

	for i := 0; i < 3; i++ {
		m.RecordNotificationFailed()
	}
	value = testutil.ToFloat64(m.NotificationsFailed)
	assert.Equal(t, 4.0, value)
}

func TestSetWebsocketConnected(t *testing.T) {
	registry := prometheus.NewRegistry()

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_websocket_connected",
		Help: "Test gauge",
	})
	registry.MustRegister(gauge)

	m := &Metrics{
		WebsocketConnected: gauge,
	}

	// Initial value should be 0
	initialValue := testutil.ToFloat64(m.WebsocketConnected)
	assert.Equal(t, 0.0, initialValue)

	// Set to connected (true)
	m.SetWebsocketConnected(true)
	value := testutil.ToFloat64(m.WebsocketConnected)
	assert.Equal(t, 1.0, value)

	// Set to disconnected (false)
	m.SetWebsocketConnected(false)
	value = testutil.ToFloat64(m.WebsocketConnected)
	assert.Equal(t, 0.0, value)

	// Toggle multiple times
	m.SetWebsocketConnected(true)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.WebsocketConnected))

	m.SetWebsocketConnected(true)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.WebsocketConnected))

	m.SetWebsocketConnected(false)
	assert.Equal(t, 0.0, testutil.ToFloat64(m.WebsocketConnected))
}

func TestNotificationDurationHistogram(t *testing.T) {
	registry := prometheus.NewRegistry()

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_notification_duration_seconds",
		Help:    "Test histogram",
		Buckets: prometheus.DefBuckets,
	})
	registry.MustRegister(histogram)

	m := &Metrics{
		NotificationDuration: histogram,
	}

	// Record some durations
	m.NotificationDuration.Observe(0.1)
	m.NotificationDuration.Observe(0.5)
	m.NotificationDuration.Observe(1.0)
	m.NotificationDuration.Observe(2.5)

	// Verify histogram was registered and observations were recorded
	// We can't easily test histogram values with testutil.ToFloat64
	// Just verify no panics occurred
	assert.NotNil(t, m.NotificationDuration)
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	m := NewMetrics()

	// Run concurrent operations
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(id int) {
			m.RecordMessageReceived()
			m.RecordMessageFiltered()
			m.RecordNotificationSent()
			m.RecordNotificationFailed()
			m.SetWebsocketConnected(id%2 == 0)
			m.NotificationDuration.Observe(0.5)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify counters incremented correctly
	messagesReceived := testutil.ToFloat64(m.MessagesReceived)
	assert.Equal(t, 100.0, messagesReceived)

	messagesFiltered := testutil.ToFloat64(m.MessagesFiltered)
	assert.Equal(t, 100.0, messagesFiltered)

	notificationsSent := testutil.ToFloat64(m.NotificationsSent)
	assert.Equal(t, 100.0, notificationsSent)

	notificationsFailed := testutil.ToFloat64(m.NotificationsFailed)
	assert.Equal(t, 100.0, notificationsFailed)

	// Gauge should be either 0 or 1
	connected := testutil.ToFloat64(m.WebsocketConnected)
	assert.True(t, connected == 0.0 || connected == 1.0)

	// Histogram observations were made (can't easily test count)
	assert.NotNil(t, m.NotificationDuration)
}

func TestMetrics_Workflow(t *testing.T) {
	m := NewMetrics()

	// Simulate a complete workflow

	// 1. WebSocket connects
	m.SetWebsocketConnected(true)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.WebsocketConnected))

	// 2. Receive 10 messages
	for i := 0; i < 10; i++ {
		m.RecordMessageReceived()
	}
	assert.Equal(t, 10.0, testutil.ToFloat64(m.MessagesReceived))

	// 3. Filter matches 7 messages
	for i := 0; i < 7; i++ {
		m.RecordMessageFiltered()
	}
	assert.Equal(t, 7.0, testutil.ToFloat64(m.MessagesFiltered))

	// 4. Send 6 notifications successfully
	for i := 0; i < 6; i++ {
		m.RecordNotificationSent()
		m.NotificationDuration.Observe(0.5)
	}
	assert.Equal(t, 6.0, testutil.ToFloat64(m.NotificationsSent))

	// 5. One notification fails
	m.RecordNotificationFailed()
	assert.Equal(t, 1.0, testutil.ToFloat64(m.NotificationsFailed))

	// 6. WebSocket disconnects
	m.SetWebsocketConnected(false)
	assert.Equal(t, 0.0, testutil.ToFloat64(m.WebsocketConnected))

	// 7. WebSocket reconnects
	m.SetWebsocketConnected(true)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.WebsocketConnected))
}

func TestMetrics_LargeVolume(t *testing.T) {
	m := NewMetrics()

	// Simulate high volume of messages
	for i := 0; i < 10000; i++ {
		m.RecordMessageReceived()
	}

	value := testutil.ToFloat64(m.MessagesReceived)
	assert.Equal(t, 10000.0, value)
}

func TestMetrics_MultipleNotificationDurations(t *testing.T) {
	registry := prometheus.NewRegistry()

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_notification_duration_seconds",
		Help:    "Test histogram",
		Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0},
	})
	registry.MustRegister(histogram)

	m := &Metrics{
		NotificationDuration: histogram,
	}

	// Record various durations
	durations := []float64{0.05, 0.2, 0.8, 1.5, 3.0, 6.0}
	for _, d := range durations {
		m.NotificationDuration.Observe(d)
	}

	count := testutil.ToFloat64(m.NotificationDuration)
	assert.Equal(t, float64(len(durations)), count)
}

func BenchmarkRecordMessageReceived(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordMessageReceived()
	}
}

func BenchmarkRecordMessageFiltered(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordMessageFiltered()
	}
}

func BenchmarkRecordNotificationSent(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordNotificationSent()
	}
}

func BenchmarkSetWebsocketConnected(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.SetWebsocketConnected(i%2 == 0)
	}
}

func BenchmarkNotificationDurationObserve(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.NotificationDuration.Observe(0.5)
	}
}

func BenchmarkConcurrentMetrics(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.RecordMessageReceived()
			m.RecordNotificationSent()
			m.NotificationDuration.Observe(0.5)
		}
	})
}
