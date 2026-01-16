package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

// BaseAgent provides common functionality for all agents
type BaseAgent struct {
	id        string
	agentType AgentType
	config    Config

	// NATS
	nc *nats.Conn
	js jetstream.JetStream

	// Logging
	logger zerolog.Logger

	// Metrics
	registry        *prometheus.Registry
	messagesTotal   *prometheus.CounterVec
	latencyHist     *prometheus.HistogramVec
	errorsTotal     *prometheus.CounterVec

	// State
	running bool
	mu      sync.RWMutex
	cancel  context.CancelFunc
}

// NewBaseAgent creates a new base agent with common setup
func NewBaseAgent(cfg Config) (*BaseAgent, error) {
	// Set up logger
	logger := zerolog.New(os.Stdout).With().
		Timestamp().
		Str("agent_id", cfg.ID).
		Str("agent_type", string(cfg.Type)).
		Logger()

	// Create metrics registry
	registry := prometheus.NewRegistry()

	messagesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_messages_total",
			Help: "Total messages processed by agent",
		},
		[]string{"status", "message_type"},
	)

	latencyHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agent_processing_latency_seconds",
			Help:    "Message processing latency in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"message_type"},
	)

	errorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_errors_total",
			Help: "Total errors encountered by agent",
		},
		[]string{"error_type"},
	)

	registry.MustRegister(messagesTotal, latencyHist, errorsTotal)

	agent := &BaseAgent{
		id:            cfg.ID,
		agentType:     cfg.Type,
		config:        cfg,
		logger:        logger,
		registry:      registry,
		messagesTotal: messagesTotal,
		latencyHist:   latencyHist,
		errorsTotal:   errorsTotal,
	}

	return agent, nil
}

// ID returns the agent ID
func (a *BaseAgent) ID() string {
	return a.id
}

// Type returns the agent type
func (a *BaseAgent) Type() AgentType {
	return a.agentType
}

// Config returns the agent configuration
func (a *BaseAgent) Config() Config {
	return a.config
}

// Logger returns the agent logger
func (a *BaseAgent) Logger() *zerolog.Logger {
	return &a.logger
}

// NATS returns the NATS connection
func (a *BaseAgent) NATS() *nats.Conn {
	return a.nc
}

// JetStream returns the JetStream context
func (a *BaseAgent) JetStream() jetstream.JetStream {
	return a.js
}

// Metrics returns the Prometheus registry
func (a *BaseAgent) Metrics() *prometheus.Registry {
	return a.registry
}

// RecordMessage records a processed message metric
func (a *BaseAgent) RecordMessage(status, msgType string) {
	a.messagesTotal.WithLabelValues(status, msgType).Inc()
}

// RecordLatency records processing latency
func (a *BaseAgent) RecordLatency(msgType string, duration time.Duration) {
	a.latencyHist.WithLabelValues(msgType).Observe(duration.Seconds())
}

// RecordError records an error metric
func (a *BaseAgent) RecordError(errorType string) {
	a.errorsTotal.WithLabelValues(errorType).Inc()
}

// Connect establishes NATS connection
func (a *BaseAgent) Connect(ctx context.Context) error {
	a.logger.Info().Str("url", a.config.NATSUrl).Msg("Connecting to NATS")

	// Get credentials for this agent type
	user, pass := a.getNATSCredentials()

	opts := []nats.Option{
		nats.Name(a.id),
		nats.UserInfo(user, pass),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			a.logger.Warn().Err(err).Msg("NATS disconnected")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			a.logger.Info().Msg("NATS reconnected")
		}),
	}

	nc, err := nats.Connect(a.config.NATSUrl, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	a.nc = nc

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	a.js = js
	a.logger.Info().Msg("Connected to NATS with JetStream")

	return nil
}

// getNATSCredentials returns the credentials for this agent type
func (a *BaseAgent) getNATSCredentials() (string, string) {
	// In production, these would come from secrets management
	credentials := map[AgentType]struct{ user, pass string }{
		AgentTypeSensor:     {"sensor", "sensor-secret"},
		AgentTypeClassifier: {"classifier", "classifier-secret"},
		AgentTypeCorrelator: {"correlator", "correlator-secret"},
		AgentTypePlanner:    {"planner", "planner-secret"},
		AgentTypeAuthorizer: {"authorizer", "authorizer-secret"},
		AgentTypeEffector:   {"effector", "effector-secret"},
	}

	if creds, ok := credentials[a.agentType]; ok {
		return creds.user, creds.pass
	}
	return "admin", "admin-secret"
}

// Health returns the health status
func (a *BaseAgent) Health() HealthStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.running {
		return HealthStatus{Healthy: false, Status: "stopped"}
	}

	if a.nc == nil || !a.nc.IsConnected() {
		return HealthStatus{Healthy: false, Status: "disconnected", Details: "NATS connection lost"}
	}

	return HealthStatus{Healthy: true, Status: "running"}
}

// Start begins the agent lifecycle
func (a *BaseAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	a.running = true

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.mu.Unlock()

	// Connect to NATS
	if err := a.Connect(ctx); err != nil {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		return err
	}

	a.logger.Info().Msg("Agent started")
	return nil
}

// Stop gracefully stops the agent
func (a *BaseAgent) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	a.logger.Info().Msg("Stopping agent")

	if a.cancel != nil {
		a.cancel()
	}

	if a.nc != nil {
		a.nc.Close()
	}

	a.running = false
	a.logger.Info().Msg("Agent stopped")
	return nil
}

// EnsureStream creates a stream if it doesn't exist
func (a *BaseAgent) EnsureStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	stream, err := a.js.Stream(ctx, cfg.Name)
	if err == nil {
		return stream, nil
	}

	// Create the stream
	stream, err = a.js.CreateStream(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream %s: %w", cfg.Name, err)
	}

	a.logger.Info().Str("stream", cfg.Name).Msg("Created stream")
	return stream, nil
}

// EnsureConsumer creates a consumer if it doesn't exist
func (a *BaseAgent) EnsureConsumer(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	s, err := a.js.Stream(ctx, stream)
	if err != nil {
		return nil, fmt.Errorf("stream %s not found: %w", stream, err)
	}

	consumer, err := s.Consumer(ctx, cfg.Durable)
	if err == nil {
		return consumer, nil
	}

	// Create the consumer
	consumer, err = s.CreateConsumer(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer %s: %w", cfg.Durable, err)
	}

	a.logger.Info().Str("consumer", cfg.Durable).Str("stream", stream).Msg("Created consumer")
	return consumer, nil
}
