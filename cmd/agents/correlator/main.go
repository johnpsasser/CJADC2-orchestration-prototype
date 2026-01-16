// Correlator Agent - Correlates tracks with sliding window deduplication and threat assessment
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

const (
	// WindowDuration is the sliding window duration for track correlation
	WindowDuration = 10 * time.Second
	// CleanupInterval is how often to clean expired tracks from the window
	CleanupInterval = 5 * time.Second
	// PositionThresholdMeters is the max distance to consider tracks as the same entity
	PositionThresholdMeters = 500.0
)

// TrackWindow holds tracks within the correlation window
type TrackWindow struct {
	mu     sync.RWMutex
	tracks map[string]*trackEntry
}

type trackEntry struct {
	track     *messages.Track
	expiresAt time.Time
	merged    bool
}

// CorrelatorAgent correlates and deduplicates tracks
type CorrelatorAgent struct {
	*agent.BaseAgent
	logger          zerolog.Logger
	consumer        jetstream.Consumer
	window          *TrackWindow
	correlatedGauge prometheus.Gauge
	mergedCounter   prometheus.Counter
}

// NewCorrelatorAgent creates a new correlator agent
func NewCorrelatorAgent(cfg agent.Config) (*CorrelatorAgent, error) {
	base, err := agent.NewBaseAgent(cfg)
	if err != nil {
		return nil, err
	}

	// Additional metrics for correlation
	correlatedGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "correlator_window_tracks",
		Help: "Number of tracks in correlation window",
	})

	mergedCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "correlator_tracks_merged_total",
		Help: "Total number of tracks merged",
	})

	base.Metrics().MustRegister(correlatedGauge, mergedCounter)

	return &CorrelatorAgent{
		BaseAgent:       base,
		logger:          *base.Logger(),
		window:          &TrackWindow{tracks: make(map[string]*trackEntry)},
		correlatedGauge: correlatedGauge,
		mergedCounter:   mergedCounter,
	}, nil
}

// Run starts the correlator agent
func (a *CorrelatorAgent) Run(ctx context.Context) error {
	// Start base agent (connects to NATS)
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base agent: %w", err)
	}

	// Ensure streams exist
	if err := natsutil.SetupStreams(ctx, a.JetStream()); err != nil {
		return fmt.Errorf("failed to setup streams: %w", err)
	}

	// Create consumer for classified tracks
	consumer, err := natsutil.SetupConsumer(ctx, a.JetStream(), "TRACKS", "correlator")
	if err != nil {
		return fmt.Errorf("failed to setup consumer: %w", err)
	}
	a.consumer = consumer

	// Start window cleanup goroutine
	go a.cleanupLoop(ctx)

	a.logger.Info().Msg("Correlator agent started, consuming from TRACKS stream")

	// Start consuming messages
	return a.consumeMessages(ctx)
}

// cleanupLoop periodically removes expired tracks from the window
func (a *CorrelatorAgent) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.cleanupWindow()
		}
	}
}

// cleanupWindow removes expired tracks
func (a *CorrelatorAgent) cleanupWindow() {
	a.window.mu.Lock()
	defer a.window.mu.Unlock()

	now := time.Now()
	for id, entry := range a.window.tracks {
		if now.After(entry.expiresAt) {
			delete(a.window.tracks, id)
		}
	}

	a.correlatedGauge.Set(float64(len(a.window.tracks)))
}

