// Sensor Simulator Agent
// Generates synthetic detection events simulating radar/sensor data
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
	"github.com/agile-defense/cjadc2/pkg/postgres"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Configuration limits
const (
	MinEmissionInterval = 100 * time.Millisecond
	MaxEmissionInterval = 10 * time.Second
	MinTrackCount       = 1
	MaxTrackCount       = 100

	DefaultEmissionInterval = 500 * time.Millisecond
	DefaultTrackCount       = 10
)

// Default type weights (must sum to 100 for percentage-based selection)
var DefaultTypeWeights = map[string]int{
	"aircraft": 40,
	"vessel":   20,
	"ground":   15,
	"missile":  5,
	"unknown":  20,
}

// Default classification weights (must sum to 100 for percentage-based selection)
var DefaultClassificationWeights = map[string]int{
	"friendly": 30,
	"hostile":  25,
	"neutral":  20,
	"unknown":  25,
}

// Missile-specific classification weights (90% hostile, 10% unknown)
var MissileClassificationWeights = map[string]int{
	"friendly": 0,
	"hostile":  90,
	"neutral":  0,
	"unknown":  10,
}

// SensorConfig holds the runtime configuration for the sensor agent
type SensorConfig struct {
	mu sync.RWMutex

	emissionInterval      time.Duration
	trackCount            int
	paused                bool
	typeWeights           map[string]int
	classificationWeights map[string]int
}

// NewSensorConfig creates a new SensorConfig with default values
func NewSensorConfig() *SensorConfig {
	return &SensorConfig{
		emissionInterval:      DefaultEmissionInterval,
		trackCount:            DefaultTrackCount,
		paused:                false,
		typeWeights:           copyWeights(DefaultTypeWeights),
		classificationWeights: copyWeights(DefaultClassificationWeights),
	}
}

// copyWeights creates a copy of a weights map
func copyWeights(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// GetEmissionInterval returns the current emission interval
func (c *SensorConfig) GetEmissionInterval() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.emissionInterval
}

// SetEmissionInterval sets the emission interval with validation
func (c *SensorConfig) SetEmissionInterval(d time.Duration) error {
	if d < MinEmissionInterval || d > MaxEmissionInterval {
		return fmt.Errorf("emission_interval must be between %v and %v", MinEmissionInterval, MaxEmissionInterval)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emissionInterval = d
	return nil
}

// GetTrackCount returns the current track count
func (c *SensorConfig) GetTrackCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trackCount
}

// SetTrackCount sets the track count with validation
func (c *SensorConfig) SetTrackCount(count int) error {
	if count < MinTrackCount || count > MaxTrackCount {
		return fmt.Errorf("track_count must be between %d and %d", MinTrackCount, MaxTrackCount)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trackCount = count
	return nil
}

// IsPaused returns whether emission is paused
func (c *SensorConfig) IsPaused() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.paused
}

// SetPaused sets the paused state
func (c *SensorConfig) SetPaused(paused bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = paused
}

// GetTypeWeights returns a copy of the current type weights
func (c *SensorConfig) GetTypeWeights() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copyWeights(c.typeWeights)
}

// SetTypeWeights sets the type weights with validation
func (c *SensorConfig) SetTypeWeights(weights map[string]int) error {
	// Validate keys are valid track types
	validTypes := map[string]bool{"aircraft": true, "vessel": true, "ground": true, "missile": true, "unknown": true}
	for key := range weights {
		if !validTypes[key] {
			return fmt.Errorf("invalid track type: %s (valid types: aircraft, vessel, ground, missile, unknown)", key)
		}
	}
	// Validate weights are non-negative
	for key, weight := range weights {
		if weight < 0 {
			return fmt.Errorf("weight for %s cannot be negative", key)
		}
	}
	// Validate at least one weight is positive
	total := 0
	for _, weight := range weights {
		total += weight
	}
	if total == 0 {
		return fmt.Errorf("at least one type weight must be positive")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.typeWeights = copyWeights(weights)
	return nil
}

// GetClassificationWeights returns a copy of the current classification weights
func (c *SensorConfig) GetClassificationWeights() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copyWeights(c.classificationWeights)
}

