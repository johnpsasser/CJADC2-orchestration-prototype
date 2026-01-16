// Package tests contains comprehensive tests for the CJADC2 platform
package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agile-defense/cjadc2/pkg/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvelopeCreation tests the creation of message envelopes
func TestEnvelopeCreation(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		sourceType string
	}{
		{
			name:       "sensor envelope",
			source:     "sensor-001",
			sourceType: "sensor",
		},
		{
			name:       "classifier envelope",
			source:     "classifier-001",
			sourceType: "classifier",
		},
		{
			name:       "correlator envelope",
			source:     "correlator-001",
			sourceType: "correlator",
		},
		{
			name:       "planner envelope",
			source:     "planner-001",
			sourceType: "planner",
		},
		{
			name:       "authorizer envelope",
			source:     "authorizer-001",
			sourceType: "authorizer",
		},
		{
			name:       "effector envelope",
			source:     "effector-001",
			sourceType: "effector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := messages.NewEnvelope(tt.source, tt.sourceType)

			assert.NotEmpty(t, env.MessageID, "MessageID should be generated")
			assert.Equal(t, tt.source, env.Source)
			assert.Equal(t, tt.sourceType, env.SourceType)
			assert.False(t, env.Timestamp.IsZero(), "Timestamp should be set")
			assert.True(t, env.Timestamp.Before(time.Now().Add(time.Second)), "Timestamp should be recent")
		})
	}
}

// TestEnvelopeWithCorrelation tests setting correlation and causation IDs
func TestEnvelopeWithCorrelation(t *testing.T) {
	tests := []struct {
		name          string
		correlationID string
		causationID   string
	}{
		{
			name:          "both IDs set",
			correlationID: "corr-12345",
			causationID:   "cause-67890",
		},
		{
			name:          "only correlation ID",
			correlationID: "corr-11111",
			causationID:   "",
		},
		{
			name:          "empty correlation ID",
			correlationID: "",
			causationID:   "cause-22222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := messages.NewEnvelope("test-source", "test").
				WithCorrelation(tt.correlationID, tt.causationID)

			assert.Equal(t, tt.correlationID, env.CorrelationID)
			assert.Equal(t, tt.causationID, env.CausationID)
		})
	}
}

// TestEnvelopeWithTracing tests setting OpenTelemetry trace context
func TestEnvelopeWithTracing(t *testing.T) {
	tests := []struct {
		name    string
		traceID string
		spanID  string
	}{
		{
			name:    "valid trace context",
			traceID: "trace-abc123",
			spanID:  "span-def456",
		},
		{
			name:    "empty trace context",
			traceID: "",
			spanID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := messages.NewEnvelope("test-source", "test").
				WithTracing(tt.traceID, tt.spanID)

			assert.Equal(t, tt.traceID, env.TraceID)
			assert.Equal(t, tt.spanID, env.SpanID)
		})
	}
}

