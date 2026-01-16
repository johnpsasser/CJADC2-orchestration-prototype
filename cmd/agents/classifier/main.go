// Classifier Agent - Enriches detections with classification and track type
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// ClassifierAgent processes raw detections and enriches them with classification
type ClassifierAgent struct {
	*agent.BaseAgent
	logger   zerolog.Logger
	consumer jetstream.Consumer
}

// NewClassifierAgent creates a new classifier agent
func NewClassifierAgent(cfg agent.Config) (*ClassifierAgent, error) {
	base, err := agent.NewBaseAgent(cfg)
	if err != nil {
		return nil, err
	}

	return &ClassifierAgent{
		BaseAgent: base,
		logger:    *base.Logger(),
	}, nil
}

// Run starts the classifier agent
func (a *ClassifierAgent) Run(ctx context.Context) error {
	// Start base agent (connects to NATS)
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base agent: %w", err)
	}

	// Ensure streams exist
	if err := natsutil.SetupStreams(ctx, a.JetStream()); err != nil {
		return fmt.Errorf("failed to setup streams: %w", err)
	}

	// Create consumer for detection events
	consumer, err := natsutil.SetupConsumer(ctx, a.JetStream(), "DETECTIONS", "classifier")
	if err != nil {
		return fmt.Errorf("failed to setup consumer: %w", err)
	}
	a.consumer = consumer

	a.logger.Info().Msg("Classifier agent started, consuming from DETECTIONS stream")

	// Start consuming messages
	return a.consumeMessages(ctx)
}

// consumeMessages processes detection messages
func (a *ClassifierAgent) consumeMessages(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch messages with timeout
		msgs, err := a.consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if err == context.DeadlineExceeded || err == context.Canceled {
				continue
			}
			a.logger.Error().Err(err).Msg("Failed to fetch messages")
			a.RecordError("fetch_error")
			time.Sleep(time.Second)
			continue
		}

		for msg := range msgs.Messages() {
			if err := a.processMessage(ctx, msg); err != nil {
				a.logger.Error().Err(err).Msg("Failed to process message")
				a.RecordError("process_error")
				msg.Nak()
			} else {
				msg.Ack()
			}
		}

		if msgs.Error() != nil && msgs.Error() != context.DeadlineExceeded {
			a.logger.Warn().Err(msgs.Error()).Msg("Message batch error")
		}
	}
}

// processMessage handles a single detection message
func (a *ClassifierAgent) processMessage(ctx context.Context, msg jetstream.Msg) error {
	start := time.Now()

	// Parse detection
	var detection messages.Detection
	if err := json.Unmarshal(msg.Data(), &detection); err != nil {
		return fmt.Errorf("failed to unmarshal detection: %w", err)
	}

	correlationID := detection.Envelope.CorrelationID
	if correlationID == "" {
		correlationID = detection.Envelope.MessageID
	}

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("track_id", detection.TrackID).
		Str("sensor_type", detection.SensorType).
		Float64("confidence", detection.Confidence).
		Msg("Processing detection")

	// Create track from detection
	track := messages.NewTrack(&detection, a.ID())

	// Enrich with correlation ID chain
	if detection.Envelope.CorrelationID == "" {
		track.Envelope.CorrelationID = detection.Envelope.MessageID
	} else {
		track.Envelope.CorrelationID = detection.Envelope.CorrelationID
	}

	// Classify the track
	a.classify(track, &detection)

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("track_id", track.TrackID).
		Str("classification", track.Classification).
		Str("type", track.Type).
		Msg("Track classified")

	// Publish to TRACKS stream
	subject := track.Subject()
	data, err := json.Marshal(track)
	if err != nil {
		return fmt.Errorf("failed to marshal track: %w", err)
	}

	_, err = a.JetStream().Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish track: %w", err)
	}

	duration := time.Since(start)
	a.RecordMessage("success", "detection")
	a.RecordLatency("detection", duration)

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("subject", subject).
		Dur("latency_ms", duration).
		Msg("Published classified track")

	return nil
}

// classify determines the classification and type of a track
func (a *ClassifierAgent) classify(track *messages.Track, detection *messages.Detection) {
	// Determine track type based on sensor type and characteristics
	track.Type = a.determineTrackType(detection)

	// Determine classification based on various factors
	track.Classification = a.determineClassification(detection, track.Type)

	// Adjust confidence based on classification certainty
	track.Confidence = a.adjustConfidence(detection.Confidence, track.Classification)
}