// SetClassificationWeights sets the classification weights with validation
func (c *SensorConfig) SetClassificationWeights(weights map[string]int) error {
	// Validate keys are valid classifications
	validClassifications := map[string]bool{"friendly": true, "hostile": true, "neutral": true, "unknown": true}
	for key := range weights {
		if !validClassifications[key] {
			return fmt.Errorf("invalid classification: %s (valid: friendly, hostile, neutral, unknown)", key)
		}
	}
	// Validate weights are non-negative
	for key, weight := range weights {
		if weight < 0 {
			return fmt.Errorf("weight for %s cannot be negative", key)
		}
	}
	// Validate at least one weight is positive
	total := 0
	for _, weight := range weights {
		total += weight
	}
	if total == 0 {
		return fmt.Errorf("at least one classification weight must be positive")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.classificationWeights = copyWeights(weights)
	return nil
}

// Reset resets configuration to default values
func (c *SensorConfig) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emissionInterval = DefaultEmissionInterval
	c.trackCount = DefaultTrackCount
	c.paused = false
	c.typeWeights = copyWeights(DefaultTypeWeights)
	c.classificationWeights = copyWeights(DefaultClassificationWeights)
}

// Snapshot returns a copy of the current configuration
func (c *SensorConfig) Snapshot() (emissionInterval time.Duration, trackCount int, paused bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.emissionInterval, c.trackCount, c.paused
}

// FullSnapshot returns a complete copy of the current configuration including weights
func (c *SensorConfig) FullSnapshot() (emissionInterval time.Duration, trackCount int, paused bool, typeWeights, classificationWeights map[string]int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.emissionInterval, c.trackCount, c.paused, copyWeights(c.typeWeights), copyWeights(c.classificationWeights)
}

// ConfigResponse represents the JSON response for configuration
type ConfigResponse struct {
	EmissionIntervalMS    int64          `json:"emission_interval_ms"`
	TrackCount            int            `json:"track_count"`
	Paused                bool           `json:"paused"`
	TypeWeights           map[string]int `json:"type_weights"`
	ClassificationWeights map[string]int `json:"classification_weights"`
}

// ConfigUpdateRequest represents a partial configuration update request
type ConfigUpdateRequest struct {
	EmissionIntervalMS    *int64          `json:"emission_interval_ms,omitempty"`
	TrackCount            *int            `json:"track_count,omitempty"`
	Paused                *bool           `json:"paused,omitempty"`
	TypeWeights           *map[string]int `json:"type_weights,omitempty"`
	ClassificationWeights *map[string]int `json:"classification_weights,omitempty"`
	ClearStreams          *bool           `json:"clear_streams,omitempty"` // Action: purge NATS streams when true
}

// SensorAgent generates synthetic detection events
type SensorAgent struct {
	*agent.BaseAgent

	// Thread-safe configuration
	config *SensorConfig

	// Database connection (optional)
	db *postgres.Pool

	// Simulated tracks
	tracksMu sync.RWMutex
	tracks   map[string]*simulatedTrack
}

type simulatedTrack struct {
	id         string
	position   messages.Position
	velocity   messages.Velocity
	confidence float64
	trackType  string
}

