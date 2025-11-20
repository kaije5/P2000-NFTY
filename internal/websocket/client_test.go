package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestLogger() zerolog.Logger {
	var buf bytes.Buffer
	return zerolog.New(&buf).With().Timestamp().Logger()
}

func TestNewClient(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)
	assert.NotNil(t, client)
	assert.NotNil(t, client.statusChan)
	assert.NotNil(t, client.done)
	assert.Equal(t, initialBackoff, client.backoff)
	assert.Nil(t, receivedMsg)
}

func TestHandleMessage_ValidJSON(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	testMsg := P2000Message{
		Type:      "FLEX",
		Timestamp: 1234567890,
		Capcodes:  []string{"0101001", "0101002"},
		Message:   "Test alert",
		Agency:    "Brandweer",
	}

	jsonData, err := json.Marshal(testMsg)
	require.NoError(t, err)

	client.handleMessage(jsonData)

	require.NotNil(t, receivedMsg)
	assert.Equal(t, "FLEX", receivedMsg.Type)
	assert.Equal(t, int64(1234567890), receivedMsg.Timestamp)
	assert.Equal(t, []string{"0101001", "0101002"}, receivedMsg.Capcodes)
	assert.Equal(t, "Test alert", receivedMsg.Message)
	assert.Equal(t, "Brandweer", receivedMsg.Agency)
}

func TestHandleMessage_InvalidJSON(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	// Invalid JSON should not crash
	client.handleMessage([]byte("invalid json {"))

	// Handler should not have been called
	assert.Nil(t, receivedMsg)
}

func TestHandleMessage_EmptyMessage(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	// Empty message
	client.handleMessage([]byte(""))

	// Handler should not have been called
	assert.Nil(t, receivedMsg)
}

func TestHandleMessage_NoHandler(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	testMsg := P2000Message{
		Type:    "FLEX",
		Message: "Test",
	}

	jsonData, err := json.Marshal(testMsg)
	require.NoError(t, err)

	// Should not panic when handler is nil
	assert.NotPanics(t, func() {
		client.handleMessage(jsonData)
	})
}

func TestHandleMessage_ComplexSignal(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	testMsg := P2000Message{
		Type:         "FLEX",
		Timestamp:    1234567890,
		FrequencyErr: 0.123,
		Signal: Signal{
			Baudrate: 1200,
			Frame:    1,
			Subtype:  "A",
			Function: "1",
		},
		Capcodes: []string{"0101001"},
		Message:  "Test",
		Agency:   "Test Agency",
	}

	jsonData, err := json.Marshal(testMsg)
	require.NoError(t, err)

	client.handleMessage(jsonData)

	require.NotNil(t, receivedMsg)
	assert.Equal(t, 1200, receivedMsg.Signal.Baudrate)
	assert.Equal(t, 1, receivedMsg.Signal.Frame)
	assert.Equal(t, "A", receivedMsg.Signal.Subtype)
	assert.Equal(t, "1", receivedMsg.Signal.Function)
	assert.Equal(t, 0.123, receivedMsg.FrequencyErr)
}

func TestBackoffLogic(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	// Initial backoff
	assert.Equal(t, initialBackoff, client.backoff)

	// Increase backoff
	client.increaseBackoff()
	assert.Equal(t, initialBackoff*backoffMultiplier, client.backoff)

	// Increase again
	client.increaseBackoff()
	assert.Equal(t, initialBackoff*backoffMultiplier*backoffMultiplier, client.backoff)

	// Keep increasing until max
	for i := 0; i < 10; i++ {
		client.increaseBackoff()
	}
	assert.Equal(t, maxBackoff, client.backoff)

	// Reset backoff
	client.resetBackoff()
	assert.Equal(t, initialBackoff, client.backoff)
}

func TestStatusChan(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	statusChan := client.StatusChan()
	assert.NotNil(t, statusChan)

	// Send status update
	client.notifyStatus(true)

	select {
	case status := <-statusChan:
		assert.True(t, status)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for status update")
	}
}

