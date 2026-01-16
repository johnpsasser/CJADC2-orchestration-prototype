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
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
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

// SensorConfig holds the runtime configuration for the sensor agent
type SensorConfig struct {
	mu sync.RWMutex

	emissionInterval time.Duration
	trackCount       int
	paused           bool
}

// NewSensorConfig creates a new SensorConfig with default values
func NewSensorConfig() *SensorConfig {
	return &SensorConfig{
		emissionInterval: DefaultEmissionInterval,
		trackCount:       DefaultTrackCount,
		paused:           false,
	}
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

// Reset resets configuration to default values
func (c *SensorConfig) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emissionInterval = DefaultEmissionInterval
	c.trackCount = DefaultTrackCount
	c.paused = false
}

// Snapshot returns a copy of the current configuration
func (c *SensorConfig) Snapshot() (emissionInterval time.Duration, trackCount int, paused bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.emissionInterval, c.trackCount, c.paused
}

// ConfigResponse represents the JSON response for configuration
type ConfigResponse struct {
	EmissionIntervalMS int64 `json:"emission_interval_ms"`
	TrackCount         int   `json:"track_count"`
	Paused             bool  `json:"paused"`
}

// ConfigUpdateRequest represents a partial configuration update request
type ConfigUpdateRequest struct {
	EmissionIntervalMS *int64 `json:"emission_interval_ms,omitempty"`
	TrackCount         *int   `json:"track_count,omitempty"`
	Paused             *bool  `json:"paused,omitempty"`
}

// SensorAgent generates synthetic detection events
type SensorAgent struct {
	*agent.BaseAgent

	// Thread-safe configuration
	config *SensorConfig

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
	interval, trackCount, paused := s.config.Snapshot()

	response := ConfigResponse{
		EmissionIntervalMS: interval.Milliseconds(),
		TrackCount:         trackCount,
		Paused:             paused,
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

	// Track if track count changed for later adjustment
	var trackCountChanged bool
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

	// Adjust tracks if count changed
	if trackCountChanged {
		s.adjustTrackCount(newTrackCount)
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
}

// initializeTracks creates initial simulated tracks
func (s *SensorAgent) initializeTracks(count int) {
	s.tracksMu.Lock()
	defer s.tracksMu.Unlock()
	s.initializeTracksLocked(count)
}

// initializeTracksLocked creates initial simulated tracks (must hold tracksMu)
func (s *SensorAgent) initializeTracksLocked(count int) {
	trackTypes := []string{"aircraft", "vessel", "ground", "unknown"}

	for i := 0; i < count; i++ {
		s.addSingleTrackLocked(i, trackTypes)
	}
}

// addTracksLocked adds new tracks (must hold tracksMu)
func (s *SensorAgent) addTracksLocked(count int) {
	trackTypes := []string{"aircraft", "vessel", "ground", "unknown"}
	startIndex := len(s.tracks)

	for i := 0; i < count; i++ {
		s.addSingleTrackLocked(startIndex+i, trackTypes)
	}
}

// addSingleTrackLocked adds a single track (must hold tracksMu)
func (s *SensorAgent) addSingleTrackLocked(index int, trackTypes []string) {
	// Distribute prefixes: 30% friendly, 20% hostile, 20% neutral, 30% unknown
	var prefix string
	r := rand.Float64()
	switch {
	case r < 0.3:
		prefix = "F"
	case r < 0.5:
		prefix = "H"
	case r < 0.7:
		prefix = "N"
	default:
		prefix = "U"
	}
	id := fmt.Sprintf("%s-TRK-%04d", prefix, index+1)

	// Ensure unique ID
	for {
		if _, exists := s.tracks[id]; !exists {
			break
		}
		index++
		id = fmt.Sprintf("%s-TRK-%04d", prefix, index+1)
	}

	// Generate altitude based on track type for more realistic classification
	trackType := trackTypes[rand.Intn(len(trackTypes))]
	var alt, speed float64
	switch trackType {
	case "aircraft":
		alt = 5000 + rand.Float64()*10000 // 5000-15000m for aircraft
		speed = 150 + rand.Float64()*300  // 150-450 m/s
	case "vessel":
		alt = 0                        // Sea level
		speed = 5 + rand.Float64()*30  // 5-35 m/s (10-70 knots)
	case "ground":
		alt = rand.Float64() * 100    // 0-100m
		speed = rand.Float64() * 40   // 0-40 m/s
	default:
		alt = rand.Float64() * 12000      // Random altitude
		speed = 100 + rand.Float64()*400  // 100-500 m/s
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
			Position:   track.position,
			Velocity:   track.velocity,
			Confidence: confidence,
			SensorType: "radar",
			SensorID:   s.ID(),
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

	// Occasionally change altitude (for aircraft)
	if track.trackType == "aircraft" && rand.Float64() < 0.05 {
		track.position.Alt += (rand.Float64() - 0.5) * 500
		track.position.Alt = math.Max(0, math.Min(15000, track.position.Alt))
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