// TestEnvelopeSignature tests HMAC signature generation and verification
func TestEnvelopeSignature(t *testing.T) {
	secret := []byte("test-secret-key-for-hmac")
	payload := []byte(`{"test": "data"}`)

	tests := []struct {
		name          string
		payload       []byte
		secret        []byte
		verifySecret  []byte
		expectValid   bool
	}{
		{
			name:         "valid signature with correct secret",
			payload:      payload,
			secret:       secret,
			verifySecret: secret,
			expectValid:  true,
		},
		{
			name:         "invalid signature with wrong secret",
			payload:      payload,
			secret:       secret,
			verifySecret: []byte("wrong-secret"),
			expectValid:  false,
		},
		{
			name:         "valid signature with different payload",
			payload:      []byte(`{"different": "payload"}`),
			secret:       secret,
			verifySecret: secret,
			expectValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := messages.NewEnvelope("test-source", "test")

			// Sign the envelope
			env.Sign(tt.payload, tt.secret)
			assert.NotEmpty(t, env.Signature, "Signature should be generated")

			// Verify the signature
			isValid := env.VerifySignature(tt.payload, tt.verifySecret)
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

// TestEnvelopeSignatureModifiedPayload tests that modified payloads fail verification
func TestEnvelopeSignatureModifiedPayload(t *testing.T) {
	secret := []byte("test-secret-key-for-hmac")
	originalPayload := []byte(`{"test": "data"}`)
	modifiedPayload := []byte(`{"test": "modified"}`)

	env := messages.NewEnvelope("test-source", "test")
	env.Sign(originalPayload, secret)

	// Verification should fail with modified payload
	isValid := env.VerifySignature(modifiedPayload, secret)
	assert.False(t, isValid, "Modified payload should fail verification")
}

// TestDetectionMessage tests Detection message creation and interface
func TestDetectionMessage(t *testing.T) {
	tests := []struct {
		name       string
		sensorID   string
		sensorType string
		trackID    string
		position   messages.Position
		velocity   messages.Velocity
		confidence float64
	}{
		{
			name:       "radar detection",
			sensorID:   "sensor-radar-001",
			sensorType: "radar",
			trackID:    "track-001",
			position:   messages.Position{Lat: 37.7749, Lon: -122.4194, Alt: 10000},
			velocity:   messages.Velocity{Speed: 250, Heading: 45},
			confidence: 0.95,
		},
		{
			name:       "sigint detection",
			sensorID:   "sensor-sigint-002",
			sensorType: "sigint",
			trackID:    "track-002",
			position:   messages.Position{Lat: 34.0522, Lon: -118.2437, Alt: 5000},
			velocity:   messages.Velocity{Speed: 500, Heading: 180},
			confidence: 0.75,
		},
		{
			name:       "eo detection with low confidence",
			sensorID:   "sensor-eo-003",
			sensorType: "eo",
			trackID:    "track-003",
			position:   messages.Position{Lat: 0, Lon: 0, Alt: 0},
			velocity:   messages.Velocity{Speed: 0, Heading: 0},
			confidence: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := messages.NewDetection(tt.sensorID, tt.sensorType)
			det.TrackID = tt.trackID
			det.Position = tt.position
			det.Velocity = tt.velocity
			det.Confidence = tt.confidence

			// Test Message interface
			assert.NotEmpty(t, det.GetEnvelope().MessageID)
			assert.Equal(t, tt.sensorID, det.GetEnvelope().Source)
			assert.Equal(t, "sensor", det.GetEnvelope().SourceType)

			// Test Subject generation
			expectedSubject := "detect." + tt.sensorID + "." + tt.sensorType
			assert.Equal(t, expectedSubject, det.Subject())

			// Test SetEnvelope
			newEnv := messages.NewEnvelope("new-source", "new-type")
			det.SetEnvelope(newEnv)
			assert.Equal(t, "new-source", det.GetEnvelope().Source)
		})
	}
}

// TestTrackMessage tests Track message creation
func TestTrackMessage(t *testing.T) {
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Position = messages.Position{Lat: 37.7749, Lon: -122.4194, Alt: 10000}
	det.Velocity = messages.Velocity{Speed: 250, Heading: 45}
	det.Confidence = 0.9
	det.Envelope.CorrelationID = "corr-001"

	track := messages.NewTrack(det, "classifier-001")

	// Test track fields
	assert.Equal(t, det.TrackID, track.TrackID)
	assert.Equal(t, "unknown", track.Classification)
	assert.Equal(t, "unknown", track.Type)
	assert.Equal(t, det.Position, track.Position)
	assert.Equal(t, det.Velocity, track.Velocity)
	assert.Equal(t, det.Confidence, track.Confidence)
	assert.Equal(t, 1, track.DetectionCount)
	assert.Contains(t, track.Sources, det.SensorID)

	// Test correlation chain
	assert.Equal(t, det.Envelope.CorrelationID, track.Envelope.CorrelationID)
	assert.Equal(t, det.Envelope.MessageID, track.Envelope.CausationID)

	// Test Subject
	assert.Equal(t, "track.classified.unknown", track.Subject())
}

// TestTrackMessageSubject tests Track subject for different classifications
func TestTrackMessageSubject(t *testing.T) {
	tests := []struct {
		classification  string
		expectedSubject string
	}{
		{"friendly", "track.classified.friendly"},
		{"hostile", "track.classified.hostile"},
		{"unknown", "track.classified.unknown"},
		{"neutral", "track.classified.neutral"},
	}

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"

	for _, tt := range tests {
		t.Run(tt.classification, func(t *testing.T) {
			track := messages.NewTrack(det, "classifier-001")
			track.Classification = tt.classification

			assert.Equal(t, tt.expectedSubject, track.Subject())
		})
	}
}

// TestCorrelatedTrackMessage tests CorrelatedTrack message creation
func TestCorrelatedTrackMessage(t *testing.T) {
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Envelope.CorrelationID = "corr-001"

	track := messages.NewTrack(det, "classifier-001")
	track.Classification = "hostile"
	track.Type = "aircraft"

	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")

	// Test correlated track fields
	assert.Equal(t, track.TrackID, corrTrack.TrackID)
	assert.Equal(t, track.Classification, corrTrack.Classification)
	assert.Equal(t, track.Type, corrTrack.Type)
	assert.Equal(t, track.Position, corrTrack.Position)
	assert.Equal(t, track.Velocity, corrTrack.Velocity)
	assert.Contains(t, corrTrack.MergedFrom, track.TrackID)
	assert.Equal(t, "low", corrTrack.ThreatLevel)

	// Test correlation chain
	assert.Equal(t, track.Envelope.CorrelationID, corrTrack.Envelope.CorrelationID)
	assert.Equal(t, track.Envelope.MessageID, corrTrack.Envelope.CausationID)

	// Test window
	assert.False(t, corrTrack.WindowStart.IsZero())
	assert.False(t, corrTrack.WindowEnd.IsZero())
	assert.True(t, corrTrack.WindowEnd.After(corrTrack.WindowStart))
}

// TestCorrelatedTrackSubject tests CorrelatedTrack subject for different threat levels
func TestCorrelatedTrackSubject(t *testing.T) {
	tests := []struct {
		threatLevel     string
		expectedSubject string
	}{
		{"low", "track.correlated.low"},
		{"medium", "track.correlated.medium"},
		{"high", "track.correlated.high"},
		{"critical", "track.correlated.critical"},
	}

	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")

	for _, tt := range tests {
		t.Run(tt.threatLevel, func(t *testing.T) {
			corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
			corrTrack.ThreatLevel = tt.threatLevel

			assert.Equal(t, tt.expectedSubject, corrTrack.Subject())
		})
	}
}

// TestActionProposalMessage tests ActionProposal message creation
func TestActionProposalMessage(t *testing.T) {
	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
	corrTrack.ThreatLevel = "high"
	corrTrack.Envelope.CorrelationID = "corr-001"

	proposal := messages.NewActionProposal(corrTrack, "planner-001")

	// Test proposal fields
	assert.Equal(t, corrTrack.TrackID, proposal.TrackID)
	assert.Equal(t, "track", proposal.ActionType)
	assert.Equal(t, 5, proposal.Priority)
	assert.Equal(t, corrTrack.ThreatLevel, proposal.ThreatLevel)
	assert.Equal(t, corrTrack, proposal.Track)
	assert.False(t, proposal.ExpiresAt.IsZero())
	assert.True(t, proposal.ExpiresAt.After(time.Now()))

	// Test correlation chain
	assert.Equal(t, corrTrack.Envelope.CorrelationID, proposal.Envelope.CorrelationID)
	assert.Equal(t, corrTrack.Envelope.MessageID, proposal.Envelope.CausationID)
}

// TestActionProposalSubject tests ActionProposal subject for different priorities
func TestActionProposalSubject(t *testing.T) {
	tests := []struct {
		priority        int
		expectedSubject string
	}{
		{1, "proposal.pending.normal"},
		{4, "proposal.pending.normal"},
		{5, "proposal.pending.medium"},
		{7, "proposal.pending.medium"},
		{8, "proposal.pending.high"},
		{10, "proposal.pending.high"},
	}

	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")

	for _, tt := range tests {
		t.Run(string(rune(tt.priority)), func(t *testing.T) {
			proposal := messages.NewActionProposal(corrTrack, "planner-001")
			proposal.Priority = tt.priority

			assert.Equal(t, tt.expectedSubject, proposal.Subject())
		})
	}
}

// TestDecisionMessage tests Decision message creation
func TestDecisionMessage(t *testing.T) {
	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
	proposal := messages.NewActionProposal(corrTrack, "planner-001")
	proposal.ProposalID = "prop-001"
	proposal.ActionType = "engage"
	proposal.Envelope.CorrelationID = "corr-001"

	decision := messages.NewDecision(proposal, "authorizer-001")

	// Test decision fields
	assert.Equal(t, proposal.ProposalID, decision.ProposalID)
	assert.Equal(t, proposal.ActionType, decision.ActionType)
	assert.Equal(t, proposal.TrackID, decision.TrackID)
	assert.False(t, decision.ApprovedAt.IsZero())

	// Test correlation chain
	assert.Equal(t, proposal.Envelope.CorrelationID, decision.Envelope.CorrelationID)
	assert.Equal(t, proposal.Envelope.MessageID, decision.Envelope.CausationID)
}

// TestDecisionSubject tests Decision subject for approved/denied states
func TestDecisionSubject(t *testing.T) {
	tests := []struct {
		name            string
		approved        bool
		actionType      string
		expectedSubject string
	}{
		{
			name:            "approved engage",
			approved:        true,
			actionType:      "engage",
			expectedSubject: "decision.approved.engage",
		},
		{
			name:            "denied engage",
			approved:        false,
			actionType:      "engage",
			expectedSubject: "decision.denied.engage",
		},
		{
			name:            "approved track",
			approved:        true,
			actionType:      "track",
			expectedSubject: "decision.approved.track",
		},
		{
			name:            "denied intercept",
			approved:        false,
			actionType:      "intercept",
			expectedSubject: "decision.denied.intercept",
		},
	}

	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
	proposal := messages.NewActionProposal(corrTrack, "planner-001")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposal.ActionType = tt.actionType
			decision := messages.NewDecision(proposal, "authorizer-001")
			decision.Approved = tt.approved

			assert.Equal(t, tt.expectedSubject, decision.Subject())
		})
	}
}