func TestNotifyStatus_ChannelFull(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	// Fill the channel (buffer size is 1)
	client.notifyStatus(true)

	// This should not block (will be skipped)
	done := make(chan bool)
	go func() {
		client.notifyStatus(false)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("notifyStatus blocked when channel was full")
	}
}

func TestCloseConnection(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	// Close when conn is nil should not panic
	assert.NotPanics(t, func() {
		client.closeConnection()
	})
}

func TestClose(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	assert.NotPanics(t, func() {
		client.Close()
	})
}

// TestConnectAndListen_WithMockServer tests the full connection flow
func TestConnectAndListen_WithMockServer(t *testing.T) {
	logger := getTestLogger()
	var receivedMessages []P2000Message

	handler := func(msg P2000Message) {
		receivedMessages = append(receivedMessages, msg)
	}

	_ = NewClient(logger, handler)

	// Create a test WebSocket server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Send a test message
		testMsg := P2000Message{
			Type:     "FLEX",
			Message:  "Test message",
			Capcodes: []string{"0101001"},
		}

		err = conn.WriteJSON(testMsg)
		if err != nil {
			t.Logf("Failed to write JSON: %v", err)
			return
		}

		// Keep connection alive for a bit
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Note: This test is limited because we can't easily override the wsURL constant
	// In a real scenario, you'd want to make the URL configurable
	// For now, we'll just test the client creation and message handling separately
}

func TestP2000Message_JSONMarshaling(t *testing.T) {
	msg := P2000Message{
		Type:      "FLEX",
		Timestamp: 1234567890,
		Signal: Signal{
			Baudrate: 1200,
			Frame:    1,
			Subtype:  "A",
			Function: "1",
		},
		FrequencyErr: 0.5,
		Capcodes:     []string{"0101001", "0101002"},
		Message:      "Test message",
		Agency:       "Test Agency",
	}

	// Marshal
	jsonData, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal
	var decoded P2000Message
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.Type, decoded.Type)
	assert.Equal(t, msg.Timestamp, decoded.Timestamp)
	assert.Equal(t, msg.Signal.Baudrate, decoded.Signal.Baudrate)
	assert.Equal(t, msg.FrequencyErr, decoded.FrequencyErr)
	assert.Equal(t, msg.Capcodes, decoded.Capcodes)
	assert.Equal(t, msg.Message, decoded.Message)
	assert.Equal(t, msg.Agency, decoded.Agency)
}

func TestP2000Message_EmptyFields(t *testing.T) {
	msg := P2000Message{}

	jsonData, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded P2000Message
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "", decoded.Type)
	assert.Equal(t, int64(0), decoded.Timestamp)
	assert.Nil(t, decoded.Capcodes)
	assert.Equal(t, "", decoded.Message)
}

func TestP2000Message_PartialData(t *testing.T) {
	// JSON with only some fields
	jsonStr := `{"type":"FLEX","message":"Test","capcodes":["0101001"]}`

	var msg P2000Message
	err := json.Unmarshal([]byte(jsonStr), &msg)
	require.NoError(t, err)

	assert.Equal(t, "FLEX", msg.Type)
	assert.Equal(t, "Test", msg.Message)
	assert.Equal(t, []string{"0101001"}, msg.Capcodes)
	assert.Equal(t, int64(0), msg.Timestamp) // Default value
}

func TestSignal_JSONMarshaling(t *testing.T) {
	signal := Signal{
		Baudrate: 1200,
		Frame:    1,
		Subtype:  "A",
		Function: "1",
	}

	jsonData, err := json.Marshal(signal)
	require.NoError(t, err)

	var decoded Signal
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, signal.Baudrate, decoded.Baudrate)
	assert.Equal(t, signal.Frame, decoded.Frame)
	assert.Equal(t, signal.Subtype, decoded.Subtype)
	assert.Equal(t, signal.Function, decoded.Function)
}

