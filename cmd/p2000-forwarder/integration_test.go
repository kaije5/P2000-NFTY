package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaije/p2000-nfty/internal/capcode"
	"github.com/kaije/p2000-nfty/internal/config"
	"github.com/kaije/p2000-nfty/internal/filter"
	"github.com/kaije/p2000-nfty/internal/notifier"
	"github.com/kaije/p2000-nfty/internal/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestLogger() zerolog.Logger {
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func TestConfigLoading_Integration(t *testing.T) {
	// Test loading actual config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
forward_all: false
capcodes:
  - "0101001"
  - "0101002"
ntfy:
  server: "https://ntfy.sh"
  topic: "test"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.False(t, cfg.ForwardAll)
	assert.Equal(t, 2, len(cfg.Capcodes))
	assert.Equal(t, "https://ntfy.sh", cfg.Ntfy.Server)
	assert.Equal(t, "test", cfg.Ntfy.Topic)
}

func TestFilterAndNotifier_Integration(t *testing.T) {
	logger := getTestLogger()

	// Create capcode CSV
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm
0101002;Ambulance;Utrecht;Oost;A1 Dienst`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	// Load capcode lookup
	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	// Create filter
	capcodeFilter := filter.NewCapcodeFilter(false, []string{"0101001"}, logger)

	// Create test server to receive notifications
	var receivedNotifications int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedNotifications++
		assert.Equal(t, "POST", r.Method)

		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "Brandweer")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create notifier
	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, lookup, logger)

	// Test messages
	messages := []websocket.P2000Message{
		{
			Type:     "FLEX",
			Capcodes: []string{"0101001"},
			Message:  "Brand woning",
		},
		{
			Type:     "FLEX",
			Capcodes: []string{"0101002"}, // Should be filtered out
			Message:  "Ambulance",
		},
		{
			Type:     "FLEX",
			Capcodes: []string{"0101001"},
			Message:  "Brand bedrijfspand",
		},
	}

	ctx := context.Background()
	for _, msg := range messages {
		if capcodeFilter.ShouldForward(msg.Capcodes) {
			err := ntfy.Send(ctx, msg)
			assert.NoError(t, err)
		}
	}

	// Should have sent 2 notifications (0101001 matched twice)
	assert.Equal(t, 2, receivedNotifications)
}

func TestMetrics_Integration(t *testing.T) {
	logger := getTestLogger()
	// Note: Skip metrics.NewMetrics() to avoid duplicate registration in tests

	// Create filter
	capcodeFilter := filter.NewCapcodeFilter(false, []string{"0101001"}, logger)

	// Test messages
	messages := []websocket.P2000Message{
		{Capcodes: []string{"0101001"}}, // Match
		{Capcodes: []string{"0101002"}}, // No match
		{Capcodes: []string{"0101001"}}, // Match
		{Capcodes: []string{"9999999"}}, // No match
	}

	matchCount := 0
	for _, msg := range messages {
		if capcodeFilter.ShouldForward(msg.Capcodes) {
			matchCount++
		}
	}

	// Verify filtering works
	assert.Equal(t, 2, matchCount)
	assert.Equal(t, 1, capcodeFilter.Count())
}

func TestEndToEnd_ForwardAll(t *testing.T) {
	logger := getTestLogger()

	// Create test server
	var receivedCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create components with forward_all enabled
	capcodeFilter := filter.NewCapcodeFilter(true, []string{}, logger)
	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, nil, logger)

	// Test messages
	messages := []websocket.P2000Message{
		{Type: "FLEX", Message: "Message 1"},
		{Type: "FLEX", Message: "Message 2"},
		{Type: "FLEX", Message: "Message 3"},
	}

	ctx := context.Background()
	for _, msg := range messages {
		if capcodeFilter.ShouldForward(msg.Capcodes) {
			err := ntfy.Send(ctx, msg)
			assert.NoError(t, err)
		}
	}

	// All messages should be forwarded
	assert.Equal(t, 3, receivedCount)
}

func TestEndToEnd_WithRetry(t *testing.T) {
	logger := getTestLogger()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	err := ntfy.Send(context.Background(), msg)
	assert.NoError(t, err)
	assert.Equal(t, 2, attempts) // Should have retried once
}

func TestEndToEnd_WithAuthentication(t *testing.T) {
	logger := getTestLogger()

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ntfy := notifier.NewNotifier(server.URL, "test", "my-token", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	err := ntfy.Send(context.Background(), msg)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer my-token", authHeader)
}

func TestEndToEnd_MultipleCapcodes(t *testing.T) {
	logger := getTestLogger()

	// Create CSV with multiple entries
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm
0101002;Ambulance;Utrecht;Oost;A1 Dienst
0101003;Politie;Utrecht;West;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	capcodeFilter := filter.NewCapcodeFilter(false, []string{"0101001", "0101002", "0101003"}, logger)
	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, lookup, logger)

	msg := websocket.P2000Message{
		Type:     "FLEX",
		Capcodes: []string{"0101001", "0101002", "0101003"},
		Message:  "Multi-unit response",
	}

	if capcodeFilter.ShouldForward(msg.Capcodes) {
		err := ntfy.Send(context.Background(), msg)
		assert.NoError(t, err)
	}

	// Verify all capcodes are in the notification
	assert.Contains(t, receivedBody, "0101001")
	assert.Contains(t, receivedBody, "0101002")
	assert.Contains(t, receivedBody, "0101003")
	// First agency is used as the main agency
	assert.Contains(t, receivedBody, "Brandweer")
	// Location details from all capcodes
	assert.Contains(t, receivedBody, "Utrecht")
	assert.Contains(t, receivedBody, "Centrum")
	assert.Contains(t, receivedBody, "Oost")
}

func TestHealthEndpoint_Integration(t *testing.T) {
	// Test that health endpoint would work
	// This is a simplified test as we can't easily test the full HTTP server

	isConnected := true
	getHealth := func() (int, string) {
		if isConnected {
			return http.StatusOK, "OK"
		}
		return http.StatusServiceUnavailable, "WebSocket disconnected"
	}

	status, body := getHealth()
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "OK", body)

	isConnected = false
	status, body = getHealth()
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Contains(t, body, "disconnected")
}

func TestMessageFlow_Complete(t *testing.T) {
	logger := getTestLogger()

	// Setup complete environment
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	capcodeFilter := filter.NewCapcodeFilter(false, []string{"0101001"}, logger)

	var notifications []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		notifications = append(notifications, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, lookup, logger)

	// Simulate message flow
	messages := []websocket.P2000Message{
		{Type: "FLEX", Capcodes: []string{"0101001"}, Message: "Brand"},
		{Type: "FLEX", Capcodes: []string{"9999999"}, Message: "Other"},
		{Type: "FLEX", Capcodes: []string{"0101001"}, Message: "Brand 2"},
	}

	ctx := context.Background()
	for _, msg := range messages {
		if capcodeFilter.ShouldForward(msg.Capcodes) {
			err := ntfy.Send(ctx, msg)
			assert.NoError(t, err)
		}
	}

	// Verify workflow
	assert.Equal(t, 2, len(notifications))
	assert.Contains(t, notifications[0], "Brandweer")
	assert.Contains(t, notifications[1], "Brandweer")
}

func TestConfigWithEnvOverrides_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
forward_all: true
capcodes:
  - "0101001"
ntfy:
  server: "https://ntfy.sh"
  topic: "default-topic"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set environment variables
	os.Setenv("FORWARD_ALL", "false")
	os.Setenv("NTFY_TOPIC", "env-topic")
	defer func() {
		os.Unsetenv("FORWARD_ALL")
		os.Unsetenv("NTFY_TOPIC")
	}()

	cfg, err := config.Load(configPath)
	require.NoError(t, err)

	// Verify env vars override config file
	assert.False(t, cfg.ForwardAll)
	assert.Equal(t, "env-topic", cfg.Ntfy.Topic)
}

func TestConcurrentMessageProcessing(t *testing.T) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate network delay
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, nil, logger)
	capcodeFilter := filter.NewCapcodeFilter(true, []string{}, logger)

	// Process multiple messages concurrently
	numMessages := 10
	done := make(chan bool, numMessages)
	ctx := context.Background()

	for i := 0; i < numMessages; i++ {
		go func(id int) {
			msg := websocket.P2000Message{
				Type:    "FLEX",
				Message: "Concurrent test",
			}

			if capcodeFilter.ShouldForward(msg.Capcodes) {
				err := ntfy.Send(ctx, msg)
				assert.NoError(t, err)
			}
			done <- true
		}(i)
	}

	// Wait for all messages
	for i := 0; i < numMessages; i++ {
		<-done
	}
}

func BenchmarkCompleteMessageFlow(b *testing.B) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	capcodeFilter := filter.NewCapcodeFilter(false, []string{"0101001"}, logger)
	ntfy := notifier.NewNotifier(server.URL, "test", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:     "FLEX",
		Capcodes: []string{"0101001"},
		Message:  "Test",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if capcodeFilter.ShouldForward(msg.Capcodes) {
			ntfy.Send(ctx, msg)
		}
	}
}