// TestEffectLogMessage tests EffectLog message creation
func TestEffectLogMessage(t *testing.T) {
	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
	proposal := messages.NewActionProposal(corrTrack, "planner-001")
	proposal.ProposalID = "prop-001"
	decision := messages.NewDecision(proposal, "authorizer-001")
	decision.DecisionID = "dec-001"
	decision.Envelope.CorrelationID = "corr-001"

	effectLog := messages.NewEffectLog(decision, "effector-001")

	// Test effect log fields
	assert.Equal(t, decision.DecisionID, effectLog.DecisionID)
	assert.Equal(t, decision.ProposalID, effectLog.ProposalID)
	assert.Equal(t, decision.TrackID, effectLog.TrackID)
	assert.Equal(t, decision.ActionType, effectLog.ActionType)
	assert.Equal(t, "pending", effectLog.Status)
	assert.False(t, effectLog.ExecutedAt.IsZero())

	// Test correlation chain
	assert.Equal(t, decision.Envelope.CorrelationID, effectLog.Envelope.CorrelationID)
	assert.Equal(t, decision.Envelope.MessageID, effectLog.Envelope.CausationID)
}

// TestEffectLogSubject tests EffectLog subject for different statuses
func TestEffectLogSubject(t *testing.T) {
	tests := []struct {
		status          string
		actionType      string
		expectedSubject string
	}{
		{"executed", "engage", "effect.executed.engage"},
		{"failed", "engage", "effect.failed.engage"},
		{"simulated", "track", "effect.simulated.track"},
		{"pending", "intercept", "effect.pending.intercept"},
	}

	det := messages.NewDetection("sensor-001", "radar")
	track := messages.NewTrack(det, "classifier-001")
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
	proposal := messages.NewActionProposal(corrTrack, "planner-001")
	decision := messages.NewDecision(proposal, "authorizer-001")

	for _, tt := range tests {
		t.Run(tt.status+"_"+tt.actionType, func(t *testing.T) {
			decision.ActionType = tt.actionType
			effectLog := messages.NewEffectLog(decision, "effector-001")
			effectLog.Status = tt.status

			assert.Equal(t, tt.expectedSubject, effectLog.Subject())
		})
	}
}

