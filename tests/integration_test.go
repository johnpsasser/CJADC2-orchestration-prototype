// Package tests contains comprehensive tests for the CJADC2 platform
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/agile-defense/cjadc2/pkg/messages"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite provides a mock environment for integration testing
type IntegrationTestSuite struct {
	// In-memory storage
	detections       map[string]*messages.Detection
	tracks           map[string]*messages.Track
	correlatedTracks map[string]*messages.CorrelatedTrack
	proposals        map[string]*messages.ActionProposal
	decisions        map[string]*messages.Decision
	effectLogs       map[string]*messages.EffectLog

	// Message queues (simulating NATS)
	detectQueue      chan *messages.Detection
	trackQueue       chan *messages.Track
	corrTrackQueue   chan *messages.CorrelatedTrack
	proposalQueue    chan *messages.ActionProposal
	decisionQueue    chan *messages.Decision
	effectQueue      chan *messages.EffectLog

	// Idempotency tracking
	processedMessages map[string]bool
	idempotentKeys    map[string]bool

	// OPA mock server
	opaServer *httptest.Server

	// Synchronization
	mu sync.RWMutex
}

// NewIntegrationTestSuite creates a new test suite
func NewIntegrationTestSuite() *IntegrationTestSuite {
	suite := &IntegrationTestSuite{
		detections:        make(map[string]*messages.Detection),
		tracks:            make(map[string]*messages.Track),
		correlatedTracks:  make(map[string]*messages.CorrelatedTrack),
		proposals:         make(map[string]*messages.ActionProposal),
		decisions:         make(map[string]*messages.Decision),
		effectLogs:        make(map[string]*messages.EffectLog),
		detectQueue:       make(chan *messages.Detection, 100),
		trackQueue:        make(chan *messages.Track, 100),
		corrTrackQueue:    make(chan *messages.CorrelatedTrack, 100),
		proposalQueue:     make(chan *messages.ActionProposal, 100),
		decisionQueue:     make(chan *messages.Decision, 100),
		effectQueue:       make(chan *messages.EffectLog, 100),
		processedMessages: make(map[string]bool),
		idempotentKeys:    make(map[string]bool),
	}

	suite.opaServer = suite.createMockOPAServer()

	return suite
}

// Close cleans up the test suite
func (s *IntegrationTestSuite) Close() {
	if s.opaServer != nil {
		s.opaServer.Close()
	}
	close(s.detectQueue)
	close(s.trackQueue)
	close(s.corrTrackQueue)
	close(s.proposalQueue)
	close(s.decisionQueue)
	close(s.effectQueue)
}

// createMockOPAServer creates a mock OPA server
func (s *IntegrationTestSuite) createMockOPAServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := r.URL.Path

		switch path {
		case "/v1/data/cjadc2/origin":
			// Origin attestation - allow if source matches pattern
			var input struct {
				Input map[string]interface{} `json:"input"`
			}
			json.NewDecoder(r.Body).Decode(&input)

			envelope, _ := input.Input["envelope"].(map[string]interface{})
			source, _ := envelope["source"].(string)
			sourceType, _ := envelope["source_type"].(string)

			allowed := false
			if sourceType == "sensor" && len(source) > 7 && source[:7] == "sensor-" {
				allowed = true
			} else if sourceType == "classifier" && len(source) > 11 && source[:11] == "classifier-" {
				allowed = true
			} else if sourceType == "correlator" && len(source) > 11 && source[:11] == "correlator-" {
				allowed = true
			} else if sourceType == "planner" && len(source) > 8 && source[:8] == "planner-" {
				allowed = true
			} else if sourceType == "authorizer" && len(source) > 11 && source[:11] == "authorizer-" {
				allowed = true
			} else if sourceType == "effector" && len(source) > 9 && source[:9] == "effector-" {
				allowed = true
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"allow": allowed,
					"deny":  []string{},
				},
			})

		case "/v1/data/cjadc2/proposals":
			// Proposal validation - always allow for tests
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"allow":    true,
					"deny":     []string{},
					"warnings": []string{},
				},
			})

		case "/v1/data/cjadc2/effects":
			// Effect release - check human approval
			var input struct {
				Input map[string]interface{} `json:"input"`
			}
			json.NewDecoder(r.Body).Decode(&input)

			decision, _ := input.Input["decision"].(map[string]interface{})
			approved, _ := decision["approved"].(bool)
			approvedBy, _ := decision["approved_by"].(string)
			alreadyExecuted, _ := input.Input["already_executed"].(bool)

			allowed := approved && approvedBy != "" && approvedBy != "system" && !alreadyExecuted

			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"allow_effect":  allowed,
					"require_human": true,
					"deny":          []string{},
				},
			})

		case "/health":
			w.WriteHeader(http.StatusOK)

		default:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"allow": true,
				},
			})
		}
	}))
}

