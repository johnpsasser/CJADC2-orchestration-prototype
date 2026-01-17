// Package main provides the CJADC2 API Gateway service
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/agile-defense/cjadc2/pkg/handler"
	"github.com/agile-defense/cjadc2/pkg/messages"
	"github.com/agile-defense/cjadc2/pkg/opa"
	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// Config holds the API gateway configuration
type Config struct {
	// Server settings
	HTTPAddr string
	HTTPPort int

	// External services
	NATSUrl     string
	PostgresURL string
	OPAUrl      string

	// CORS settings
	CORSOrigins []string

	// Logging
	LogLevel string
	LogJSON  bool
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		HTTPAddr:    "0.0.0.0",
		HTTPPort:    8080,
		NATSUrl:     getEnv("NATS_URL", "nats://localhost:4222"),
		PostgresURL: getEnv("POSTGRES_URL", "postgres://cjadc2:cjadc2@localhost:5432/cjadc2?sslmode=disable"),
		OPAUrl:      getEnv("OPA_URL", "http://localhost:8181"),
		CORSOrigins: []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:3001", "http://127.0.0.1:3001"},
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		LogJSON:     getEnv("LOG_JSON", "false") == "true",
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Prometheus metrics
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cjadc2_api_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cjadc2_api_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	wsConnectionsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cjadc2_api_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	natsConnectionStatus = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cjadc2_api_nats_connection_status",
			Help: "NATS connection status (1=connected, 0=disconnected)",
		},
	)

	dbConnectionStatus = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cjadc2_api_db_connection_status",
			Help: "Database connection status (1=connected, 0=disconnected)",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(wsConnectionsActive)
	prometheus.MustRegister(natsConnectionStatus)
	prometheus.MustRegister(dbConnectionStatus)
}

func main() {
	cfg := DefaultConfig()

	// Setup logging
	setupLogging(cfg)

	log.Info().
		Str("nats_url", cfg.NATSUrl).
		Str("postgres_url", maskPassword(cfg.PostgresURL)).
		Str("opa_url", cfg.OPAUrl).
		Int("http_port", cfg.HTTPPort).
		Msg("Starting CJADC2 API Gateway")

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	}()

	// Connect to services
	nc, db, opaClient, err := connectServices(ctx, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to services")
	}
	defer func() {
		if nc != nil {
			nc.Close()
		}
		if db != nil {
			db.Close()
		}
	}()

	// Create WebSocket hub
	wsHub := handler.NewWebSocketHub(nc, log.Logger)

	// Create router
	router := setupRouter(cfg, db, nc, opaClient, wsHub)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.HTTPAddr, cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start services
	g, gCtx := errgroup.WithContext(ctx)

	// Start WebSocket hub
	g.Go(func() error {
		wsHub.Run(gCtx)
		return nil
	})

	// Start track persistence consumer (persist correlated tracks to PostgreSQL)
	if nc != nil {
		g.Go(func() error {
			return runTrackPersistenceConsumer(gCtx, nc, db)
		})
	}

	// Update WebSocket connection gauge periodically
	g.Go(func() error {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-gCtx.Done():
				return nil
			case <-ticker.C:
				wsConnectionsActive.Set(float64(wsHub.ClientCount()))
			}
		}
	})

	// Start HTTP server
	g.Go(func() error {
		log.Info().Str("addr", server.Addr).Msg("HTTP server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("HTTP server error: %w", err)
		}
		return nil
	})

	// Graceful shutdown
	g.Go(func() error {
		<-gCtx.Done()
		log.Info().Msg("Shutting down HTTP server")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		return server.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		log.Error().Err(err).Msg("Server error")
	}

	log.Info().Msg("CJADC2 API Gateway shutdown complete")
}

func setupLogging(cfg Config) {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output format
	if cfg.LogJSON {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
			With().Timestamp().Logger()
	}
}