// consumeMessages processes track messages
func (a *CorrelatorAgent) consumeMessages(ctx context.Context) error {
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

// processMessage handles a single track message
func (a *CorrelatorAgent) processMessage(ctx context.Context, msg jetstream.Msg) error {
	start := time.Now()

	// Parse track
	var track messages.Track
	if err := json.Unmarshal(msg.Data(), &track); err != nil {
		return fmt.Errorf("failed to unmarshal track: %w", err)
	}

	correlationID := track.Envelope.CorrelationID
	if correlationID == "" {
		correlationID = track.Envelope.MessageID
	}

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("track_id", track.TrackID).
		Str("classification", track.Classification).
		Msg("Processing classified track")

	// Correlate with existing tracks
	correlatedTrack, mergedTrackIDs := a.correlate(&track)

	// Determine threat level
	correlatedTrack.ThreatLevel = a.determineThreatLevel(correlatedTrack)

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("track_id", correlatedTrack.TrackID).
		Str("threat_level", correlatedTrack.ThreatLevel).
		Int("merged_count", len(mergedTrackIDs)).
		Msg("Track correlated")

	// Publish to TRACKS stream with threat level
	subject := correlatedTrack.Subject()
	data, err := json.Marshal(correlatedTrack)
	if err != nil {
		return fmt.Errorf("failed to marshal correlated track: %w", err)
	}

	_, err = a.JetStream().Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish correlated track: %w", err)
	}

	duration := time.Since(start)
	a.RecordMessage("success", "track")
	a.RecordLatency("track", duration)

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("subject", subject).
		Dur("latency_ms", duration).
		Msg("Published correlated track")

	return nil
}

// correlate finds and merges related tracks within the window
func (a *CorrelatorAgent) correlate(track *messages.Track) (*messages.CorrelatedTrack, []string) {
	a.window.mu.Lock()
	defer a.window.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-WindowDuration)
	mergedTrackIDs := []string{}
	mergedEntries := []*trackEntry{}

	// Find tracks that should be merged
	for id, entry := range a.window.tracks {
		if entry.merged {
			continue
		}

		// Check if tracks are within spatial threshold and same classification
		if a.shouldMerge(track, entry.track) {
			mergedTrackIDs = append(mergedTrackIDs, id)
			mergedEntries = append(mergedEntries, entry)
			entry.merged = true
			a.mergedCounter.Inc()
		}
	}

	// Create correlated track
	correlatedTrack := messages.NewCorrelatedTrack(track, a.ID())
	correlatedTrack.WindowStart = windowStart
	correlatedTrack.WindowEnd = now

	// Merge data from related tracks
	if len(mergedEntries) > 0 {
		correlatedTrack.MergedFrom = append([]string{track.TrackID}, mergedTrackIDs...)

		// Aggregate data from merged tracks
		for _, entry := range mergedEntries {
			correlatedTrack.DetectionCount += entry.track.DetectionCount
			correlatedTrack.Sources = a.mergeSources(correlatedTrack.Sources, entry.track.Sources)

			// Use weighted position averaging
			correlatedTrack.Position = a.averagePosition(correlatedTrack.Position, entry.track.Position)

			// Average velocities
			correlatedTrack.Velocity = a.averageVelocity(correlatedTrack.Velocity, entry.track.Velocity)

			// Boost confidence when tracks correlate
			correlatedTrack.Confidence = min(1.0, correlatedTrack.Confidence+0.05)
		}
	}

	// Add current track to window
	a.window.tracks[track.TrackID] = &trackEntry{
		track:     track,
		expiresAt: now.Add(WindowDuration),
		merged:    false,
	}

	a.correlatedGauge.Set(float64(len(a.window.tracks)))

	return correlatedTrack, mergedTrackIDs
}

// shouldMerge determines if two tracks should be merged
func (a *CorrelatorAgent) shouldMerge(t1 *messages.Track, t2 *messages.Track) bool {
	// Same track ID is definitely a match
	if t1.TrackID == t2.TrackID {
		return true
	}

	// Must be same classification
	if t1.Classification != t2.Classification {
		return false
	}

	// Must be same type
	if t1.Type != t2.Type {
		return false
	}

	// Check spatial proximity
	distance := a.haversineDistance(t1.Position, t2.Position)
	if distance > PositionThresholdMeters {
		return false
	}

	// Check velocity similarity (within 20%)
	speedDiff := math.Abs(t1.Velocity.Speed - t2.Velocity.Speed)
	avgSpeed := (t1.Velocity.Speed + t2.Velocity.Speed) / 2
	if avgSpeed > 0 && speedDiff/avgSpeed > 0.2 {
		return false
	}

	return true
}

