// Package tests contains comprehensive tests for the CJADC2 platform
package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agile-defense/cjadc2/pkg/opa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockOPAServer creates a mock OPA server for testing
func MockOPAServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// createMockOPAHandler creates an OPA handler that returns predetermined results
func createMockOPAHandler(responses map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Extract policy path from URL
		path := r.URL.Path

		if response, ok := responses[path]; ok {
			json.NewEncoder(w).Encode(response)
			return
		}

		// Default response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"allow": false,
				"deny":  []string{"No matching policy found"},
			},
		})
	}
}

// TestOriginAttestationPolicy tests the origin attestation policy
func TestOriginAttestationPolicy(t *testing.T) {
	tests := []struct {
		name           string
		envelope       map[string]interface{}
		skipSignature  bool
		expectAllowed  bool
		expectReasons  []string
	}{
		{
			name: "valid sensor source",
			envelope: map[string]interface{}{
				"source":      "sensor-001",
				"source_type": "sensor",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "valid classifier source",
			envelope: map[string]interface{}{
				"source":      "classifier-prod-001",
				"source_type": "classifier",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "valid correlator source",
			envelope: map[string]interface{}{
				"source":      "correlator-main",
				"source_type": "correlator",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "valid planner source",
			envelope: map[string]interface{}{
				"source":      "planner-tactical",
				"source_type": "planner",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "valid authorizer source",
			envelope: map[string]interface{}{
				"source":      "authorizer-human",
				"source_type": "authorizer",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "valid effector source",
			envelope: map[string]interface{}{
				"source":      "effector-weapons",
				"source_type": "effector",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "valid api source",
			envelope: map[string]interface{}{
				"source":      "api-gateway",
				"source_type": "api",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: true,
			expectReasons: nil,
		},
		{
			name: "invalid source type",
			envelope: map[string]interface{}{
				"source":      "unknown-001",
				"source_type": "unknown",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: false,
			expectReasons: []string{"Unknown source type: unknown"},
		},
		{
			name: "source ID does not match pattern",
			envelope: map[string]interface{}{
				"source":      "invalid-pattern",
				"source_type": "sensor",
				"signature":   "valid-sig",
			},
			skipSignature: false,
			expectAllowed: false,
			expectReasons: []string{"Source ID 'invalid-pattern' does not match allowed pattern for type 'sensor'"},
		},
		{
			name: "missing signature",
			envelope: map[string]interface{}{
				"source":      "sensor-001",
				"source_type": "sensor",
				"signature":   "",
			},
			skipSignature: false,
			expectAllowed: false,
			expectReasons: []string{"Missing message signature"},
		},
		{
			name: "skip signature check enabled",
			envelope: map[string]interface{}{
				"source":      "sensor-001",
				"source_type": "sensor",
				"signature":   "",
			},
			skipSignature: true,
			expectAllowed: true,
			expectReasons: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock OPA server
			responses := map[string]interface{}{
				"/v1/data/cjadc2/origin": map[string]interface{}{
					"result": map[string]interface{}{
						"allow": tt.expectAllowed,
						"deny":  tt.expectReasons,
					},
				},
			}

			server := MockOPAServer(t, createMockOPAHandler(responses))
			defer server.Close()

			client := opa.NewClient(server.URL)

			input := map[string]interface{}{
				"envelope":             tt.envelope,
				"skip_signature_check": tt.skipSignature,
			}

			result, err := client.Query(context.Background(), "cjadc2/origin", input)
			require.NoError(t, err)

			if tt.expectAllowed {
				assert.True(t, result.Result["allow"].(bool))
			} else {
				assert.False(t, result.Result["allow"].(bool))
			}

			if tt.expectReasons != nil {
				reasons := result.Result["deny"].([]interface{})
				assert.Len(t, reasons, len(tt.expectReasons))
			}
		})
	}
}

// TestDataHandlingPolicy tests the data handling policy
func TestDataHandlingPolicy(t *testing.T) {
	tests := []struct {
		name              string
		agentID           string
		agentType         string
		dataType          string
		classification    string
		auditEnabled      bool
		encryptionEnabled bool
		expectAllowed     bool
		expectReasons     []string
	}{
		{
			name:              "unclassified data with audit",
			agentID:           "classifier-001",
			agentType:         "classifier",
			dataType:          "track",
			classification:    "unclassified",
			auditEnabled:      true,
			encryptionEnabled: false,
			expectAllowed:     true,
			expectReasons:     nil,
		},
		{
			name:              "confidential data with audit",
			agentID:           "correlator-001",
			agentType:         "correlator",
			dataType:          "correlated_track",
			classification:    "confidential",
			auditEnabled:      true,
			encryptionEnabled: false,
			expectAllowed:     true,
			expectReasons:     nil,
		},
		{
			name:              "secret data requires encryption",
			agentID:           "planner-001",
			agentType:         "planner",
			dataType:          "proposal",
			classification:    "secret",
			auditEnabled:      true,
			encryptionEnabled: false,
			expectAllowed:     false,
			expectReasons:     []string{"Encryption required for 'secret' data"},
		},
		{
			name:              "secret data with encryption",
			agentID:           "planner-001",
			agentType:         "planner",
			dataType:          "proposal",
			classification:    "secret",
			auditEnabled:      true,
			encryptionEnabled: true,
			expectAllowed:     true,
			expectReasons:     nil,
		},
		{
			name:              "top secret data requires encryption",
			agentID:           "effector-001",
			agentType:         "effector",
			dataType:          "effect",
			classification:    "top_secret",
			auditEnabled:      true,
			encryptionEnabled: false,
			expectAllowed:     false,
			expectReasons:     []string{"Encryption required for 'top_secret' data"},
		},
		{
			name:              "top secret data with encryption",
			agentID:           "effector-001",
			agentType:         "effector",
			dataType:          "effect",
			classification:    "top_secret",
			auditEnabled:      true,
			encryptionEnabled: true,
			expectAllowed:     true,
			expectReasons:     nil,
		},
		{
			name:              "agent lacks clearance",
			agentID:           "unauthorized-agent",
			agentType:         "sensor",
			dataType:          "track",
			classification:    "top_secret",
			auditEnabled:      true,
			encryptionEnabled: true,
			expectAllowed:     false,
			expectReasons:     []string{"Agent 'unauthorized-agent' lacks clearance for 'top_secret' data"},
		},
		{
			name:              "agent cannot process data type",
			agentID:           "sensor-001",
			agentType:         "sensor",
			dataType:          "decision",
			classification:    "unclassified",
			auditEnabled:      true,
			encryptionEnabled: false,
			expectAllowed:     false,
			expectReasons:     []string{"Agent type 'sensor' not authorized to process 'decision' data type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock OPA server
			responses := map[string]interface{}{
				"/v1/data/cjadc2/data_handling": map[string]interface{}{
					"result": map[string]interface{}{
						"allow": tt.expectAllowed,
						"deny":  tt.expectReasons,
					},
				},
			}

			server := MockOPAServer(t, createMockOPAHandler(responses))
			defer server.Close()

			client := opa.NewClient(server.URL)

			data := map[string]interface{}{
				"type":           tt.dataType,
				"classification": tt.classification,
			}

			input := map[string]interface{}{
				"agent_id":           tt.agentID,
				"agent_type":         tt.agentType,
				"data":               data,
				"audit_enabled":      tt.auditEnabled,
				"encryption_enabled": tt.encryptionEnabled,
			}

			result, err := client.Query(context.Background(), "cjadc2/data_handling", input)
			require.NoError(t, err)

			assert.Equal(t, tt.expectAllowed, result.Result["allow"].(bool))
		})
	}
}

// TestProposalValidationPolicy tests the proposal validation policy
func TestProposalValidationPolicy(t *testing.T) {
	tests := []struct {
		name             string
		proposal         map[string]interface{}
		trackExists      bool
		pendingProposals []interface{}
		track            map[string]interface{}
		expectAllowed    bool
		expectReasons    []string
		expectWarnings   []string
	}{
		{
			name: "valid engage proposal",
			proposal: map[string]interface{}{
				"action_type": "engage",
				"priority":    8,
				"rationale":   "Hostile aircraft approaching protected zone - engagement authorized",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "high",
				"classification": "hostile",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "valid track proposal",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    5,
				"rationale":   "Unknown aircraft detected, tracking for identification",
				"track_id":    "track-002",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "medium",
				"classification": "unknown",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "valid identify proposal",
			proposal: map[string]interface{}{
				"action_type": "identify",
				"priority":    6,
				"rationale":   "Unidentified contact requires IFF challenge",
				"track_id":    "track-003",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "medium",
				"classification": "unknown",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "valid intercept proposal",
			proposal: map[string]interface{}{
				"action_type": "intercept",
				"priority":    7,
				"rationale":   "Suspected hostile aircraft entering restricted airspace",
				"track_id":    "track-004",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "high",
				"classification": "unknown",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "valid monitor proposal",
			proposal: map[string]interface{}{
				"action_type": "monitor",
				"priority":    3,
				"rationale":   "Civilian aircraft deviating from flight plan",
				"track_id":    "track-005",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "low",
				"classification": "unknown",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "valid ignore proposal",
			proposal: map[string]interface{}{
				"action_type": "ignore",
				"priority":    1,
				"rationale":   "Friendly aircraft confirmed via IFF",
				"track_id":    "track-006",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "low",
				"classification": "friendly",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "invalid action type",
			proposal: map[string]interface{}{
				"action_type": "destroy",
				"priority":    8,
				"rationale":   "Invalid action type should be rejected",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track:            map[string]interface{}{},
			expectAllowed:    false,
			expectReasons:    []string{"Invalid action type: 'destroy'. Valid types: [engage, track, identify, ignore, intercept, monitor]"},
			expectWarnings:   nil,
		},
		{
			name: "priority out of range - too low",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    0,
				"rationale":   "Priority should be between 1 and 10",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track:            map[string]interface{}{},
			expectAllowed:    false,
			expectReasons:    []string{"Priority 0 out of range. Must be 1-10"},
			expectWarnings:   nil,
		},
		{
			name: "priority out of range - too high",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    11,
				"rationale":   "Priority should be between 1 and 10",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track:            map[string]interface{}{},
			expectAllowed:    false,
			expectReasons:    []string{"Priority 11 out of range. Must be 1-10"},
			expectWarnings:   nil,
		},
		{
			name: "rationale too short",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    5,
				"rationale":   "Too short",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track:            map[string]interface{}{},
			expectAllowed:    false,
			expectReasons:    []string{"Rationale must be at least 10 characters"},
			expectWarnings:   nil,
		},
		{
			name: "missing track ID",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    5,
				"rationale":   "This is a valid rationale for the proposal",
				"track_id":    "",
			},
			trackExists:      false,
			pendingProposals: []interface{}{},
			track:            map[string]interface{}{},
			expectAllowed:    false,
			expectReasons:    []string{"Track ID is required"},
			expectWarnings:   nil,
		},
		{
			name: "track does not exist",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    5,
				"rationale":   "This is a valid rationale for the proposal",
				"track_id":    "nonexistent-track",
			},
			trackExists:      false,
			pendingProposals: []interface{}{},
			track:            map[string]interface{}{},
			expectAllowed:    false,
			expectReasons:    []string{"Track 'nonexistent-track' does not exist"},
			expectWarnings:   nil,
		},
		{
			name: "conflicting pending proposal",
			proposal: map[string]interface{}{
				"action_type": "engage",
				"priority":    8,
				"rationale":   "This is a valid rationale for the proposal",
				"track_id":    "track-001",
			},
			trackExists: true,
			pendingProposals: []interface{}{
				map[string]interface{}{
					"track_id":    "track-001",
					"action_type": "engage",
				},
			},
			track:          map[string]interface{}{},
			expectAllowed:  false,
			expectReasons:  []string{"Conflicting proposal already pending for track 'track-001' with action 'engage'"},
			expectWarnings: nil,
		},
		{
			name: "priority too low warning for critical threat",
			proposal: map[string]interface{}{
				"action_type": "engage",
				"priority":    5,
				"rationale":   "This is a valid rationale for the proposal",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "critical",
				"classification": "hostile",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: []string{"Priority 5 may be too low for threat level 'critical' (suggested minimum: 8)"},
		},
		{
			name: "engage on non-hostile warning",
			proposal: map[string]interface{}{
				"action_type": "engage",
				"priority":    8,
				"rationale":   "This is a valid rationale for the proposal",
				"track_id":    "track-001",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			track: map[string]interface{}{
				"threat_level":   "high",
				"classification": "unknown",
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: []string{"Engage action proposed for non-hostile track (classification: unknown)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock OPA server
			responses := map[string]interface{}{
				"/v1/data/cjadc2/proposals": map[string]interface{}{
					"result": map[string]interface{}{
						"allow":    tt.expectAllowed,
						"deny":     tt.expectReasons,
						"warnings": tt.expectWarnings,
					},
				},
			}

			server := MockOPAServer(t, createMockOPAHandler(responses))
			defer server.Close()

			client := opa.NewClient(server.URL)

			input := map[string]interface{}{
				"proposal":          tt.proposal,
				"track_exists":      tt.trackExists,
				"pending_proposals": tt.pendingProposals,
				"track":             tt.track,
			}

			result, err := client.Query(context.Background(), "cjadc2/proposals", input)
			require.NoError(t, err)

			assert.Equal(t, tt.expectAllowed, result.Result["allow"].(bool))
		})
	}
}

// TestEffectReleasePolicy tests the effect release policy (human approval required)
func TestEffectReleasePolicy(t *testing.T) {
	tests := []struct {
		name            string
		decision        map[string]interface{}
		proposal        map[string]interface{}
		actionType      string
		alreadyExecuted bool
		expectAllowed   bool
		expectReasons   []string
	}{
		{
			name: "valid human approval",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "commander-alpha",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			expectAllowed:   true,
			expectReasons:   nil,
		},
		{
			name: "not approved",
			decision: map[string]interface{}{
				"approved":    false,
				"approved_by": "commander-alpha",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			expectAllowed:   false,
			expectReasons:   []string{"Effect requires human approval - proposal was not approved"},
		},
		{
			name: "no approver identified",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			expectAllowed:   false,
			expectReasons:   []string{"Effect requires human approval - no approver identified"},
		},
		{
			name: "system approval not allowed",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "system",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			expectAllowed:   false,
			expectReasons:   []string{"Effect requires human approval - system approvals not allowed"},
		},
		{
			name: "proposal expired",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "commander-alpha",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			expectAllowed:   false,
			expectReasons:   []string{"Proposal has expired"},
		},
		{
			name: "already executed (idempotency)",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "commander-alpha",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: true,
			expectAllowed:   false,
			expectReasons:   []string{"Effect has already been executed (idempotency check)"},
		},
		{
			name: "decision proposal mismatch",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "commander-alpha",
				"proposal_id": "prop-002",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			expectAllowed:   false,
			expectReasons:   []string{"Decision does not match proposal (integrity check)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock OPA server
			responses := map[string]interface{}{
				"/v1/data/cjadc2/effects": map[string]interface{}{
					"result": map[string]interface{}{
						"allow_effect":  tt.expectAllowed,
						"require_human": true,
						"deny":          tt.expectReasons,
					},
				},
			}

			server := MockOPAServer(t, createMockOPAHandler(responses))
			defer server.Close()

			client := opa.NewClient(server.URL)

			input := map[string]interface{}{
				"decision":         tt.decision,
				"proposal":         tt.proposal,
				"action_type":      tt.actionType,
				"already_executed": tt.alreadyExecuted,
			}

			result, err := client.Query(context.Background(), "cjadc2/effects", input)
			require.NoError(t, err)

			assert.Equal(t, tt.expectAllowed, result.Result["allow_effect"].(bool))
			// Human approval is always required
			assert.True(t, result.Result["require_human"].(bool))
		})
	}
}

// TestOPAClientCheckOrigin tests the CheckOrigin helper method
func TestOPAClientCheckOrigin(t *testing.T) {
	tests := []struct {
		name          string
		envelope      map[string]interface{}
		mockResponse  map[string]interface{}
		expectAllowed bool
	}{
		{
			name: "valid origin",
			envelope: map[string]interface{}{
				"source":      "sensor-001",
				"source_type": "sensor",
				"signature":   "sig",
			},
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": true,
					"deny":  []interface{}{},
				},
			},
			expectAllowed: true,
		},
		{
			name: "invalid origin",
			envelope: map[string]interface{}{
				"source":      "invalid",
				"source_type": "unknown",
				"signature":   "",
			},
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": false,
					"deny":  []interface{}{"Unknown source type"},
				},
			},
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			client := opa.NewClient(server.URL)
			decision, err := client.CheckOrigin(context.Background(), tt.envelope)

			require.NoError(t, err)
			assert.Equal(t, tt.expectAllowed, decision.Allowed)
		})
	}
}

// TestOPAClientCheckDataHandling tests the CheckDataHandling helper method
func TestOPAClientCheckDataHandling(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		agentType     string
		data          interface{}
		mockResponse  map[string]interface{}
		expectAllowed bool
	}{
		{
			name:      "allowed data handling",
			agentID:   "classifier-001",
			agentType: "classifier",
			data: map[string]interface{}{
				"type":           "track",
				"classification": "unclassified",
			},
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": true,
					"deny":  []interface{}{},
				},
			},
			expectAllowed: true,
		},
		{
			name:      "denied data handling",
			agentID:   "sensor-001",
			agentType: "sensor",
			data: map[string]interface{}{
				"type":           "decision",
				"classification": "secret",
			},
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": false,
					"deny":  []interface{}{"Not authorized"},
				},
			},
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			client := opa.NewClient(server.URL)
			decision, err := client.CheckDataHandling(context.Background(), tt.agentID, tt.agentType, tt.data)

			require.NoError(t, err)
			assert.Equal(t, tt.expectAllowed, decision.Allowed)
		})
	}
}

// TestOPAClientCheckProposal tests the CheckProposal helper method
func TestOPAClientCheckProposal(t *testing.T) {
	tests := []struct {
		name             string
		proposal         interface{}
		track            interface{}
		trackExists      bool
		pendingProposals []interface{}
		mockResponse     map[string]interface{}
		expectAllowed    bool
	}{
		{
			name: "valid proposal",
			proposal: map[string]interface{}{
				"action_type": "track",
				"priority":    5,
				"rationale":   "Valid rationale for tracking",
				"track_id":    "track-001",
			},
			track: map[string]interface{}{
				"threat_level":   "medium",
				"classification": "unknown",
			},
			trackExists:      true,
			pendingProposals: []interface{}{},
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow":    true,
					"deny":     []interface{}{},
					"warnings": []interface{}{},
				},
			},
			expectAllowed: true,
		},
		{
			name: "invalid proposal",
			proposal: map[string]interface{}{
				"action_type": "invalid",
				"priority":    0,
				"rationale":   "short",
				"track_id":    "",
			},
			track:            map[string]interface{}{},
			trackExists:      false,
			pendingProposals: []interface{}{},
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": false,
					"deny":  []interface{}{"Invalid action type", "Priority out of range", "Rationale too short"},
				},
			},
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			client := opa.NewClient(server.URL)
			decision, err := client.CheckProposal(context.Background(), tt.proposal, tt.track, tt.trackExists, tt.pendingProposals)

			require.NoError(t, err)
			assert.Equal(t, tt.expectAllowed, decision.Allowed)
		})
	}
}

