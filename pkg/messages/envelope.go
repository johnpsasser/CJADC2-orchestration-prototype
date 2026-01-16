// Package messages defines the data structures for inter-agent communication
package messages

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope contains metadata common to all messages for tracing and security
type Envelope struct {
	// Identity
	MessageID     string `json:"message_id"`     // UUIDv7 for time-ordering
	CorrelationID string `json:"correlation_id"` // Chain tracking across agents
	CausationID   string `json:"causation_id"`   // Parent message that caused this

	// Routing
	Source     string `json:"source"`      // Agent ID that sent this message
	SourceType string `json:"source_type"` // Agent type (sensor, classifier, etc.)

	// Timing
	Timestamp time.Time `json:"timestamp"` // When message was created

	// Security
	Signature     string `json:"signature"`      // HMAC-SHA256 of payload
	PolicyVersion string `json:"policy_version"` // OPA bundle version used

	// Tracing (OpenTelemetry)
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

// NewEnvelope creates a new envelope with generated IDs
func NewEnvelope(source, sourceType string) Envelope {
	return Envelope{
		MessageID:  uuid.New().String(),
		Source:     source,
		SourceType: sourceType,
		Timestamp:  time.Now().UTC(),
	}
}

// WithCorrelation sets the correlation and causation IDs
func (e Envelope) WithCorrelation(correlationID, causationID string) Envelope {
	e.CorrelationID = correlationID
	e.CausationID = causationID
	return e
}

// WithTracing sets OpenTelemetry trace context
func (e Envelope) WithTracing(traceID, spanID string) Envelope {
	e.TraceID = traceID
	e.SpanID = spanID
	return e
}

// Sign generates an HMAC signature for the message
func (e *Envelope) Sign(payload []byte, secret []byte) {
	h := hmac.New(sha256.New, secret)
	h.Write(payload)
	e.Signature = hex.EncodeToString(h.Sum(nil))
}

// VerifySignature checks the HMAC signature
func (e *Envelope) VerifySignature(payload []byte, secret []byte) bool {
	expected := hmac.New(sha256.New, secret)
	expected.Write(payload)
	expectedSig := hex.EncodeToString(expected.Sum(nil))
	return hmac.Equal([]byte(e.Signature), []byte(expectedSig))
}

// Message is an interface for all message types
type Message interface {
	GetEnvelope() Envelope
	SetEnvelope(Envelope)
	Subject() string
}

// BaseMessage provides common functionality
type BaseMessage struct {
	Envelope Envelope `json:"envelope"`
}

func (m *BaseMessage) GetEnvelope() Envelope {
	return m.Envelope
}

func (m *BaseMessage) SetEnvelope(e Envelope) {
	m.Envelope = e
}

// MarshalWithSignature marshals the message and signs it
func MarshalWithSignature(msg Message, secret []byte) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	env := msg.GetEnvelope()
	env.Sign(data, secret)
	msg.SetEnvelope(env)

	return json.Marshal(msg)
}

// PolicyDecision captures an OPA policy evaluation result
type PolicyDecision struct {
	Allowed    bool              `json:"allowed"`
	Reasons    []string          `json:"reasons,omitempty"`
	Violations []string          `json:"violations,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Position represents a geographic position
type Position struct {
	Lat float64 `json:"lat"` // Latitude in degrees
	Lon float64 `json:"lon"` // Longitude in degrees
	Alt float64 `json:"alt"` // Altitude in meters MSL
}

// Velocity represents speed and direction
type Velocity struct {
	Speed   float64 `json:"speed"`   // Speed in m/s
	Heading float64 `json:"heading"` // Heading in degrees true
}