// TestMarshalWithSignature tests marshaling messages with signature
func TestMarshalWithSignature(t *testing.T) {
	secret := []byte("test-secret")

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Position = messages.Position{Lat: 37.7749, Lon: -122.4194, Alt: 10000}
	det.Confidence = 0.9

	data, err := messages.MarshalWithSignature(det, secret)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal and verify signature is set
	var unmarshaled messages.Detection
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.NotEmpty(t, unmarshaled.Envelope.Signature)
}

// TestMessageJSONSerialization tests JSON serialization/deserialization
func TestMessageJSONSerialization(t *testing.T) {
	t.Run("Detection serialization", func(t *testing.T) {
		det := messages.NewDetection("sensor-001", "radar")
		det.TrackID = "track-001"
		det.Position = messages.Position{Lat: 37.7749, Lon: -122.4194, Alt: 10000}
		det.Velocity = messages.Velocity{Speed: 250, Heading: 45}
		det.Confidence = 0.95
		det.RawData = []byte("raw sensor data")

		data, err := json.Marshal(det)
		require.NoError(t, err)

		var unmarshaled messages.Detection
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, det.TrackID, unmarshaled.TrackID)
		assert.Equal(t, det.SensorID, unmarshaled.SensorID)
		assert.Equal(t, det.SensorType, unmarshaled.SensorType)
		assert.Equal(t, det.Position, unmarshaled.Position)
		assert.Equal(t, det.Velocity, unmarshaled.Velocity)
		assert.InDelta(t, det.Confidence, unmarshaled.Confidence, 0.001)
	})

	t.Run("Track serialization", func(t *testing.T) {
		det := messages.NewDetection("sensor-001", "radar")
		track := messages.NewTrack(det, "classifier-001")
		track.Classification = "hostile"
		track.Type = "aircraft"

		data, err := json.Marshal(track)
		require.NoError(t, err)

		var unmarshaled messages.Track
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, track.TrackID, unmarshaled.TrackID)
		assert.Equal(t, track.Classification, unmarshaled.Classification)
		assert.Equal(t, track.Type, unmarshaled.Type)
	})

	t.Run("ActionProposal serialization", func(t *testing.T) {
		det := messages.NewDetection("sensor-001", "radar")
		track := messages.NewTrack(det, "classifier-001")
		corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
		proposal := messages.NewActionProposal(corrTrack, "planner-001")
		proposal.ProposalID = "prop-001"
		proposal.ActionType = "engage"
		proposal.Rationale = "Hostile aircraft approaching protected zone"
		proposal.Constraints = []string{"ROE-ALPHA", "AIRSPACE-CLEARED"}

		data, err := json.Marshal(proposal)
		require.NoError(t, err)

		var unmarshaled messages.ActionProposal
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, proposal.ProposalID, unmarshaled.ProposalID)
		assert.Equal(t, proposal.ActionType, unmarshaled.ActionType)
		assert.Equal(t, proposal.Rationale, unmarshaled.Rationale)
		assert.Equal(t, proposal.Constraints, unmarshaled.Constraints)
	})

	t.Run("Decision serialization", func(t *testing.T) {
		det := messages.NewDetection("sensor-001", "radar")
		track := messages.NewTrack(det, "classifier-001")
		corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
		proposal := messages.NewActionProposal(corrTrack, "planner-001")
		decision := messages.NewDecision(proposal, "authorizer-001")
		decision.DecisionID = "dec-001"
		decision.Approved = true
		decision.ApprovedBy = "commander-alpha"
		decision.Reason = "Target confirmed hostile, weapons release authorized"
		decision.Conditions = []string{"Visual confirmation required", "Collateral damage assessment completed"}

		data, err := json.Marshal(decision)
		require.NoError(t, err)

		var unmarshaled messages.Decision
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, decision.DecisionID, unmarshaled.DecisionID)
		assert.Equal(t, decision.Approved, unmarshaled.Approved)
		assert.Equal(t, decision.ApprovedBy, unmarshaled.ApprovedBy)
		assert.Equal(t, decision.Reason, unmarshaled.Reason)
		assert.Equal(t, decision.Conditions, unmarshaled.Conditions)
	})

	t.Run("EffectLog serialization", func(t *testing.T) {
		det := messages.NewDetection("sensor-001", "radar")
		track := messages.NewTrack(det, "classifier-001")
		corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
		proposal := messages.NewActionProposal(corrTrack, "planner-001")
		decision := messages.NewDecision(proposal, "authorizer-001")
		effectLog := messages.NewEffectLog(decision, "effector-001")
		effectLog.EffectID = "eff-001"
		effectLog.Status = "executed"
		effectLog.Result = "Target engaged successfully"
		effectLog.IdempotentKey = "effect-key-001"
		effectLog.Idempotent = false

		data, err := json.Marshal(effectLog)
		require.NoError(t, err)

		var unmarshaled messages.EffectLog
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, effectLog.EffectID, unmarshaled.EffectID)
		assert.Equal(t, effectLog.Status, unmarshaled.Status)
		assert.Equal(t, effectLog.Result, unmarshaled.Result)
		assert.Equal(t, effectLog.IdempotentKey, unmarshaled.IdempotentKey)
		assert.Equal(t, effectLog.Idempotent, unmarshaled.Idempotent)
	})
}