// haversineDistance calculates distance between two positions in meters
func (a *CorrelatorAgent) haversineDistance(p1, p2 messages.Position) float64 {
	const earthRadius = 6371000 // meters

	lat1 := p1.Lat * math.Pi / 180
	lat2 := p2.Lat * math.Pi / 180
	dLat := (p2.Lat - p1.Lat) * math.Pi / 180
	dLon := (p2.Lon - p1.Lon) * math.Pi / 180

	a1 := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a1), math.Sqrt(1-a1))

	return earthRadius * c
}

// averagePosition computes average position
func (a *CorrelatorAgent) averagePosition(p1, p2 messages.Position) messages.Position {
	return messages.Position{
		Lat: (p1.Lat + p2.Lat) / 2,
		Lon: (p1.Lon + p2.Lon) / 2,
		Alt: (p1.Alt + p2.Alt) / 2,
	}
}

// averageVelocity computes average velocity
func (a *CorrelatorAgent) averageVelocity(v1, v2 messages.Velocity) messages.Velocity {
	return messages.Velocity{
		Speed:   (v1.Speed + v2.Speed) / 2,
		Heading: a.averageHeading(v1.Heading, v2.Heading),
	}
}

// averageHeading handles circular averaging of headings
func (a *CorrelatorAgent) averageHeading(h1, h2 float64) float64 {
	// Convert to radians
	r1 := h1 * math.Pi / 180
	r2 := h2 * math.Pi / 180

	// Average using vector components
	x := (math.Cos(r1) + math.Cos(r2)) / 2
	y := (math.Sin(r1) + math.Sin(r2)) / 2

	// Convert back to degrees
	avg := math.Atan2(y, x) * 180 / math.Pi
	if avg < 0 {
		avg += 360
	}
	return avg
}

// mergeSources combines source lists without duplicates
func (a *CorrelatorAgent) mergeSources(s1, s2 []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, s := range s1 {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range s2 {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// determineThreatLevel assigns threat level based on track characteristics
func (a *CorrelatorAgent) determineThreatLevel(ct *messages.CorrelatedTrack) string {
	// Critical: Hostile missiles or aircraft approaching at high speed
	if ct.Classification == "hostile" {
		if ct.Type == "missile" {
			return "critical"
		}
		if ct.Type == "aircraft" && ct.Velocity.Speed > 300 {
			return "high"
		}
		return "medium"
	}

	// Unknown tracks with high speed are concerning
	if ct.Classification == "unknown" {
		if ct.Velocity.Speed > 500 {
			return "high"
		}
		if ct.Velocity.Speed > 200 {
			return "medium"
		}
		return "low"
	}

	// Neutral tracks are low threat
	if ct.Classification == "neutral" {
		return "low"
	}

	// Friendly tracks are low threat (but still tracked)
	if ct.Classification == "friendly" {
		return "low"
	}

	return "low"
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func main() {
	// Configuration from environment
	cfg := agent.Config{
		ID:      getEnv("AGENT_ID", "correlator-"+uuid.New().String()[:8]),
		Type:    agent.AgentTypeCorrelator,
		NATSUrl: getEnv("NATS_URL", "nats://localhost:4222"),
		OPAUrl:  getEnv("OPA_URL", "http://localhost:8181"),
		Secret:  []byte(getEnv("AGENT_SECRET", "correlator-secret")),
	}

	// Create agent
	correlator, err := NewCorrelatorAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create correlator agent: %v\n", err)
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
		mux.Handle("/metrics", promhttp.HandlerFor(correlator.Metrics(), promhttp.HandlerOpts{}))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			health := correlator.Health()
			if health.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			json.NewEncoder(w).Encode(health)
		})
		correlator.logger.Info().Str("addr", metricsAddr).Msg("Starting metrics server")
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			correlator.logger.Error().Err(err).Msg("Metrics server error")
		}
	}()

	// Run agent
	go func() {
		if err := correlator.Run(ctx); err != nil && err != context.Canceled {
			correlator.logger.Error().Err(err).Msg("Correlator agent error")
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	correlator.logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := correlator.Stop(shutdownCtx); err != nil {
		correlator.logger.Error().Err(err).Msg("Error during shutdown")
	}

	correlator.logger.Info().Msg("Correlator agent stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