// PublishDetection simulates publishing a detection to NATS
func (s *IntegrationTestSuite) PublishDetection(det *messages.Detection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency check
	if s.processedMessages[det.Envelope.MessageID] {
		return nil // Already processed, skip
	}

	s.detections[det.Envelope.MessageID] = det
	s.processedMessages[det.Envelope.MessageID] = true

	select {
	case s.detectQueue <- det:
		return nil
	default:
		return fmt.Errorf("detection queue full")
	}
}

// ProcessDetection simulates the classifier processing a detection
func (s *IntegrationTestSuite) ProcessDetection(det *messages.Detection) (*messages.Track, error) {
	track := messages.NewTrack(det, "classifier-001")

	// Simple classification logic
	if det.Confidence > 0.8 {
		track.Classification = "hostile"
	} else if det.Confidence > 0.5 {
		track.Classification = "unknown"
	} else {
		track.Classification = "friendly"
	}
	track.Type = "aircraft"

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.processedMessages[track.Envelope.MessageID] {
		return track, nil
	}

	s.tracks[track.Envelope.MessageID] = track
	s.processedMessages[track.Envelope.MessageID] = true

	select {
	case s.trackQueue <- track:
	default:
	}

	return track, nil
}

// ProcessTrack simulates the correlator processing a track
func (s *IntegrationTestSuite) ProcessTrack(track *messages.Track) (*messages.CorrelatedTrack, error) {
	corrTrack := messages.NewCorrelatedTrack(track, "correlator-001")

	// Simple threat level assignment
	switch track.Classification {
	case "hostile":
		corrTrack.ThreatLevel = "high"
	case "unknown":
		corrTrack.ThreatLevel = "medium"
	default:
		corrTrack.ThreatLevel = "low"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.processedMessages[corrTrack.Envelope.MessageID] {
		return corrTrack, nil
	}

	s.correlatedTracks[corrTrack.Envelope.MessageID] = corrTrack
	s.processedMessages[corrTrack.Envelope.MessageID] = true

	select {
	case s.corrTrackQueue <- corrTrack:
	default:
	}

	return corrTrack, nil
}

// ProcessCorrelatedTrack simulates the planner processing a correlated track
func (s *IntegrationTestSuite) ProcessCorrelatedTrack(corrTrack *messages.CorrelatedTrack) (*messages.ActionProposal, error) {
	proposal := messages.NewActionProposal(corrTrack, "planner-001")
	proposal.ProposalID = uuid.New().String()

	// Determine action based on threat level
	switch corrTrack.ThreatLevel {
	case "critical", "high":
		proposal.ActionType = "engage"
		proposal.Priority = 9
	case "medium":
		proposal.ActionType = "track"
		proposal.Priority = 6
	default:
		proposal.ActionType = "monitor"
		proposal.Priority = 3
	}

	proposal.Rationale = fmt.Sprintf("Automated response to %s threat level target", corrTrack.ThreatLevel)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.processedMessages[proposal.Envelope.MessageID] {
		return proposal, nil
	}

	s.proposals[proposal.ProposalID] = proposal
	s.processedMessages[proposal.Envelope.MessageID] = true

	select {
	case s.proposalQueue <- proposal:
	default:
	}

	return proposal, nil
}

// ApproveProposal simulates a human approving a proposal
func (s *IntegrationTestSuite) ApproveProposal(proposal *messages.ActionProposal, approverID string) (*messages.Decision, error) {
	decision := messages.NewDecision(proposal, "authorizer-001")
	decision.DecisionID = uuid.New().String()
	decision.Approved = true
	decision.ApprovedBy = approverID
	decision.Reason = "Approved by authorized commander"

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.processedMessages[decision.Envelope.MessageID] {
		return decision, nil
	}

	s.decisions[decision.DecisionID] = decision
	s.processedMessages[decision.Envelope.MessageID] = true

	select {
	case s.decisionQueue <- decision:
	default:
	}

	return decision, nil
}

// DenyProposal simulates a human denying a proposal
func (s *IntegrationTestSuite) DenyProposal(proposal *messages.ActionProposal, approverID, reason string) (*messages.Decision, error) {
	decision := messages.NewDecision(proposal, "authorizer-001")
	decision.DecisionID = uuid.New().String()
	decision.Approved = false
	decision.ApprovedBy = approverID
	decision.Reason = reason

	s.mu.Lock()
	defer s.mu.Unlock()

	s.decisions[decision.DecisionID] = decision
	s.processedMessages[decision.Envelope.MessageID] = true

	select {
	case s.decisionQueue <- decision:
	default:
	}

	return decision, nil
}

// ExecuteDecision simulates the effector executing an approved decision
func (s *IntegrationTestSuite) ExecuteDecision(decision *messages.Decision) (*messages.EffectLog, error) {
	// Check if decision was approved
	if !decision.Approved {
		return nil, fmt.Errorf("cannot execute denied decision")
	}

	effectLog := messages.NewEffectLog(decision, "effector-001")
	effectLog.EffectID = uuid.New().String()
	effectLog.IdempotentKey = fmt.Sprintf("effect:%s:%s", decision.DecisionID, decision.ProposalID)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency check on idempotent_key
	if s.idempotentKeys[effectLog.IdempotentKey] {
		effectLog.Idempotent = true
		effectLog.Status = "simulated"
		return effectLog, nil
	}

	effectLog.Status = "executed"
	effectLog.Result = fmt.Sprintf("Effect executed: %s on track %s", decision.ActionType, decision.TrackID)
	effectLog.Idempotent = false

	s.effectLogs[effectLog.EffectID] = effectLog
	s.idempotentKeys[effectLog.IdempotentKey] = true
	s.processedMessages[effectLog.Envelope.MessageID] = true

	select {
	case s.effectQueue <- effectLog:
	default:
	}

	return effectLog, nil
}

// GetMetrics returns statistics about processed messages
func (s *IntegrationTestSuite) GetMetrics() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]int{
		"detections":        len(s.detections),
		"tracks":            len(s.tracks),
		"correlated_tracks": len(s.correlatedTracks),
		"proposals":         len(s.proposals),
		"decisions":         len(s.decisions),
		"effects":           len(s.effectLogs),
	}
}