func main() {
	cfg := agent.Config{
		ID:      getEnv("AGENT_ID", "sensor-001"),
		Type:    agent.AgentTypeSensor,
		NATSUrl: getEnv("NATS_URL", "nats://localhost:4222"),
		OPAUrl:  getEnv("OPA_URL", "http://localhost:8181"),
		Secret:  []byte(getEnv("SIGNING_SECRET", "dev-secret")),
	}

	sensor, err := NewSensorAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create sensor agent: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database connection (optional - sensor continues without it)
	postgresURL := getEnv("POSTGRES_URL", "postgres://cjadc2:cjadc2@localhost:5432/cjadc2?sslmode=disable")
	dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := postgres.NewPoolFromURL(dbCtx, postgresURL)
	dbCancel()
	if err != nil {
		sensor.Logger().Warn().Err(err).Msg("Failed to connect to PostgreSQL, counter tracking disabled")
	} else {
		sensor.db = db
		sensor.Logger().Info().Msg("Connected to PostgreSQL for counter tracking")
	}

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		sensor.Logger().Info().Msg("Shutdown signal received")
		cancel()
	}()

	// Start HTTP server with chi router
	go sensor.startHTTPServer()

	if err := sensor.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start sensor agent: %v\n", err)
		os.Exit(1)
	}

	if err := sensor.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Sensor agent error: %v\n", err)
		os.Exit(1)
	}

	// Close database connection on shutdown
	if sensor.db != nil {
		sensor.db.Close()
	}

	sensor.Stop(context.Background())
}

// NewSensorAgent creates a new sensor simulator agent
func NewSensorAgent(cfg agent.Config) (*SensorAgent, error) {
	base, err := agent.NewBaseAgent(cfg)
	if err != nil {
		return nil, err
	}

	config := NewSensorConfig()

	// Override defaults from environment
	if intervalStr := os.Getenv("EMISSION_INTERVAL"); intervalStr != "" {
		if interval, err := time.ParseDuration(intervalStr); err == nil {
			if err := config.SetEmissionInterval(interval); err != nil {
				// Use default if invalid
				base.Logger().Warn().Err(err).Msg("Invalid EMISSION_INTERVAL, using default")
			}
		}
	}

	if countStr := os.Getenv("TRACK_COUNT"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil {
			if err := config.SetTrackCount(count); err != nil {
				// Use default if invalid
				base.Logger().Warn().Err(err).Msg("Invalid TRACK_COUNT, using default")
			}
		}
	}

	sensor := &SensorAgent{
		BaseAgent: base,
		config:    config,
		tracks:    make(map[string]*simulatedTrack),
	}

	// Initialize simulated tracks
	sensor.initializeTracks(config.GetTrackCount())

	return sensor, nil
}

// startHTTPServer starts the HTTP server with chi router
func (s *SensorAgent) startHTTPServer() {
	r := chi.NewRouter()

	// Add CORS middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Correlation-ID"},
		ExposedHeaders:   []string{"X-Correlation-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Metrics endpoint
	r.Handle("/metrics", promhttp.HandlerFor(s.Metrics(), promhttp.HandlerOpts{}))

	// Health endpoint
	r.Get("/health", s.handleHealth)

	// Configuration endpoints
	r.Route("/api/v1/config", func(r chi.Router) {
		r.Get("/", s.handleGetConfig)
		r.Patch("/", s.handlePatchConfig)
		r.Post("/reset", s.handleResetConfig)
	})

	s.Logger().Info().Msg("Starting HTTP server on :9090")
	if err := http.ListenAndServe(":9090", r); err != nil {
		s.Logger().Error().Err(err).Msg("HTTP server error")
	}
}

// handleHealth handles GET /health
func (s *SensorAgent) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.Health()
	w.Header().Set("Content-Type", "application/json")
	if health.Healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(health)
}

