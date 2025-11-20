package notifier

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kaije/p2000-nfty/internal/capcode"
	"github.com/kaije/p2000-nfty/internal/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestLogger() zerolog.Logger {
	var buf bytes.Buffer
	return zerolog.New(&buf).With().Timestamp().Logger()
}

func TestNewNotifier(t *testing.T) {
	logger := getTestLogger()
	translations := map[string]string{"0101001": "Fire Dept"}

	tests := []struct {
		name     string
		server   string
		topic    string
		token    string
		username string
		password string
		wantURL  string
	}{
		{
			name:    "Basic setup",
			server:  "https://ntfy.sh",
			topic:   "test-topic",
			token:   "test-token",
			wantURL: "https://ntfy.sh",
		},
		{
			name:    "Server with trailing slash",
			server:  "https://ntfy.sh/",
			topic:   "test-topic",
			wantURL: "https://ntfy.sh",
		},
		{
			name:     "With Basic Auth",
			server:   "https://ntfy.sh",
			topic:    "test-topic",
			username: "user",
			password: "pass",
			wantURL:  "https://ntfy.sh",
		},
		{
			name:    "Custom server",
			server:  "https://custom.ntfy.io",
			topic:   "alerts",
			wantURL: "https://custom.ntfy.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := NewNotifier(tt.server, tt.topic, tt.token, tt.username, tt.password, translations, nil, logger)
			assert.NotNil(t, notifier)
			assert.Equal(t, tt.wantURL, notifier.server)
			assert.Equal(t, tt.topic, notifier.topic)
			assert.Equal(t, tt.token, notifier.token)
			assert.Equal(t, tt.username, notifier.username)
			assert.Equal(t, tt.password, notifier.password)
			assert.NotNil(t, notifier.httpClient)
		})
	}
}

func TestSend_Success(t *testing.T) {
	logger := getTestLogger()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/test-topic", r.URL.Path)
		assert.Equal(t, "3", r.Header.Get("Priority"))
		assert.Contains(t, r.Header.Get("Tags"), "rotating_light")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:     "FLEX",
		Message:  "Test alert",
		Capcodes: []string{"0101001"},
	}

	ctx := context.Background()
	err := notifier.Send(ctx, msg)
	assert.NoError(t, err)
}

func TestSend_WithBearerToken(t *testing.T) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer test-token-123", auth)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "test-token-123", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test alert",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, msg)
	assert.NoError(t, err)
}

func TestSend_WithBasicAuth(t *testing.T) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "testuser", username)
		assert.Equal(t, "testpass", password)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "testuser", "testpass", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test alert",
	}

	ctx := context.Background()
	err := notifier.Send(ctx, msg)
	assert.NoError(t, err)
}

func TestSend_BasicAuthPreferredOverToken(t *testing.T) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should use Basic Auth, not Bearer token
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "user", username)
		assert.Equal(t, "pass", password)

		// Authorization header should start with "Basic", not "Bearer"
		authHeader := r.Header.Get("Authorization")
		assert.Contains(t, authHeader, "Basic")
		assert.NotContains(t, authHeader, "Bearer")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "token", "user", "pass", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	err := notifier.Send(context.Background(), msg)
	assert.NoError(t, err)
}