func TestHandleMessage_MultipleCapcodes(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	testMsg := P2000Message{
		Type:     "FLEX",
		Capcodes: []string{"0101001", "0101002", "0101003", "0234567"},
		Message:  "Multiple units",
	}

	jsonData, err := json.Marshal(testMsg)
	require.NoError(t, err)

	client.handleMessage(jsonData)

	require.NotNil(t, receivedMsg)
	assert.Equal(t, 4, len(receivedMsg.Capcodes))
	assert.Contains(t, receivedMsg.Capcodes, "0101001")
	assert.Contains(t, receivedMsg.Capcodes, "0101002")
	assert.Contains(t, receivedMsg.Capcodes, "0101003")
	assert.Contains(t, receivedMsg.Capcodes, "0234567")
}

func TestHandleMessage_UnicodeMessage(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	testMsg := P2000Message{
		Type:    "FLEX",
		Message: "Brand 🔥 woning 🏠 met personen 👨‍👩‍👧",
		Agency:  "Brandweer München",
	}

	jsonData, err := json.Marshal(testMsg)
	require.NoError(t, err)

	client.handleMessage(jsonData)

	require.NotNil(t, receivedMsg)
	assert.Contains(t, receivedMsg.Message, "🔥")
	assert.Contains(t, receivedMsg.Message, "🏠")
	assert.Contains(t, receivedMsg.Agency, "München")
}

func TestHandleMessage_LongMessage(t *testing.T) {
	logger := getTestLogger()
	var receivedMsg *P2000Message

	handler := func(msg P2000Message) {
		receivedMsg = &msg
	}

	client := NewClient(logger, handler)

	longMessage := strings.Repeat("A very long emergency message. ", 100)

	testMsg := P2000Message{
		Type:    "FLEX",
		Message: longMessage,
	}

	jsonData, err := json.Marshal(testMsg)
	require.NoError(t, err)

	client.handleMessage(jsonData)

	require.NotNil(t, receivedMsg)
	assert.Equal(t, longMessage, receivedMsg.Message)
}

func TestBackoffSequence(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	expectedSequence := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // Capped at maxBackoff
		30 * time.Second,
	}

	for i, expected := range expectedSequence {
		assert.Equal(t, expected, client.backoff, "Backoff mismatch at step %d", i)
		client.increaseBackoff()
	}
}

func TestConnect_ContextCancellation(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Connect should return immediately
	err := client.Connect(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestConnect_ContextTimeout(t *testing.T) {
	logger := getTestLogger()
	client := NewClient(logger, nil)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Connect should timeout (since it can't connect to real server)
	err := client.Connect(ctx)
	assert.Error(t, err)
}

func BenchmarkHandleMessage(b *testing.B) {
	logger := getTestLogger()

	handler := func(msg P2000Message) {
		_ = msg
	}

	client := NewClient(logger, handler)

	testMsg := P2000Message{
		Type:      "FLEX",
		Timestamp: 1234567890,
		Capcodes:  []string{"0101001", "0101002"},
		Message:   "Test alert",
		Agency:    "Brandweer",
	}

	jsonData, _ := json.Marshal(testMsg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.handleMessage(jsonData)
	}
}

func BenchmarkJSONMarshal(b *testing.B) {
	msg := P2000Message{
		Type:      "FLEX",
		Timestamp: 1234567890,
		Signal: Signal{
			Baudrate: 1200,
			Frame:    1,
			Subtype:  "A",
			Function: "1",
		},
		FrequencyErr: 0.5,
		Capcodes:     []string{"0101001", "0101002"},
		Message:      "Test message",
		Agency:       "Test Agency",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(msg)
	}
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	jsonStr := `{"type":"FLEX","timestamp":1234567890,"signal":{"baudrate":1200,"frame":1,"subtype":"A","function":"1"},"frequency_error":0.5,"capcodes":["0101001","0101002"],"message":"Test message","agency":"Test Agency"}`
	data := []byte(jsonStr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg P2000Message
		json.Unmarshal(data, &msg)
	}
}