// determineTrackType infers the type of track from detection characteristics
func (a *ClassifierAgent) determineTrackType(detection *messages.Detection) string {
	// If the sensor provided a track type hint, use it (trusted sensor data)
	if detection.Type != "" {
		return detection.Type
	}

	// Fallback to heuristics if no type provided
	speed := detection.Velocity.Speed
	alt := detection.Position.Alt

	// Simple heuristics for track type classification
	switch {
	case alt > 10000 && speed > 200:
		return "aircraft"
	case alt > 1000 && speed > 500:
		return "missile"
	case alt < 100 && speed > 0 && speed < 50:
		// Could be ground or vessel based on position
		if a.isOverWater(detection.Position) {
			return "vessel"
		}
		return "ground"
	case alt < 5000 && speed > 50 && speed < 300:
		return "aircraft"
	case speed == 0:
		return "ground"
	default:
		return "unknown"
	}
}

// isOverWater is a simplified check for maritime classification
func (a *ClassifierAgent) isOverWater(pos messages.Position) bool {
	// Simplified: use longitude ranges to approximate ocean areas
	// In production, this would use proper GIS data
	return pos.Lon < -100 || pos.Lon > 100 || (pos.Lon > -50 && pos.Lon < 50 && pos.Lat < 0)
}

// determineClassification determines if a track is friendly, hostile, unknown, or neutral
func (a *ClassifierAgent) determineClassification(detection *messages.Detection, trackType string) string {
	// Simplified classification logic
	// In production, this would use IFF data, known track databases, etc.

	confidence := detection.Confidence

	// Check for known neutral tracks first (commercial/civilian)
	if a.isNeutralTrack(detection) {
		return "neutral"
	}

	// Check for IFF-confirmed friendly tracks
	if a.simulateIFFCheck(detection) {
		return "friendly"
	}

	// Check against known hostile patterns
	if a.checkHostilePatterns(detection, trackType) {
		return "hostile"
	}

	// High confidence detections without matches are neutral
	if confidence > 0.85 {
		return "neutral"
	}

	// Medium confidence - unknown
	return "unknown"
}

// simulateIFFCheck simulates an IFF (Identification Friend or Foe) check
func (a *ClassifierAgent) simulateIFFCheck(detection *messages.Detection) bool {
	// In production, this would query actual IFF systems
	// For simulation, track IDs starting with 'F' are friendly
	hash := detection.TrackID
	if len(hash) > 0 && hash[0] == 'F' {
		return true
	}
	return false
}

// isNeutralTrack checks if the track is from a known neutral entity
func (a *ClassifierAgent) isNeutralTrack(detection *messages.Detection) bool {
	// Track IDs starting with 'N' are neutral (commercial/civilian)
	if len(detection.TrackID) > 0 && detection.TrackID[0] == 'N' {
		return true
	}
	return false
}

// checkHostilePatterns checks if the detection matches known hostile patterns
func (a *ClassifierAgent) checkHostilePatterns(detection *messages.Detection, trackType string) bool {
	// Simplified pattern matching
	// In production, this would use ML models and threat databases

	// High-speed missiles are assumed hostile unless identified
	if trackType == "missile" && detection.Velocity.Speed > 500 {
		return true
	}

	// Tracks with specific ID patterns (simulation)
	if len(detection.TrackID) > 0 && detection.TrackID[0] == 'H' {
		return true
	}

	return false
}

// adjustConfidence adjusts the confidence based on classification certainty
func (a *ClassifierAgent) adjustConfidence(originalConfidence float64, classification string) float64 {
	switch classification {
	case "friendly":
		// IFF confirmed - boost confidence
		return min(1.0, originalConfidence*1.1)
	case "hostile":
		// Pattern matched - slight reduction for uncertainty
		return originalConfidence * 0.95
	case "neutral":
		return originalConfidence
	default:
		// Unknown - reduce confidence
		return originalConfidence * 0.8
	}
}

func main() {
	// Configuration from environment
	cfg := agent.Config{
		ID:      getEnv("AGENT_ID", "classifier-"+uuid.New().String()[:8]),
		Type:    agent.AgentTypeClassifier,
		NATSUrl: getEnv("NATS_URL", "nats://localhost:4222"),
		OPAUrl:  getEnv("OPA_URL", "http://localhost:8181"),
		Secret:  []byte(getEnv("AGENT_SECRET", "classifier-secret")),
	}

	// Create agent
	classifier, err := NewClassifierAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create classifier agent: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start metrics server
	go func() {
		metricsAddr := getEnv("METRICS_ADDR", ":9090")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(classifier.Metrics(), promhttp.HandlerOpts{}))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			health := classifier.Health()
			if health.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			json.NewEncoder(w).Encode(health)
		})
		classifier.logger.Info().Str("addr", metricsAddr).Msg("Starting metrics server")
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			classifier.logger.Error().Err(err).Msg("Metrics server error")
		}
	}()

	// Run agent
	go func() {
		if err := classifier.Run(ctx); err != nil && err != context.Canceled {
			classifier.logger.Error().Err(err).Msg("Classifier agent error")
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	classifier.logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := classifier.Stop(shutdownCtx); err != nil {
		classifier.logger.Error().Err(err).Msg("Error during shutdown")
	}

	classifier.logger.Info().Msg("Classifier agent stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Ensure string matching for sensor type is handled properly
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