func TestSend_RetryOnFailure(t *testing.T) {
	logger := getTestLogger()
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	err := notifier.Send(context.Background(), msg)
	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestSend_MaxRetriesExceeded(t *testing.T) {
	logger := getTestLogger()
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	err := notifier.Send(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
	assert.Equal(t, 3, attempts)
}

func TestSend_ContextCancellation(t *testing.T) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.Send(ctx, msg)
	assert.Error(t, err)
}

func TestSend_ContextTimeout(t *testing.T) {
	logger := getTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := notifier.Send(ctx, msg)
	assert.Error(t, err)
}

func TestFormatTitle(t *testing.T) {
	logger := getTestLogger()
	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, nil, logger)

	tests := []struct {
		name     string
		msg      websocket.P2000Message
		expected string
	}{
		{
			name: "With message",
			msg: websocket.P2000Message{
				Message: "Brand woning",
			},
			expected: "🚨 Brand woning",
		},
		{
			name:     "Without message",
			msg:      websocket.P2000Message{},
			expected: "🚨 P2000",
		},
		{
			name: "Empty message",
			msg: websocket.P2000Message{
				Message: "",
			},
			expected: "🚨 P2000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := notifier.formatTitle(tt.msg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatMessage_NoLookup(t *testing.T) {
	logger := getTestLogger()
	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, nil, logger)

	msg := websocket.P2000Message{
		Capcodes: []string{"0101001", "0101002"},
	}

	result := notifier.formatMessage(msg)
	assert.Contains(t, result, "overig")
}

func TestFormatMessage_WithLookup(t *testing.T) {
	logger := getTestLogger()

	// Create a temporary capcode CSV
	tmpDir := t.TempDir()
	csvPath := tmpDir + "/capcodes.csv"
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm
0101002;Ambulance;Utrecht;Oost;A1 Dienst`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, lookup, logger)

	msg := websocket.P2000Message{
		Capcodes: []string{"0101001", "0101002"},
	}

	result := notifier.formatMessage(msg)
	assert.Contains(t, result, "Brandweer")
	assert.Contains(t, result, "0101001")
	assert.Contains(t, result, "Utrecht")
	assert.Contains(t, result, "Centrum")
	assert.Contains(t, result, "Kazernealarm")
}

func TestFormatMessage_WithPartialLookup(t *testing.T) {
	logger := getTestLogger()

	tmpDir := t.TempDir()
	csvPath := tmpDir + "/capcodes.csv"
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, lookup, logger)

	// First capcode exists, second doesn't
	msg := websocket.P2000Message{
		Capcodes: []string{"0101001", "9999999"},
	}

	result := notifier.formatMessage(msg)
	assert.Contains(t, result, "Brandweer")
	assert.Contains(t, result, "0101001")
}

func TestFormatMessage_EmptyFields(t *testing.T) {
	logger := getTestLogger()

	tmpDir := t.TempDir()
	csvPath := tmpDir + "/capcodes.csv"
	csvContent := `0101001;;;;`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, lookup, logger)

	msg := websocket.P2000Message{
		Capcodes: []string{"0101001"},
	}

	result := notifier.formatMessage(msg)
	// Should handle empty fields gracefully
	assert.NotEmpty(t, result)
}

func TestGetTags(t *testing.T) {
	logger := getTestLogger()
	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, nil, logger)

	tests := []struct {
		name     string
		msgType  string
		expected string
	}{
		{
			name:     "FLEX type",
			msgType:  "FLEX",
			expected: "rotating_light,emergency",
		},
		{
			name:     "Unknown type",
			msgType:  "UNKNOWN",
			expected: "warning",
		},
		{
			name:     "Empty type",
			msgType:  "",
			expected: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := notifier.getTags(tt.msgType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSendRequest_ErrorCases(t *testing.T) {
	logger := getTestLogger()

	tests := []struct {
		name       string
		statusCode int
		wantError  bool
		errorMsg   string
	}{
		{
			name:       "Success 200",
			statusCode: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Success 201",
			statusCode: http.StatusCreated,
			wantError:  false,
		},
		{
			name:       "Bad Request",
			statusCode: http.StatusBadRequest,
			wantError:  true,
			errorMsg:   "unexpected status code: 400",
		},
		{
			name:       "Unauthorized",
			statusCode: http.StatusUnauthorized,
			wantError:  true,
			errorMsg:   "unexpected status code: 401",
		},
		{
			name:       "Internal Server Error",
			statusCode: http.StatusInternalServerError,
			wantError:  true,
			errorMsg:   "unexpected status code: 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

			err := notifier.sendRequest(context.Background(), "title", "message", "3", "tags")

			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSendRequest_Headers(t *testing.T) {
	logger := getTestLogger()

	receivedHeaders := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders["Title"] = r.Header.Get("Title")
		receivedHeaders["Priority"] = r.Header.Get("Priority")
		receivedHeaders["Tags"] = r.Header.Get("Tags")

		body, _ := io.ReadAll(r.Body)
		receivedHeaders["Body"] = string(body)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewNotifier(server.URL, "test-topic", "", "", "", nil, nil, logger)

	err := notifier.sendRequest(context.Background(), "Test Title", "Test Message", "5", "fire,emergency")
	assert.NoError(t, err)

	assert.Equal(t, "Test Title", receivedHeaders["Title"])
	assert.Equal(t, "5", receivedHeaders["Priority"])
	assert.Equal(t, "fire,emergency", receivedHeaders["Tags"])
	assert.Equal(t, "Test Message", receivedHeaders["Body"])
}

func TestSend_FullIntegration(t *testing.T) {
	logger := getTestLogger()

	var receivedRequest *http.Request
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = r
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	csvPath := tmpDir + "/capcodes.csv"
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	notifier := NewNotifier(server.URL, "alerts", "my-token", "", "", nil, lookup, logger)

	msg := websocket.P2000Message{
		Type:     "FLEX",
		Message:  "Brand in gebouw",
		Capcodes: []string{"0101001"},
	}

	err = notifier.Send(context.Background(), msg)
	require.NoError(t, err)

	// Verify request
	assert.NotNil(t, receivedRequest)
	assert.Equal(t, "POST", receivedRequest.Method)
	assert.Equal(t, "/alerts", receivedRequest.URL.Path)
	assert.Equal(t, "Bearer my-token", receivedRequest.Header.Get("Authorization"))
	assert.Contains(t, receivedRequest.Header.Get("Title"), "🚨")
	assert.Contains(t, receivedRequest.Header.Get("Title"), "Brand in gebouw")
	assert.Equal(t, "3", receivedRequest.Header.Get("Priority"))
	assert.Equal(t, "rotating_light,emergency", receivedRequest.Header.Get("Tags"))

	// Verify body contains capcode info
	assert.Contains(t, receivedBody, "Brandweer")
	assert.Contains(t, receivedBody, "0101001")
}

func TestNotifier_MultipleCapcodes(t *testing.T) {
	logger := getTestLogger()

	tmpDir := t.TempDir()
	csvPath := tmpDir + "/capcodes.csv"
	csvContent := `0101001;Brandweer;Utrecht;Centrum;Kazernealarm
0101002;Ambulance;Utrecht;Oost;A1 Dienst
0101003;Politie;Utrecht;West;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := capcode.NewLookup(csvPath)
	require.NoError(t, err)

	notifier := NewNotifier("https://ntfy.sh", "topic", "", "", "", nil, lookup, logger)

	msg := websocket.P2000Message{
		Capcodes: []string{"0101001", "0101002", "0101003"},
	}

	result := notifier.formatMessage(msg)

	// Should contain all three capcodes
	assert.Contains(t, result, "0101001")
	assert.Contains(t, result, "0101002")
	assert.Contains(t, result, "0101003")

	// Should contain the first agency (used as main agency)
	assert.Contains(t, result, "Brandweer")

	// Should contain location details from all capcodes
	assert.Contains(t, result, "Utrecht")
	assert.Contains(t, result, "Centrum")
	assert.Contains(t, result, "Oost")
	assert.Contains(t, result, "West")
}