// TestCorrelationIDPropagation tests that correlation IDs propagate through the message chain
func TestCorrelationIDPropagation(t *testing.T) {
	// Create initial detection with correlation ID
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	initialCorrelationID := "corr-initial-001"
	det.Envelope.CorrelationID = initialCorrelationID

	// Track should inherit correlation ID
	track := messages.NewTrack(det, "classifier-001")
	assert.Equal(t, initialCorrelationID, track.Envelope.CorrelationID)
	assert.Equal(t, det.Envelope.MessageID, track.Envelope.CausationID)

	// CorrelatedTrack should inherit correlation ID
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")
	assert.Equal(t, initialCorrelationID, corrTrack.Envelope.CorrelationID)
	assert.Equal(t, track.Envelope.MessageID, corrTrack.Envelope.CausationID)

	// Proposal should inherit correlation ID
	proposal := messages.NewActionProposal(corrTrack, "planner-001")
	assert.Equal(t, initialCorrelationID, proposal.Envelope.CorrelationID)
	assert.Equal(t, corrTrack.Envelope.MessageID, proposal.Envelope.CausationID)

	// Decision should inherit correlation ID
	decision := messages.NewDecision(proposal, "authorizer-001")
	assert.Equal(t, initialCorrelationID, decision.Envelope.CorrelationID)
	assert.Equal(t, proposal.Envelope.MessageID, decision.Envelope.CausationID)

	// EffectLog should inherit correlation ID
	effectLog := messages.NewEffectLog(decision, "effector-001")
	assert.Equal(t, initialCorrelationID, effectLog.Envelope.CorrelationID)
	assert.Equal(t, decision.Envelope.MessageID, effectLog.Envelope.CausationID)
}