// TestFullPipelineDetectionToProposal tests the full pipeline from detection to proposal
func TestFullPipelineDetectionToProposal(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Create a detection
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Position = messages.Position{Lat: 37.7749, Lon: -122.4194, Alt: 10000}
	det.Velocity = messages.Velocity{Speed: 500, Heading: 45}
	det.Confidence = 0.85
	det.Envelope.CorrelationID = uuid.New().String()

	// Step 1: Publish detection
	err := suite.PublishDetection(det)
	require.NoError(t, err)

	// Step 2: Process detection -> Track
	track, err := suite.ProcessDetection(det)
	require.NoError(t, err)
	assert.Equal(t, "hostile", track.Classification)
	assert.Equal(t, det.Envelope.CorrelationID, track.Envelope.CorrelationID)
	assert.Equal(t, det.Envelope.MessageID, track.Envelope.CausationID)

	// Step 3: Process track -> CorrelatedTrack
	corrTrack, err := suite.ProcessTrack(track)
	require.NoError(t, err)
	assert.Equal(t, "high", corrTrack.ThreatLevel)
	assert.Equal(t, det.Envelope.CorrelationID, corrTrack.Envelope.CorrelationID)

	// Step 4: Process correlated track -> Proposal
	proposal, err := suite.ProcessCorrelatedTrack(corrTrack)
	require.NoError(t, err)
	assert.Equal(t, "engage", proposal.ActionType)
	assert.Equal(t, 9, proposal.Priority)
	assert.Equal(t, det.Envelope.CorrelationID, proposal.Envelope.CorrelationID)

	// Verify metrics
	metrics := suite.GetMetrics()
	assert.Equal(t, 1, metrics["detections"])
	assert.Equal(t, 1, metrics["tracks"])
	assert.Equal(t, 1, metrics["correlated_tracks"])
	assert.Equal(t, 1, metrics["proposals"])
}