// handleGetConfig handles GET /api/v1/config
func (s *SensorAgent) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	interval, trackCount, paused, typeWeights, classificationWeights := s.config.FullSnapshot()

	response := ConfigResponse{
		EmissionIntervalMS:    interval.Milliseconds(),
		TrackCount:            trackCount,
		Paused:                paused,
		TypeWeights:           typeWeights,
		ClassificationWeights: classificationWeights,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handlePatchConfig handles PATCH /api/v1/config
func (s *SensorAgent) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	var req ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// Track changes for later track regeneration
	var trackCountChanged bool
	var weightsChanged bool
	var newTrackCount int

	// Apply updates
	if req.EmissionIntervalMS != nil {
		interval := time.Duration(*req.EmissionIntervalMS) * time.Millisecond
		if err := s.config.SetEmissionInterval(interval); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.Logger().Info().Dur("emission_interval", interval).Msg("Updated emission interval")
	}

	if req.TrackCount != nil {
		if err := s.config.SetTrackCount(*req.TrackCount); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		trackCountChanged = true
		newTrackCount = *req.TrackCount
		s.Logger().Info().Int("track_count", *req.TrackCount).Msg("Updated track count")
	}

	if req.Paused != nil {
		s.config.SetPaused(*req.Paused)
		s.Logger().Info().Bool("paused", *req.Paused).Msg("Updated paused state")
	}

	if req.TypeWeights != nil {
		if err := s.config.SetTypeWeights(*req.TypeWeights); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		weightsChanged = true
		s.Logger().Info().Interface("type_weights", *req.TypeWeights).Msg("Updated type weights")
	}

	if req.ClassificationWeights != nil {
		if err := s.config.SetClassificationWeights(*req.ClassificationWeights); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		weightsChanged = true
		s.Logger().Info().Interface("classification_weights", *req.ClassificationWeights).Msg("Updated classification weights")
	}

	// Regenerate all tracks if weights changed (to apply new type/classification distribution)
	// Otherwise just adjust track count if needed
	if weightsChanged {
		currentCount := s.config.GetTrackCount()
		if trackCountChanged {
			currentCount = newTrackCount
		}
		s.reinitializeTracks(currentCount)
		s.Logger().Info().Int("track_count", currentCount).Msg("Regenerated all tracks with new distribution weights")
	} else if trackCountChanged {
		s.adjustTrackCount(newTrackCount)
	}

	// Purge NATS streams if requested (typically used with paused=true)
	if req.ClearStreams != nil && *req.ClearStreams {
		s.Logger().Info().Msg("Purging NATS JetStream streams")
		if err := s.purgeStreams(r.Context()); err != nil {
			s.Logger().Error().Err(err).Msg("Error during stream purge")
			// Continue anyway - partial purge is still useful
		}
	}

	// Return updated config
	s.handleGetConfig(w, r)
}

// handleResetConfig handles POST /api/v1/config/reset
func (s *SensorAgent) handleResetConfig(w http.ResponseWriter, r *http.Request) {
	s.config.Reset()
	s.Logger().Info().Msg("Configuration reset to defaults")

	// Reinitialize tracks to default count
	s.reinitializeTracks(DefaultTrackCount)

	// Return updated config
	s.handleGetConfig(w, r)
}

// writeError writes an error response
func (s *SensorAgent) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}

// purgeStreams purges all NATS JetStream streams and deletes consumers to clear message backlogs
// This ensures that in-flight messages held by consumers are also discarded
func (s *SensorAgent) purgeStreams(ctx context.Context) error {
	js := s.JetStream()

	// Stream -> Consumer mappings
	streamConsumers := map[string][]string{
		"DETECTIONS": {"classifier"},
		"TRACKS":     {"correlator", "planner"},
		"PROPOSALS":  {"authorizer"},
		"DECISIONS":  {"effector"},
		"EFFECTS":    {},
	}

	for streamName, consumers := range streamConsumers {
		stream, err := js.Stream(ctx, streamName)
		if err != nil {
			s.Logger().Warn().Str("stream", streamName).Err(err).Msg("Could not access stream for purge")
			continue
		}

		// Delete consumers first - this clears their in-flight message buffers
		// Consumers will be auto-recreated by the agents when they next try to consume
		for _, consumerName := range consumers {
			if err := stream.DeleteConsumer(ctx, consumerName); err != nil {
				s.Logger().Warn().Str("stream", streamName).Str("consumer", consumerName).Err(err).Msg("Could not delete consumer")
			} else {
				s.Logger().Info().Str("stream", streamName).Str("consumer", consumerName).Msg("Deleted consumer")
			}
		}

		// Then purge the stream
		if err := stream.Purge(ctx); err != nil {
			s.Logger().Error().Str("stream", streamName).Err(err).Msg("Failed to purge stream")
			continue
		}

		s.Logger().Info().Str("stream", streamName).Msg("Purged stream")
	}

	return nil
}

