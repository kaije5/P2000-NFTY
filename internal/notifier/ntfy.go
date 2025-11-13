package notifier

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kaije/p2000-nfty/internal/capcode"
	"github.com/kaije/p2000-nfty/internal/websocket"
	"github.com/rs/zerolog"
)

const (
	maxRetries       = 3
	retryBackoff     = 2 * time.Second
	requestTimeout   = 10 * time.Second
	defaultPriority  = "3" // Default ntfy priority (1=min, 5=max)
)

// Notifier sends notifications to ntfy.sh
type Notifier struct {
	server         string
	topic          string
	token          string
	translations   map[string]string
	capcodeLookup  *capcode.Lookup
	httpClient     *http.Client
	logger         zerolog.Logger
}

// NewNotifier creates a new ntfy notifier
func NewNotifier(server, topic, token string, translations map[string]string, capcodeLookup *capcode.Lookup, logger zerolog.Logger) *Notifier {
	return &Notifier{
		server:        strings.TrimSuffix(server, "/"),
		topic:         topic,
		token:         token,
		translations:  translations,
		capcodeLookup: capcodeLookup,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		logger: logger,
	}
}

// Send sends a P2000 message to ntfy with retry logic
func (n *Notifier) Send(ctx context.Context, msg websocket.P2000Message) error {
	// Format message body
	message := n.formatMessage(msg)

	// Format title using capcode lookup
	title := n.formatTitle(msg)

	priority := defaultPriority
	tags := n.getTags(msg.Type)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			n.logger.Debug().
				Int("attempt", attempt+1).
				Int("max_retries", maxRetries).
				Msg("retrying notification")

			select {
			case <-time.After(retryBackoff * time.Duration(attempt)):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if err := n.sendRequest(ctx, title, message, priority, tags); err != nil {
			lastErr = err
			n.logger.Warn().
				Err(err).
				Int("attempt", attempt+1).
				Msg("failed to send notification")
			continue
		}

		n.logger.Info().
			Str("title", title).
			Str("priority", priority).
			Msg("notification sent successfully")
		return nil
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// sendRequest sends HTTP request to ntfy
func (n *Notifier) sendRequest(ctx context.Context, title, message, priority, tags string) error {
	url := fmt.Sprintf("%s/%s", n.server, n.topic)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", tags)

	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// formatTitle creates the notification title
// Format: 🚨 P2000 {CSV-Agency}
func (n *Notifier) formatTitle(msg websocket.P2000Message) string {
	// Try to get agency from first capcode if lookup is available
	agency := msg.Agency

	if n.capcodeLookup != nil && len(msg.Capcodes) > 0 {
		if info := n.capcodeLookup.Get(msg.Capcodes[0]); info != nil {
			agency = info.Agency
		}
	}

	if agency != "" {
		return fmt.Sprintf("🚨 P2000 %s", agency)
	}

	return "🚨 P2000"
}

// formatMessage formats the notification message body with capcodes and translations
func (n *Notifier) formatMessage(msg websocket.P2000Message) string {
	var sb strings.Builder

	// First: message text
	sb.WriteString(msg.Message)
	sb.WriteString("\n\n")

	// Capcode details section
	if len(msg.Capcodes) > 0 {
		sb.WriteString("Capcode Details:\n")

		for i, capcode := range msg.Capcodes {
			if i > 0 {
				sb.WriteString("\n")
			}

			// Try to get detailed info from CSV lookup
			if n.capcodeLookup != nil {
				if info := n.capcodeLookup.Get(capcode); info != nil {
					sb.WriteString(fmt.Sprintf("Capcode: %s\n", capcode))
					if info.Region != "" {
						sb.WriteString(fmt.Sprintf("Regio: %s\n", info.Region))
					}
					if info.Station != "" {
						sb.WriteString(fmt.Sprintf("Kazerne: %s\n", info.Station))
					}
					if info.Function != "" {
						sb.WriteString(fmt.Sprintf("Functie: %s\n", info.Function))
					}
					continue
				}
			}

			// Fallback: use old translation if available
			if translation, ok := n.translations[capcode]; ok {
				sb.WriteString(fmt.Sprintf("Capcode: %s - %s\n", capcode, translation))
			} else {
				sb.WriteString(fmt.Sprintf("Capcode: %s\n", capcode))
			}
		}
	}

	return sb.String()
}

// getTags returns appropriate emoji tags based on message type
func (n *Notifier) getTags(msgType string) string {
	switch msgType {
	case "FLEX":
		return "rotating_light,emergency"
	default:
		return "warning"
	}
}