// TestPolicyDecision tests PolicyDecision struct
func TestPolicyDecision(t *testing.T) {
	pd := messages.PolicyDecision{
		Allowed:    true,
		Reasons:    []string{"All conditions met"},
		Violations: nil,
		Warnings:   []string{"Priority may be too low for threat level"},
		Metadata:   map[string]string{"policy_version": "1.0.0"},
	}

	data, err := json.Marshal(pd)
	require.NoError(t, err)

	var unmarshaled messages.PolicyDecision
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, pd.Allowed, unmarshaled.Allowed)
	assert.Equal(t, pd.Reasons, unmarshaled.Reasons)
	assert.Nil(t, unmarshaled.Violations)
	assert.Equal(t, pd.Warnings, unmarshaled.Warnings)
	assert.Equal(t, pd.Metadata, unmarshaled.Metadata)
}

// TestPositionAndVelocity tests Position and Velocity structs
func TestPositionAndVelocity(t *testing.T) {
	t.Run("Position", func(t *testing.T) {
		pos := messages.Position{
			Lat: 37.7749,
			Lon: -122.4194,
			Alt: 10000,
		}

		data, err := json.Marshal(pos)
		require.NoError(t, err)

		var unmarshaled messages.Position
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.InDelta(t, pos.Lat, unmarshaled.Lat, 0.0001)
		assert.InDelta(t, pos.Lon, unmarshaled.Lon, 0.0001)
		assert.InDelta(t, pos.Alt, unmarshaled.Alt, 0.01)
	})

	t.Run("Velocity", func(t *testing.T) {
		vel := messages.Velocity{
			Speed:   250.5,
			Heading: 45.0,
		}

		data, err := json.Marshal(vel)
		require.NoError(t, err)

		var unmarshaled messages.Velocity
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.InDelta(t, vel.Speed, unmarshaled.Speed, 0.01)
		assert.InDelta(t, vel.Heading, unmarshaled.Heading, 0.01)
	})
}

// TestEnvelopeImmutability tests that WithCorrelation and WithTracing return new envelopes
func TestEnvelopeImmutability(t *testing.T) {
	original := messages.NewEnvelope("source-001", "sensor")

	withCorrelation := original.WithCorrelation("corr-001", "cause-001")
	assert.Empty(t, original.CorrelationID, "Original should not be modified")
	assert.Equal(t, "corr-001", withCorrelation.CorrelationID)

	withTracing := original.WithTracing("trace-001", "span-001")
	assert.Empty(t, original.TraceID, "Original should not be modified")
	assert.Equal(t, "trace-001", withTracing.TraceID)
}

// TestBaseMessage tests BaseMessage struct
func TestBaseMessage(t *testing.T) {
	base := &messages.BaseMessage{
		Envelope: messages.NewEnvelope("source-001", "test"),
	}

	env := base.GetEnvelope()
	assert.Equal(t, "source-001", env.Source)
	assert.Equal(t, "test", env.SourceType)

	newEnv := messages.NewEnvelope("new-source", "new-type")
	base.SetEnvelope(newEnv)
	assert.Equal(t, "new-source", base.GetEnvelope().Source)
}