// adjustTrackCount adds or removes tracks to match the new count
func (s *SensorAgent) adjustTrackCount(newCount int) {
	s.tracksMu.Lock()
	defer s.tracksMu.Unlock()

	currentCount := len(s.tracks)

	if newCount > currentCount {
		// Add new tracks
		s.addTracksLocked(newCount - currentCount)
	} else if newCount < currentCount {
		// Remove excess tracks
		s.removeTracksLocked(currentCount - newCount)
	}
}

// reinitializeTracks clears and reinitializes all tracks
func (s *SensorAgent) reinitializeTracks(count int) {
	s.tracksMu.Lock()
	defer s.tracksMu.Unlock()

	s.tracks = make(map[string]*simulatedTrack)
	s.initializeTracksLocked(count)

	// Log summary of track types generated
	typeCounts := make(map[string]int)
	for _, track := range s.tracks {
		typeCounts[track.trackType]++
	}
	s.Logger().Info().
		Interface("type_distribution", typeCounts).
		Int("total_tracks", len(s.tracks)).
		Msg("Track generation summary after reinitialization")
}

// initializeTracks creates initial simulated tracks
func (s *SensorAgent) initializeTracks(count int) {
	s.tracksMu.Lock()
	defer s.tracksMu.Unlock()
	s.initializeTracksLocked(count)

	// Log summary of track types generated
	typeCounts := make(map[string]int)
	for _, track := range s.tracks {
		typeCounts[track.trackType]++
	}
	s.Logger().Info().
		Interface("type_distribution", typeCounts).
		Int("total_tracks", len(s.tracks)).
		Msg("Track generation summary after initialization")
}