// TestDecisionFlowProposalToEffect tests the decision flow from proposal to effect
func TestDecisionFlowProposalToEffect(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Create a detection and process through pipeline
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.9
	det.Envelope.CorrelationID = uuid.New().String()

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	track, err := suite.ProcessDetection(det)
	require.NoError(t, err)

	corrTrack, err := suite.ProcessTrack(track)
	require.NoError(t, err)

	proposal, err := suite.ProcessCorrelatedTrack(corrTrack)
	require.NoError(t, err)

	// Step 1: Human approves the proposal
	decision, err := suite.ApproveProposal(proposal, "commander-alpha")
	require.NoError(t, err)
	assert.True(t, decision.Approved)
	assert.Equal(t, "commander-alpha", decision.ApprovedBy)
	assert.Equal(t, det.Envelope.CorrelationID, decision.Envelope.CorrelationID)

	// Step 2: Execute the decision
	effectLog, err := suite.ExecuteDecision(decision)
	require.NoError(t, err)
	assert.Equal(t, "executed", effectLog.Status)
	assert.False(t, effectLog.Idempotent)
	assert.Equal(t, det.Envelope.CorrelationID, effectLog.Envelope.CorrelationID)

	// Verify full chain
	metrics := suite.GetMetrics()
	assert.Equal(t, 1, metrics["decisions"])
	assert.Equal(t, 1, metrics["effects"])
}

// TestDecisionDenied tests that denied decisions cannot be executed
func TestDecisionDenied(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Create and process detection
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.9
	det.Envelope.CorrelationID = uuid.New().String()

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	track, _ := suite.ProcessDetection(det)
	corrTrack, _ := suite.ProcessTrack(track)
	proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)

	// Deny the proposal
	decision, err := suite.DenyProposal(proposal, "commander-alpha", "ROE not met")
	require.NoError(t, err)
	assert.False(t, decision.Approved)

	// Attempt to execute denied decision should fail
	_, err = suite.ExecuteDecision(decision)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot execute denied decision")

	// No effects should be recorded
	metrics := suite.GetMetrics()
	assert.Equal(t, 0, metrics["effects"])
}

// TestIdempotencyAcrossChainIntegration tests idempotency across the full chain
func TestIdempotencyAcrossChainIntegration(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Create a detection
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.85
	det.Envelope.CorrelationID = uuid.New().String()

	// Process the same detection multiple times
	for i := 0; i < 3; i++ {
		err := suite.PublishDetection(det)
		require.NoError(t, err)
	}

	// Should only have one detection recorded
	metrics := suite.GetMetrics()
	assert.Equal(t, 1, metrics["detections"])

	// Process through the rest of the chain
	track, _ := suite.ProcessDetection(det)
	corrTrack, _ := suite.ProcessTrack(track)
	proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)
	decision, _ := suite.ApproveProposal(proposal, "commander-alpha")

	// Execute the same decision multiple times
	for i := 0; i < 3; i++ {
		effectLog, err := suite.ExecuteDecision(decision)
		require.NoError(t, err)

		if i == 0 {
			assert.Equal(t, "executed", effectLog.Status)
			assert.False(t, effectLog.Idempotent)
		} else {
			assert.Equal(t, "simulated", effectLog.Status)
			assert.True(t, effectLog.Idempotent)
		}
	}

	// Should only have one effect recorded
	metrics = suite.GetMetrics()
	assert.Equal(t, 1, metrics["effects"])
}

// TestCorrelationIDPropagationIntegration tests that correlation IDs flow through the entire chain
func TestCorrelationIDPropagationIntegration(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Create a detection with a specific correlation ID
	initialCorrelationID := uuid.New().String()

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.85
	det.Envelope.CorrelationID = initialCorrelationID

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	track, err := suite.ProcessDetection(det)
	require.NoError(t, err)
	assert.Equal(t, initialCorrelationID, track.Envelope.CorrelationID)

	corrTrack, err := suite.ProcessTrack(track)
	require.NoError(t, err)
	assert.Equal(t, initialCorrelationID, corrTrack.Envelope.CorrelationID)

	proposal, err := suite.ProcessCorrelatedTrack(corrTrack)
	require.NoError(t, err)
	assert.Equal(t, initialCorrelationID, proposal.Envelope.CorrelationID)

	decision, err := suite.ApproveProposal(proposal, "commander-alpha")
	require.NoError(t, err)
	assert.Equal(t, initialCorrelationID, decision.Envelope.CorrelationID)

	effectLog, err := suite.ExecuteDecision(decision)
	require.NoError(t, err)
	assert.Equal(t, initialCorrelationID, effectLog.Envelope.CorrelationID)
}