func connectServices(ctx context.Context, cfg Config) (*nats.Conn, *postgres.Pool, *opa.Client, error) {
	var nc *nats.Conn
	var db *postgres.Pool
	var err error

	// Connect to NATS
	nc, err = nats.Connect(cfg.NATSUrl,
		nats.Name("cjadc2-api-gateway"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn().Err(err).Msg("NATS disconnected")
			natsConnectionStatus.Set(0)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Info().Msg("NATS reconnected")
			natsConnectionStatus.Set(1)
		}),
	)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to NATS, continuing without real-time updates")
		nc = nil
	} else {
		log.Info().Str("url", cfg.NATSUrl).Msg("Connected to NATS")
		natsConnectionStatus.Set(1)
	}

	// Connect to PostgreSQL
	db, err = postgres.NewPoolFromURL(ctx, cfg.PostgresURL)
	if err != nil {
		if nc != nil {
			nc.Close()
		}
		return nil, nil, nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	log.Info().Msg("Connected to PostgreSQL")
	dbConnectionStatus.Set(1)

	// Create OPA client
	opaClient := opa.NewClient(cfg.OPAUrl)

	return nc, db, opaClient, nil
}

func setupRouter(cfg Config, db *postgres.Pool, nc *nats.Conn, opaClient *opa.Client, wsHub *handler.WebSocketHub) chi.Router {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(correlationIDMiddleware)
	r.Use(middleware.RealIP)
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(prometheusMiddleware)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Correlation-ID", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Correlation-ID", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", healthHandler(db, nc, opaClient))

	// Prometheus metrics
	r.Handle("/metrics", promhttp.Handler())

	// WebSocket endpoint
	wsHandler := handler.NewWebSocketHandler(wsHub, log.Logger)
	r.Handle("/ws", wsHandler)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Track handlers
		trackHandler := handler.NewTrackHandler(db, log.Logger)
		r.Mount("/tracks", trackHandler.Routes())

		// Proposal handlers
		proposalHandler := handler.NewProposalHandler(db, nc, opaClient, log.Logger)
		r.Mount("/proposals", proposalHandler.Routes())

		// Decision handlers
		decisionHandler := handler.NewDecisionHandler(db, log.Logger)
		r.Mount("/decisions", decisionHandler.Routes())

		// Effect handlers
		effectHandler := handler.NewEffectHandler(db, log.Logger)
		r.Mount("/effects", effectHandler.Routes())

		// Metrics handlers
		metricsHandler := handler.NewMetricsHandler(db, nc, log.Logger)
		r.Mount("/metrics", metricsHandler.Routes())

		// Audit handlers
		auditHandler := handler.NewAuditHandler(db, log.Logger)
		r.Mount("/audit", auditHandler.Routes())

		// Classifier handler
		classifierURL := getEnv("CLASSIFIER_URL", "http://classifier:9090")
		classifierHandler := handler.NewClassifierHandler(classifierURL, log.Logger)
		r.Mount("/classifier", classifierHandler.Routes())

		// Clear all data endpoint
		r.Post("/clear", clearHandler(db))
	})

	return r
}

// correlationIDMiddleware adds a correlation ID to each request
func correlationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		ctx := handler.WithCorrelationID(r.Context(), correlationID)
		w.Header().Set("X-Correlation-ID", correlationID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestLogger logs each HTTP request
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		correlationID := handler.GetCorrelationID(r.Context())

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Int("bytes", ww.BytesWritten()).
			Dur("duration", duration).
			Str("correlation_id", correlationID).
			Str("remote_addr", r.RemoteAddr).
			Msg("HTTP request")
	})
}

// prometheusMiddleware records HTTP metrics
func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = r.URL.Path
		}

		httpRequestsTotal.WithLabelValues(r.Method, path, fmt.Sprintf("%d", ww.Status())).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration.Seconds())
	})
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status        string            `json:"status"`
	Version       string            `json:"version"`
	Uptime        string            `json:"uptime"`
	Components    map[string]string `json:"components"`
	CorrelationID string            `json:"correlation_id"`
}

var startTime = time.Now()

func healthHandler(db *postgres.Pool, nc *nats.Conn, opaClient *opa.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		correlationID := handler.GetCorrelationID(ctx)

		response := HealthResponse{
			Status:        "healthy",
			Version:       "1.0.0",
			Uptime:        time.Since(startTime).Round(time.Second).String(),
			Components:    make(map[string]string),
			CorrelationID: correlationID,
		}

		// Check PostgreSQL
		if err := db.Health(ctx); err != nil {
			response.Components["postgres"] = "unhealthy: " + err.Error()
			response.Status = "degraded"
			dbConnectionStatus.Set(0)
		} else {
			response.Components["postgres"] = "healthy"
			dbConnectionStatus.Set(1)
		}

		// Check NATS
		if nc == nil || !nc.IsConnected() {
			response.Components["nats"] = "disconnected"
			response.Status = "degraded"
			natsConnectionStatus.Set(0)
		} else {
			response.Components["nats"] = "connected"
			natsConnectionStatus.Set(1)
		}

		// Check OPA
		if err := opaClient.Health(ctx); err != nil {
			response.Components["opa"] = "unhealthy: " + err.Error()
			response.Status = "degraded"
		} else {
			response.Components["opa"] = "healthy"
		}

		status := http.StatusOK
		if response.Status != "healthy" {
			status = http.StatusServiceUnavailable
		}

		handler.WriteJSON(w, status, response)
	}
}