// weightedRandomSelect selects a key from a weights map using weighted random selection
func weightedRandomSelect(weights map[string]int) string {
	// Get sorted keys for deterministic iteration order
	keys := make([]string, 0, len(weights))
	for key := range weights {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Calculate total weight
	total := 0
	for _, weight := range weights {
		total += weight
	}
	if total == 0 {
		// Fallback: return first key
		if len(keys) > 0 {
			return keys[0]
		}
		return ""
	}

	// Generate random number in range [0, total)
	r := rand.Intn(total)

	// Select based on cumulative weights using sorted keys
	cumulative := 0
	for _, key := range keys {
		cumulative += weights[key]
		if r < cumulative {
			return key
		}
	}

	// Fallback (shouldn't reach here)
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// getClassificationPrefix returns the track ID prefix for a classification
func getClassificationPrefix(classification string) string {
	switch classification {
	case "friendly":
		return "F"
	case "hostile":
		return "H"
	case "neutral":
		return "N"
	default:
		return "U"
	}
}

// initializeTracksLocked creates initial simulated tracks (must hold tracksMu)
func (s *SensorAgent) initializeTracksLocked(count int) {
	for i := 0; i < count; i++ {
		s.addSingleTrackLocked(i)
	}
}

// addTracksLocked adds new tracks (must hold tracksMu)
func (s *SensorAgent) addTracksLocked(count int) {
	startIndex := len(s.tracks)

	for i := 0; i < count; i++ {
		s.addSingleTrackLocked(startIndex + i)
	}
}

// addSingleTrackLocked adds a single track (must hold tracksMu)
func (s *SensorAgent) addSingleTrackLocked(index int) {
	// Get current configuration weights
	typeWeights := s.config.GetTypeWeights()
	classificationWeights := s.config.GetClassificationWeights()

	// Select track type using weighted random
	trackType := weightedRandomSelect(typeWeights)

	// Debug logging to verify track type generation
	s.Logger().Debug().
		Int("index", index).
		Str("selected_type", trackType).
		Interface("type_weights", typeWeights).
		Msg("Generated track with type")

	// Select classification using weighted random
	// For missiles, use special missile classification weights (90% hostile, 10% unknown)
	var classification string
	if trackType == "missile" {
		classification = weightedRandomSelect(MissileClassificationWeights)
	} else {
		classification = weightedRandomSelect(classificationWeights)
	}

	// Get track ID prefix based on classification
	prefix := getClassificationPrefix(classification)
	id := fmt.Sprintf("%s-TRK-%04d", prefix, index+1)

	// Ensure unique ID
	for {
		if _, exists := s.tracks[id]; !exists {
			break
		}
		index++
		id = fmt.Sprintf("%s-TRK-%04d", prefix, index+1)
	}

	// Generate altitude and speed based on track type for more realistic simulation
	var alt, speed float64
	switch trackType {
	case "aircraft":
		alt = 5000 + rand.Float64()*10000 // 5000-15000m for aircraft
		speed = 150 + rand.Float64()*300  // 150-450 m/s
	case "vessel":
		alt = 0                       // Sea level
		speed = 5 + rand.Float64()*30 // 5-35 m/s (10-70 knots)
	case "ground":
		alt = rand.Float64() * 100  // 0-100m
		speed = rand.Float64() * 40 // 0-40 m/s
	case "missile":
		alt = 1000 + rand.Float64()*15000 // 1000-16000m for missiles
		speed = 300 + rand.Float64()*700  // 300-1000 m/s (Mach 1-3)
	default: // unknown
		alt = rand.Float64() * 12000     // Random altitude
		speed = 100 + rand.Float64()*400 // 100-500 m/s
	}

	s.tracks[id] = &simulatedTrack{
		id: id,
		position: messages.Position{
			Lat: 35.0 + rand.Float64()*5,     // Around 35-40 degrees lat
			Lon: -120.0 + rand.Float64()*10,  // Around -120 to -110 degrees lon
			Alt: alt,
		},
		velocity: messages.Velocity{
			Speed:   speed,
			Heading: rand.Float64() * 360,
		},
		confidence: 0.7 + rand.Float64()*0.25, // 0.7-0.95 confidence for better classification
		trackType:  trackType,
	}
}

// removeTracksLocked removes tracks (must hold tracksMu)
func (s *SensorAgent) removeTracksLocked(count int) {
	removed := 0
	for id := range s.tracks {
		if removed >= count {
			break
		}
		delete(s.tracks, id)
		removed++
	}
}

// Run starts the sensor simulation loop
func (s *SensorAgent) Run(ctx context.Context) error {
	// Ensure streams exist
	if err := natsutil.SetupStreams(ctx, s.JetStream()); err != nil {
		return fmt.Errorf("failed to setup streams: %w", err)
	}

	interval, trackCount, paused := s.config.Snapshot()
	s.Logger().Info().
		Dur("interval", interval).
		Int("track_count", trackCount).
		Bool("paused", paused).
		Msg("Starting sensor simulation")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Get current configuration
			currentInterval, _, isPaused := s.config.Snapshot()

			// Check if interval changed and reset ticker
			if currentInterval != interval {
				ticker.Reset(currentInterval)
				interval = currentInterval
				s.Logger().Debug().Dur("interval", interval).Msg("Ticker interval updated")
			}

			// Skip emission if paused
			if isPaused {
				continue
			}

			s.emitDetections(ctx)
		}
	}
}