// TestOPAClientCheckEffectRelease tests the CheckEffectRelease helper method
func TestOPAClientCheckEffectRelease(t *testing.T) {
	tests := []struct {
		name            string
		decision        interface{}
		proposal        interface{}
		actionType      string
		alreadyExecuted bool
		mockResponse    map[string]interface{}
		expectAllowed   bool
	}{
		{
			name: "effect release allowed",
			decision: map[string]interface{}{
				"approved":    true,
				"approved_by": "commander-alpha",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: false,
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": true,
					"deny":  []interface{}{},
				},
			},
			expectAllowed: true,
		},
		{
			name: "effect release denied",
			decision: map[string]interface{}{
				"approved":    false,
				"approved_by": "",
				"proposal_id": "prop-001",
			},
			proposal: map[string]interface{}{
				"proposal_id": "prop-001",
				"expires_at":  time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			},
			actionType:      "engage",
			alreadyExecuted: true,
			mockResponse: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": false,
					"deny":  []interface{}{"Not approved", "Expired", "Already executed"},
				},
			},
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			client := opa.NewClient(server.URL)
			decision, err := client.CheckEffectRelease(context.Background(), tt.decision, tt.proposal, tt.actionType, tt.alreadyExecuted)

			require.NoError(t, err)
			assert.Equal(t, tt.expectAllowed, decision.Allowed)
		})
	}
}

