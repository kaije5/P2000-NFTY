package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	wsURL              = "wss://p2000.riekeltbrands.nl/websocket"
	initialBackoff     = 1 * time.Second
	maxBackoff         = 30 * time.Second
	backoffMultiplier  = 2
	pingInterval       = 30 * time.Second
	pongTimeout        = 10 * time.Second
	writeTimeout       = 10 * time.Second
)

// P2000Message represents a P2000 notification message
type P2000Message struct {
	Type          string   `json:"type"`
	Timestamp     int64    `json:"timestamp"`
	Signal        Signal   `json:"signal"`
	FrequencyErr  float64  `json:"frequency_error"`
	Capcodes      []string `json:"capcodes"`
	Message       string   `json:"message"`
	Agency        string   `json:"agency"`
}

// Signal represents the signal information
type Signal struct {
	Baudrate int    `json:"baudrate"`
	Frame    int    `json:"frame"`
	Subtype  string `json:"subtype"`
	Function string `json:"function"`
}

// Client handles WebSocket connection with automatic reconnection
type Client struct {
	conn         *websocket.Conn
	logger       zerolog.Logger
	msgHandler   func(P2000Message)
	statusChan   chan bool // true = connected, false = disconnected
	done         chan struct{}
	backoff      time.Duration
}

// NewClient creates a new WebSocket client
func NewClient(logger zerolog.Logger, msgHandler func(P2000Message)) *Client {
	return &Client{
		logger:     logger,
		msgHandler: msgHandler,
		statusChan: make(chan bool, 1),
		done:       make(chan struct{}),
		backoff:    initialBackoff,
	}
}

// Connect establishes WebSocket connection with retry logic
func (c *Client) Connect(ctx context.Context) error {
	c.logger.Info().Msg("starting websocket client")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info().Msg("websocket client shutting down")
			return ctx.Err()
		default:
			if err := c.connectAndListen(ctx); err != nil {
				c.notifyStatus(false)
				c.logger.Error().Err(err).
					Dur("backoff", c.backoff).
					Msg("connection failed, retrying")

				select {
				case <-time.After(c.backoff):
					c.increaseBackoff()
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// connectAndListen establishes connection and processes messages
func (c *Client) connectAndListen(ctx context.Context) error {
	c.logger.Info().Str("url", wsURL).Msg("connecting to websocket")

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.conn = conn
	c.resetBackoff()
	c.notifyStatus(true)
	c.logger.Info().Msg("websocket connection established")

	// Set initial read deadline
	readDeadline := pingInterval + pongTimeout
	c.conn.SetReadDeadline(time.Now().Add(readDeadline))

	// Setup ping/pong handlers
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(readDeadline))
		return nil
	})

	// Start ping ticker
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	// Goroutine for sending pings
	go func() {
		for {
			select {
			case <-pingTicker.C:
				c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					c.logger.Error().Err(err).Msg("failed to send ping")
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Read messages
	for {
		select {
		case <-ctx.Done():
			c.closeConnection()
			return ctx.Err()
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				c.closeConnection()
				return fmt.Errorf("read failed: %w", err)
			}

			// Extend read deadline after successful read
			c.conn.SetReadDeadline(time.Now().Add(readDeadline))
			c.handleMessage(message)
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *Client) handleMessage(data []byte) {
	var msg P2000Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Error().Err(err).
			Str("raw_message", string(data)).
			Msg("failed to parse message")
		return
	}

	c.logger.Debug().
		Str("type", msg.Type).
		Str("agency", msg.Agency).
		Strs("capcodes", msg.Capcodes).
		Str("message", msg.Message).
		Msg("received P2000 message")

	if c.msgHandler != nil {
		c.msgHandler(msg)
	}
}

// closeConnection safely closes the WebSocket connection
func (c *Client) closeConnection() {
	if c.conn != nil {
		c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		c.conn.Close()
		c.conn = nil
	}
}

// StatusChan returns a channel that receives connection status updates
func (c *Client) StatusChan() <-chan bool {
	return c.statusChan
}

// notifyStatus sends connection status update
func (c *Client) notifyStatus(connected bool) {
	select {
	case c.statusChan <- connected:
	default:
		// Channel full, skip
	}
}

// increaseBackoff increases reconnection backoff time
func (c *Client) increaseBackoff() {
	c.backoff *= backoffMultiplier
	if c.backoff > maxBackoff {
		c.backoff = maxBackoff
	}
}

// resetBackoff resets reconnection backoff to initial value
func (c *Client) resetBackoff() {
	c.backoff = initialBackoff
}

// Close gracefully shuts down the client
func (c *Client) Close() {
	close(c.done)
	c.closeConnection()
	c.logger.Info().Msg("websocket client closed")
}