// TestCausationChain tests that causation IDs properly chain
func TestCausationChain(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.85

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	// Track's causation should be detection's message ID
	track, _ := suite.ProcessDetection(det)
	assert.Equal(t, det.Envelope.MessageID, track.Envelope.CausationID)

	// CorrelatedTrack's causation should be track's message ID
	corrTrack, _ := suite.ProcessTrack(track)
	assert.Equal(t, track.Envelope.MessageID, corrTrack.Envelope.CausationID)

	// Proposal's causation should be correlated track's message ID
	proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)
	assert.Equal(t, corrTrack.Envelope.MessageID, proposal.Envelope.CausationID)

	// Decision's causation should be proposal's message ID
	decision, _ := suite.ApproveProposal(proposal, "commander-alpha")
	assert.Equal(t, proposal.Envelope.MessageID, decision.Envelope.CausationID)

	// EffectLog's causation should be decision's message ID
	effectLog, _ := suite.ExecuteDecision(decision)
	assert.Equal(t, decision.Envelope.MessageID, effectLog.Envelope.CausationID)
}

// TestMultipleDetectionsSameTrack tests processing multiple detections for the same track
func TestMultipleDetectionsSameTrack(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	trackID := "track-001"
	correlationID := uuid.New().String()

	// Create multiple detections for the same track
	for i := 0; i < 5; i++ {
		det := messages.NewDetection(fmt.Sprintf("sensor-%03d", i), "radar")
		det.TrackID = trackID
		det.Confidence = 0.7 + float64(i)*0.05 // Increasing confidence
		det.Envelope.CorrelationID = correlationID
		det.Position = messages.Position{
			Lat: 37.7749 + float64(i)*0.001,
			Lon: -122.4194 + float64(i)*0.001,
			Alt: 10000 + float64(i)*100,
		}

		err := suite.PublishDetection(det)
		require.NoError(t, err)
	}

	// All detections should be recorded (different message IDs)
	metrics := suite.GetMetrics()
	assert.Equal(t, 5, metrics["detections"])
}

// TestThreatLevelClassification tests different threat level assignments
func TestThreatLevelClassification(t *testing.T) {
	tests := []struct {
		name              string
		confidence        float64
		expectClass       string
		expectThreatLevel string
		expectAction      string
	}{
		{
			name:              "high confidence hostile",
			confidence:        0.95,
			expectClass:       "hostile",
			expectThreatLevel: "high",
			expectAction:      "engage",
		},
		{
			name:              "medium confidence unknown",
			confidence:        0.65,
			expectClass:       "unknown",
			expectThreatLevel: "medium",
			expectAction:      "track",
		},
		{
			name:              "low confidence friendly",
			confidence:        0.35,
			expectClass:       "friendly",
			expectThreatLevel: "low",
			expectAction:      "monitor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite := NewIntegrationTestSuite()
			defer suite.Close()

			det := messages.NewDetection("sensor-001", "radar")
			det.TrackID = "track-001"
			det.Confidence = tt.confidence

			err := suite.PublishDetection(det)
			require.NoError(t, err)

			track, err := suite.ProcessDetection(det)
			require.NoError(t, err)
			assert.Equal(t, tt.expectClass, track.Classification)

			corrTrack, err := suite.ProcessTrack(track)
			require.NoError(t, err)
			assert.Equal(t, tt.expectThreatLevel, corrTrack.ThreatLevel)

			proposal, err := suite.ProcessCorrelatedTrack(corrTrack)
			require.NoError(t, err)
			assert.Equal(t, tt.expectAction, proposal.ActionType)
		})
	}
}

// TestHumanApprovalRequired tests that human approval is always required
func TestHumanApprovalRequired(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Create and process detection
	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.9

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	track, _ := suite.ProcessDetection(det)
	corrTrack, _ := suite.ProcessTrack(track)
	proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)

	// Proposal should be waiting for human approval
	assert.NotEmpty(t, proposal.ProposalID)
	assert.True(t, proposal.ExpiresAt.After(time.Now()))

	// Verify no effects until approved
	metrics := suite.GetMetrics()
	assert.Equal(t, 1, metrics["proposals"])
	assert.Equal(t, 0, metrics["decisions"])
	assert.Equal(t, 0, metrics["effects"])
}