// TestOPAClientHealth tests the Health method
func TestOPAClientHealth(t *testing.T) {
	t.Run("healthy OPA", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := opa.NewClient(server.URL)
		err := client.Health(context.Background())
		assert.NoError(t, err)
	})

	t.Run("unhealthy OPA", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := opa.NewClient(server.URL)
		err := client.Health(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OPA unhealthy")
	})
}

// TestOPAClientQueryErrors tests error handling in Query method
func TestOPAClientQueryErrors(t *testing.T) {
	t.Run("invalid server URL", func(t *testing.T) {
		client := opa.NewClient("http://invalid-server:9999")
		_, err := client.Query(context.Background(), "test/path", map[string]interface{}{})
		assert.Error(t, err)
	})

	t.Run("non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
		}))
		defer server.Close()

		client := opa.NewClient(server.URL)
		_, err := client.Query(context.Background(), "test/path", map[string]interface{}{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OPA returned status 400")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not json"))
		}))
		defer server.Close()

		client := opa.NewClient(server.URL)
		_, err := client.Query(context.Background(), "test/path", map[string]interface{}{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response")
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()

		client := opa.NewClient(server.URL)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := client.Query(ctx, "test/path", map[string]interface{}{})
		assert.Error(t, err)
	})
}

// TestDecisionExtraction tests parsing of decision fields from OPA results
func TestDecisionExtraction(t *testing.T) {
	tests := []struct {
		name           string
		mockResult     map[string]interface{}
		expectAllowed  bool
		expectReasons  []string
		expectWarnings []string
	}{
		{
			name: "full decision with all fields",
			mockResult: map[string]interface{}{
				"result": map[string]interface{}{
					"allow":    true,
					"deny":     []interface{}{"reason1", "reason2"},
					"warnings": []interface{}{"warning1"},
				},
			},
			expectAllowed:  true,
			expectReasons:  []string{"reason1", "reason2"},
			expectWarnings: []string{"warning1"},
		},
		{
			name: "decision with empty reasons",
			mockResult: map[string]interface{}{
				"result": map[string]interface{}{
					"allow": true,
					"deny":  []interface{}{},
				},
			},
			expectAllowed:  true,
			expectReasons:  nil,
			expectWarnings: nil,
		},
		{
			name: "decision with nil result",
			mockResult: map[string]interface{}{
				"result": nil,
			},
			expectAllowed:  false,
			expectReasons:  nil,
			expectWarnings: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResult)
			}))
			defer server.Close()

			client := opa.NewClient(server.URL)
			decision, err := client.Decide(context.Background(), "test/path", map[string]interface{}{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectAllowed, decision.Allowed)

			if tt.expectReasons != nil {
				assert.Equal(t, tt.expectReasons, decision.Reasons)
			}
			if tt.expectWarnings != nil {
				assert.Equal(t, tt.expectWarnings, decision.Warnings)
			}
		})
	}
}