// ClearDeletedCounts represents the counts of deleted records per table
type ClearDeletedCounts struct {
	Tracks     int64 `json:"tracks"`
	Proposals  int64 `json:"proposals"`
	Decisions  int64 `json:"decisions"`
	Effects    int64 `json:"effects"`
	Detections int64 `json:"detections"`
}

// ClearResponse represents the response for the clear endpoint
type ClearResponse struct {
	Success       bool               `json:"success"`
	Message       string             `json:"message"`
	Deleted       ClearDeletedCounts `json:"deleted"`
	CorrelationID string             `json:"correlation_id"`
}

// clearHandler handles POST /api/v1/clear to delete all data from the database
func clearHandler(db *postgres.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		correlationID := handler.GetCorrelationID(ctx)

		log.Info().
			Str("correlation_id", correlationID).
			Msg("Clearing all data from database")

		result, err := db.ClearAll(ctx)
		if err != nil {
			log.Error().
				Err(err).
				Str("correlation_id", correlationID).
				Msg("Failed to clear database")

			handler.WriteJSON(w, http.StatusInternalServerError, ClearResponse{
				Success:       false,
				Message:       "Failed to clear data: " + err.Error(),
				CorrelationID: correlationID,
			})
			return
		}

		log.Info().
			Str("correlation_id", correlationID).
			Int64("tracks", result.Tracks).
			Int64("proposals", result.Proposals).
			Int64("decisions", result.Decisions).
			Int64("effects", result.Effects).
			Int64("detections", result.Detections).
			Msg("Successfully cleared all data from database")

		handler.WriteJSON(w, http.StatusOK, ClearResponse{
			Success: true,
			Message: "All data cleared successfully",
			Deleted: ClearDeletedCounts{
				Tracks:     result.Tracks,
				Proposals:  result.Proposals,
				Decisions:  result.Decisions,
				Effects:    result.Effects,
				Detections: result.Detections,
			},
			CorrelationID: correlationID,
		})
	}
}

// maskPassword masks the password in a connection URL for logging
func maskPassword(url string) string {
	// Simple masking - replace password portion
	// This is a basic implementation; a more robust solution would parse the URL properly
	return url // In production, actually mask the password
}

// runTrackPersistenceConsumer subscribes to correlated tracks and persists them to PostgreSQL
func runTrackPersistenceConsumer(ctx context.Context, nc *nats.Conn, db *postgres.Pool) error {
	log.Info().Msg("Starting track persistence consumer")

	// Subscribe to all correlated track subjects (track.correlated.>)
	sub, err := nc.Subscribe("track.correlated.>", func(msg *nats.Msg) {
		var track messages.CorrelatedTrack
		if err := json.Unmarshal(msg.Data, &track); err != nil {
			log.Warn().Err(err).Str("subject", msg.Subject).Msg("Failed to unmarshal correlated track")
			return
		}

		// Persist the track to PostgreSQL
		if err := db.UpsertTrack(ctx, &track); err != nil {
			log.Error().Err(err).
				Str("track_id", track.TrackID).
				Str("subject", msg.Subject).
				Msg("Failed to persist track to database")
			return
		}

		log.Debug().
			Str("track_id", track.TrackID).
			Str("classification", track.Classification).
			Str("threat_level", track.ThreatLevel).
			Msg("Persisted correlated track to database")
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to track.correlated.>: %w", err)
	}

	log.Info().Str("subject", "track.correlated.>").Msg("Subscribed to correlated tracks for persistence")

	// Wait for context cancellation
	<-ctx.Done()

	// Unsubscribe
	if err := sub.Unsubscribe(); err != nil {
		log.Warn().Err(err).Msg("Failed to unsubscribe from track subject")
	}

	log.Info().Msg("Track persistence consumer stopped")
	return nil
}