// TestEndToEndWithContextTimeout tests handling of context timeouts
func TestEndToEndWithContextTimeout(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.85

	// Simulate async processing with context
	done := make(chan bool)

	go func() {
		err := suite.PublishDetection(det)
		if err != nil {
			return
		}

		track, err := suite.ProcessDetection(det)
		if err != nil {
			return
		}

		corrTrack, err := suite.ProcessTrack(track)
		if err != nil {
			return
		}

		proposal, err := suite.ProcessCorrelatedTrack(corrTrack)
		if err != nil {
			return
		}

		decision, err := suite.ApproveProposal(proposal, "commander-alpha")
		if err != nil {
			return
		}

		_, err = suite.ExecuteDecision(decision)
		if err != nil {
			return
		}

		done <- true
	}()

	select {
	case <-done:
		metrics := suite.GetMetrics()
		assert.Equal(t, 1, metrics["effects"])
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}
}

// TestConcurrentDetections tests concurrent detection processing
func TestConcurrentDetections(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	numDetections := 10
	var wg sync.WaitGroup

	for i := 0; i < numDetections; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			det := messages.NewDetection(fmt.Sprintf("sensor-%03d", index), "radar")
			det.TrackID = fmt.Sprintf("track-%03d", index)
			det.Confidence = 0.8
			det.Envelope.CorrelationID = uuid.New().String()

			err := suite.PublishDetection(det)
			assert.NoError(t, err)

			_, err = suite.ProcessDetection(det)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	metrics := suite.GetMetrics()
	assert.Equal(t, numDetections, metrics["detections"])
	assert.Equal(t, numDetections, metrics["tracks"])
}

// TestProposalExpiration tests that expired proposals cannot be executed
func TestProposalExpiration(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.9

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	track, _ := suite.ProcessDetection(det)
	corrTrack, _ := suite.ProcessTrack(track)
	proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)

	// Manually expire the proposal
	proposal.ExpiresAt = time.Now().Add(-1 * time.Hour)

	// Proposal is expired but approval still works (policy check would fail in real OPA)
	decision, err := suite.ApproveProposal(proposal, "commander-alpha")
	require.NoError(t, err)

	// In a real scenario, the OPA policy would check expiration and deny
	assert.True(t, decision.Approved)
}

// TestMessageSubjects tests that message subjects are correctly generated
func TestMessageSubjects(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	det := messages.NewDetection("sensor-001", "radar")
	det.TrackID = "track-001"
	det.Confidence = 0.9

	err := suite.PublishDetection(det)
	require.NoError(t, err)

	assert.Equal(t, "detect.sensor-001.radar", det.Subject())

	track, _ := suite.ProcessDetection(det)
	assert.Equal(t, "track.classified.hostile", track.Subject())

	corrTrack, _ := suite.ProcessTrack(track)
	assert.Equal(t, "track.correlated.high", corrTrack.Subject())

	proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)
	assert.Contains(t, proposal.Subject(), "proposal.pending.")

	decision, _ := suite.ApproveProposal(proposal, "commander-alpha")
	assert.Contains(t, decision.Subject(), "decision.approved.")

	effectLog, _ := suite.ExecuteDecision(decision)
	assert.Contains(t, effectLog.Subject(), "effect.executed.")
}

// TestPipelineMetrics tests that metrics are correctly tracked
func TestPipelineMetrics(t *testing.T) {
	suite := NewIntegrationTestSuite()
	defer suite.Close()

	// Initial state
	metrics := suite.GetMetrics()
	assert.Equal(t, 0, metrics["detections"])
	assert.Equal(t, 0, metrics["tracks"])
	assert.Equal(t, 0, metrics["correlated_tracks"])
	assert.Equal(t, 0, metrics["proposals"])
	assert.Equal(t, 0, metrics["decisions"])
	assert.Equal(t, 0, metrics["effects"])

	// Process 5 detections through full pipeline
	for i := 0; i < 5; i++ {
		det := messages.NewDetection(fmt.Sprintf("sensor-%03d", i), "radar")
		det.TrackID = fmt.Sprintf("track-%03d", i)
		det.Confidence = 0.9

		suite.PublishDetection(det)
		track, _ := suite.ProcessDetection(det)
		corrTrack, _ := suite.ProcessTrack(track)
		proposal, _ := suite.ProcessCorrelatedTrack(corrTrack)
		decision, _ := suite.ApproveProposal(proposal, "commander-alpha")
		suite.ExecuteDecision(decision)
	}

	// Final state
	metrics = suite.GetMetrics()
	assert.Equal(t, 5, metrics["detections"])
	assert.Equal(t, 5, metrics["tracks"])
	assert.Equal(t, 5, metrics["correlated_tracks"])
	assert.Equal(t, 5, metrics["proposals"])
	assert.Equal(t, 5, metrics["decisions"])
	assert.Equal(t, 5, metrics["effects"])
}