// emitDetections generates and publishes detection events for all tracks
func (s *SensorAgent) emitDetections(ctx context.Context) {
	// Get current emission interval for position updates
	interval := s.config.GetEmissionInterval()

	// Get snapshot of tracks
	s.tracksMu.RLock()
	tracksCopy := make([]*simulatedTrack, 0, len(s.tracks))
	for _, track := range s.tracks {
		tracksCopy = append(tracksCopy, track)
	}
	s.tracksMu.RUnlock()

	for _, track := range tracksCopy {
		// Update track position
		s.updateTrackPosition(track, interval)

		// Sometimes add noise to confidence
		confidence := track.confidence + (rand.Float64()-0.5)*0.1
		confidence = math.Max(0.1, math.Min(1.0, confidence))

		// Create detection
		detection := &messages.Detection{
			Envelope:   messages.NewEnvelope(s.ID(), "sensor"),
			TrackID:    track.id,
			Type:       track.trackType, // Pass track type hint to classifier
			Position:   track.position,
			Velocity:   track.velocity,
			Confidence: confidence,
			SensorType: "radar",
			SensorID:   s.ID(),
		}

		// Debug log for missile types to verify they're being emitted
		if track.trackType == "missile" {
			s.Logger().Info().
				Str("track_id", track.id).
				Str("track_type", track.trackType).
				Str("detection_type", detection.Type).
				Msg("Emitting missile detection")
		}

		// Set correlation ID (new chain for each detection)
		detection.Envelope.CorrelationID = uuid.New().String()

		// Publish
		if err := s.publishDetection(ctx, detection); err != nil {
			s.Logger().Error().Err(err).Str("track_id", track.id).Msg("Failed to publish detection")
			s.RecordError("publish_failed")
			continue
		}

		s.RecordMessage("success", "detection")
	}
}

// updateTrackPosition simulates track movement
func (s *SensorAgent) updateTrackPosition(track *simulatedTrack, interval time.Duration) {
	// Convert heading to radians
	headingRad := track.velocity.Heading * math.Pi / 180

	// Calculate displacement (simplified, not accounting for Earth curvature)
	distance := track.velocity.Speed * interval.Seconds()

	// Convert to degrees (rough approximation)
	latDelta := (distance * math.Cos(headingRad)) / 111000 // ~111km per degree
	lonDelta := (distance * math.Sin(headingRad)) / (111000 * math.Cos(track.position.Lat*math.Pi/180))

	track.position.Lat += latDelta
	track.position.Lon += lonDelta

	// Occasionally change heading
	if rand.Float64() < 0.05 {
		track.velocity.Heading += (rand.Float64() - 0.5) * 20
		if track.velocity.Heading < 0 {
			track.velocity.Heading += 360
		}
		if track.velocity.Heading >= 360 {
			track.velocity.Heading -= 360
		}
	}

	// Occasionally change speed
	if rand.Float64() < 0.05 {
		track.velocity.Speed += (rand.Float64() - 0.5) * 50
		track.velocity.Speed = math.Max(50, math.Min(600, track.velocity.Speed))
	}

	// Occasionally change altitude (for aircraft and missiles)
	if rand.Float64() < 0.05 {
		switch track.trackType {
		case "aircraft":
			track.position.Alt += (rand.Float64() - 0.5) * 500
			track.position.Alt = math.Max(0, math.Min(15000, track.position.Alt))
		case "missile":
			// Missiles have more dramatic altitude changes
			track.position.Alt += (rand.Float64() - 0.5) * 1000
			track.position.Alt = math.Max(100, math.Min(20000, track.position.Alt))
		}
	}
}

// publishDetection publishes a detection to NATS
func (s *SensorAgent) publishDetection(ctx context.Context, det *messages.Detection) error {
	start := time.Now()
	defer func() {
		s.RecordLatency("detection", time.Since(start))
	}()

	data, err := json.Marshal(det)
	if err != nil {
		return fmt.Errorf("failed to marshal detection: %w", err)
	}

	subject := det.Subject()
	_, err = s.JetStream().Publish(ctx, subject, data, jetstream.WithMsgID(det.Envelope.MessageID))
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	// Increment database counter after successful publish
	if s.db != nil {
		counterCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		_, err := s.db.IncrementCounter(counterCtx, "messages_processed", 1)
		cancel()
		if err != nil {
			s.Logger().Warn().Err(err).Msg("Failed to increment message counter")
		}
	}

	s.Logger().Debug().
		Str("track_id", det.TrackID).
		Str("message_id", det.Envelope.MessageID).
		Str("correlation_id", det.Envelope.CorrelationID).
		Msg("Published detection")

	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
