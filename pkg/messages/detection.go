package messages

import "time"

// Detection represents a raw sensor detection event
type Detection struct {
	Envelope Envelope `json:"envelope"`

	// Detection data
	TrackID    string   `json:"track_id"`              // External track identifier
	Type       string   `json:"type,omitempty"`        // Track type hint from sensor: aircraft, vessel, ground, missile, unknown
	Position   Position `json:"position"`              // Geographic position
	Velocity   Velocity `json:"velocity"`              // Speed and heading
	Confidence float64  `json:"confidence"`            // Detection confidence 0.0-1.0
	SensorType string   `json:"sensor_type"`           // radar, eo, sigint, etc.
	SensorID   string   `json:"sensor_id"`             // Sensor that made detection
	RawData    []byte   `json:"raw_data,omitempty"`
}

func (d *Detection) GetEnvelope() Envelope {
	return d.Envelope
}

func (d *Detection) SetEnvelope(e Envelope) {
	d.Envelope = e
}

func (d *Detection) Subject() string {
	return "detect." + d.SensorID + "." + d.SensorType
}

// NewDetection creates a new detection message
func NewDetection(sensorID, sensorType string) *Detection {
	return &Detection{
		Envelope:   NewEnvelope(sensorID, "sensor"),
		SensorID:   sensorID,
		SensorType: sensorType,
		Confidence: 0.0,
	}
}

// Track represents a classified and enriched track
type Track struct {
	Envelope Envelope `json:"envelope"`

	// Track identification
	TrackID        string `json:"track_id"`        // External track identifier
	Classification string `json:"classification"`  // friendly, hostile, unknown, neutral
	Type           string `json:"type"`            // aircraft, vessel, ground, missile, unknown

	// Track data
	Position   Position `json:"position"`
	Velocity   Velocity `json:"velocity"`
	Confidence float64  `json:"confidence"` // Fused confidence 0.0-1.0

	// History
	FirstSeen      time.Time `json:"first_seen"`
	LastUpdated    time.Time `json:"last_updated"`
	DetectionCount int       `json:"detection_count"`
	Sources        []string  `json:"sources"` // Contributing sensor IDs
}

func (t *Track) GetEnvelope() Envelope {
	return t.Envelope
}

func (t *Track) SetEnvelope(e Envelope) {
	t.Envelope = e
}

func (t *Track) Subject() string {
	return "track.classified." + t.Classification
}

// NewTrack creates a new track from a detection
func NewTrack(det *Detection, classifierID string) *Track {
	now := time.Now().UTC()
	return &Track{
		Envelope: NewEnvelope(classifierID, "classifier").
			WithCorrelation(det.Envelope.CorrelationID, det.Envelope.MessageID),
		TrackID:        det.TrackID,
		Classification: "unknown",
		Type:           "unknown",
		Position:       det.Position,
		Velocity:       det.Velocity,
		Confidence:     det.Confidence,
		FirstSeen:      now,
		LastUpdated:    now,
		DetectionCount: 1,
		Sources:        []string{det.SensorID},
	}
}

// CorrelatedTrack represents a track after correlation/deduplication
type CorrelatedTrack struct {
	Envelope Envelope `json:"envelope"`

	// Track identification
	TrackID      string   `json:"track_id"`
	MergedFrom   []string `json:"merged_from"` // Source track IDs that were merged
	Classification string `json:"classification"`
	Type         string   `json:"type"`

	// Track data
	Position    Position `json:"position"`
	Velocity    Velocity `json:"velocity"`
	Confidence  float64  `json:"confidence"`  // Fused confidence
	ThreatLevel string   `json:"threat_level"` // low, medium, high, critical

	// Correlation window
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	LastUpdated time.Time `json:"last_updated"` // Track last update time

	// History
	DetectionCount int      `json:"detection_count"`
	Sources        []string `json:"sources"`
}

func (ct *CorrelatedTrack) GetEnvelope() Envelope {
	return ct.Envelope
}

func (ct *CorrelatedTrack) SetEnvelope(e Envelope) {
	ct.Envelope = e
}

func (ct *CorrelatedTrack) Subject() string {
	return "track.correlated." + ct.ThreatLevel
}

// NewCorrelatedTrack creates a correlated track from a track
func NewCorrelatedTrack(track *Track, correlatorID string) *CorrelatedTrack {
	now := time.Now().UTC()
	return &CorrelatedTrack{
		Envelope: NewEnvelope(correlatorID, "correlator").
			WithCorrelation(track.Envelope.CorrelationID, track.Envelope.MessageID),
		TrackID:        track.TrackID,
		MergedFrom:     []string{track.TrackID},
		Classification: track.Classification,
		Type:           track.Type,
		Position:       track.Position,
		Velocity:       track.Velocity,
		Confidence:     track.Confidence,
		ThreatLevel:    "low",
		WindowStart:    now.Add(-10 * time.Second),
		WindowEnd:      now,
		LastUpdated:    now,
		DetectionCount: track.DetectionCount,
		Sources:        track.Sources,
	}
}
