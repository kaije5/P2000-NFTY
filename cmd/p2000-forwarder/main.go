package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaije/p2000-nfty/internal/capcode"
	"github.com/kaije/p2000-nfty/internal/config"
	"github.com/kaije/p2000-nfty/internal/filter"
	"github.com/kaije/p2000-nfty/internal/metrics"
	"github.com/kaije/p2000-nfty/internal/notifier"
	"github.com/kaije/p2000-nfty/internal/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	healthCheckWindow = 5 * time.Minute
)

type Application struct {
	cfg        *config.Config
	logger     zerolog.Logger
	metrics    *metrics.Metrics
	wsClient   *websocket.Client
	filter     *filter.CapcodeFilter
	notifier   *notifier.Notifier
	httpServer *http.Server
	lastMsg    time.Time
	wsConnected bool
}

func main() {
	// Setup structured logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load configuration")
	}

	logger.Info().
		Str("ntfy_server", cfg.Ntfy.Server).
		Str("ntfy_topic", cfg.Ntfy.Topic).
		Bool("forward_all", cfg.ForwardAll).
		Int("capcodes", len(cfg.Capcodes)).
		Msg("configuration loaded")

	// Initialize capcode lookup
	var capcodeLookup *capcode.Lookup
	if cfg.CapcodeCSVPath != "" {
		lookup, err := capcode.NewLookup(cfg.CapcodeCSVPath)
		if err != nil {
			logger.Warn().
				Err(err).
				Str("csv_path", cfg.CapcodeCSVPath).
				Msg("failed to load capcode CSV, continuing without lookup")
		} else {
			capcodeLookup = lookup
			logger.Info().
				Str("csv_path", cfg.CapcodeCSVPath).
				Msg("capcode lookup loaded successfully")
		}
	}

	// Initialize application
	app := &Application{
		cfg:     cfg,
		logger:  logger,
		metrics: metrics.NewMetrics(),
		lastMsg: time.Now(),
	}

	// Initialize filter
	app.filter = filter.NewCapcodeFilter(cfg.ForwardAll, cfg.Capcodes, logger)

	// Initialize notifier
	app.notifier = notifier.NewNotifier(
		cfg.Ntfy.Server,
		cfg.Ntfy.Topic,
		cfg.Ntfy.Token,
		cfg.CapcodeTranslations,
		capcodeLookup,
		logger,
	)

	// Initialize WebSocket client
	app.wsClient = websocket.NewClient(logger, app.handleMessage)

	// Setup HTTP server for metrics and health checks
	app.setupHTTPServer()

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start WebSocket client in goroutine
	go func() {
		if err := app.wsClient.Connect(ctx); err != nil && err != context.Canceled {
			logger.Error().Err(err).Msg("websocket client error")
		}
	}()

	// Monitor WebSocket connection status
	go app.monitorConnectionStatus(ctx)

	// Start HTTP server
	go func() {
		logger.Info().
			Int("port", cfg.Server.Port).
			Str("metrics", cfg.Server.MetricsPath).
			Str("health", cfg.Server.HealthPath).
			Msg("starting HTTP server")

		if err := app.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	logger.Info().Msg("shutdown signal received")

	// Graceful shutdown
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := app.httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("HTTP server shutdown error")
	}

	app.wsClient.Close()
	logger.Info().Msg("application stopped")
}

// setupHTTPServer configures the HTTP server with metrics and health endpoints
func (app *Application) setupHTTPServer() {
	mux := http.NewServeMux()

	// Metrics endpoint
	mux.Handle(app.cfg.Server.MetricsPath, promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc(app.cfg.Server.HealthPath, app.healthCheckHandler)

	app.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", app.cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(app.cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(app.cfg.Server.WriteTimeout) * time.Second,
	}
}

// healthCheckHandler handles health check requests
func (app *Application) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// Check if WebSocket is connected
	if !app.wsConnected {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "unhealthy: websocket disconnected\n")
		return
	}

	// Check if we've received a message recently
	if time.Since(app.lastMsg) > healthCheckWindow {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "unhealthy: no messages received in %v\n", healthCheckWindow)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "healthy\n")
}

// monitorConnectionStatus monitors WebSocket connection status changes
func (app *Application) monitorConnectionStatus(ctx context.Context) {
	for {
		select {
		case connected := <-app.wsClient.StatusChan():
			app.wsConnected = connected
			app.metrics.SetWebsocketConnected(connected)
			if connected {
				app.logger.Info().Msg("websocket connection established")
			} else {
				app.logger.Warn().Msg("websocket connection lost")
			}
		case <-ctx.Done():
			return
		}
	}
}

// handleMessage processes incoming P2000 messages
func (app *Application) handleMessage(msg websocket.P2000Message) {
	app.metrics.RecordMessageReceived()
	app.lastMsg = time.Now()

	// Check if message should be forwarded
	if !app.filter.ShouldForward(msg.Capcodes) {
		return
	}

	app.metrics.RecordMessageFiltered()

	// Send notification with timing
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.notifier.Send(ctx, msg); err != nil {
		app.logger.Error().
			Err(err).
			Str("agency", msg.Agency).
			Strs("capcodes", msg.Capcodes).
			Msg("failed to send notification")
		app.metrics.RecordNotificationFailed()
		return
	}

	duration := time.Since(start)
	app.metrics.NotificationDuration.Observe(duration.Seconds())
	app.metrics.RecordNotificationSent()

	app.logger.Info().
		Str("agency", msg.Agency).
		Strs("capcodes", msg.Capcodes).
		Dur("duration", duration).
		Msg("notification forwarded")
}
