package messages

import "time"

// ActionProposal represents a proposed action requiring human approval
type ActionProposal struct {
	Envelope Envelope `json:"envelope"`

	// Proposal identification
	ProposalID string `json:"proposal_id"`
	TrackID    string `json:"track_id"`

	// Action details
	ActionType string   `json:"action_type"` // engage, track, identify, ignore, intercept, monitor
	Priority   int      `json:"priority"`    // 1-10, higher is more urgent
	Rationale  string   `json:"rationale"`   // Why this action is proposed
	Constraints []string `json:"constraints,omitempty"`

	// Context
	Track       *CorrelatedTrack `json:"track,omitempty"`
	ThreatLevel string           `json:"threat_level"`

	// Timing
	ExpiresAt time.Time `json:"expires_at"`

	// De-duplication tracking
	HitCount  int       `json:"hit_count"`   // Number of sensor hits for this track
	LastHitAt time.Time `json:"last_hit_at"` // When the most recent sensor hit occurred

	// Policy
	PolicyDecision PolicyDecision `json:"policy_decision"`
}

func (ap *ActionProposal) GetEnvelope() Envelope {
	return ap.Envelope
}

func (ap *ActionProposal) SetEnvelope(e Envelope) {
	ap.Envelope = e
}

func (ap *ActionProposal) Subject() string {
	priority := "normal"
	if ap.Priority >= 8 {
		priority = "high"
	} else if ap.Priority >= 5 {
		priority = "medium"
	}
	return "proposal.pending." + priority
}

// NewActionProposal creates a new action proposal
func NewActionProposal(track *CorrelatedTrack, plannerID string) *ActionProposal {
	now := time.Now().UTC()
	return &ActionProposal{
		Envelope: NewEnvelope(plannerID, "planner").
			WithCorrelation(track.Envelope.CorrelationID, track.Envelope.MessageID),
		ProposalID:  "", // Set by planner
		TrackID:     track.TrackID,
		ActionType:  "track",
		Priority:    5,
		ThreatLevel: track.ThreatLevel,
		Track:       track,
		ExpiresAt:   now.Add(5 * time.Minute),
		HitCount:    1,
		LastHitAt:   now,
	}
}

// Decision represents a human decision on an action proposal
type Decision struct {
	Envelope Envelope `json:"envelope"`

	// Decision identification
	DecisionID string `json:"decision_id"`
	ProposalID string `json:"proposal_id"`

	// Decision
	Approved   bool      `json:"approved"`
	ApprovedBy string    `json:"approved_by"` // User ID
	ApprovedAt time.Time `json:"approved_at"`
	Reason     string    `json:"reason,omitempty"`
	Conditions []string  `json:"conditions,omitempty"`

	// Context
	ActionType string `json:"action_type"`
	TrackID    string `json:"track_id"`
}

func (d *Decision) GetEnvelope() Envelope {
	return d.Envelope
}

func (d *Decision) SetEnvelope(e Envelope) {
	d.Envelope = e
}

func (d *Decision) Subject() string {
	if d.Approved {
		return "decision.approved." + d.ActionType
	}
	return "decision.denied." + d.ActionType
}

// NewDecision creates a new decision for a proposal
func NewDecision(proposal *ActionProposal, authorizerID string) *Decision {
	return &Decision{
		Envelope: NewEnvelope(authorizerID, "authorizer").
			WithCorrelation(proposal.Envelope.CorrelationID, proposal.Envelope.MessageID),
		ProposalID: proposal.ProposalID,
		ActionType: proposal.ActionType,
		TrackID:    proposal.TrackID,
		ApprovedAt: time.Now().UTC(),
	}
}

// EffectLog represents the execution of an approved action
type EffectLog struct {
	Envelope Envelope `json:"envelope"`

	// Effect identification
	EffectID   string `json:"effect_id"`
	DecisionID string `json:"decision_id"`
	ProposalID string `json:"proposal_id"`
	TrackID    string `json:"track_id"`

	// Execution
	ActionType   string    `json:"action_type"`
	Status       string    `json:"status"` // executed, failed, simulated
	ExecutedAt   time.Time `json:"executed_at"`
	Result       string    `json:"result"`
	IdempotentKey string   `json:"idempotent_key"`
	Idempotent   bool      `json:"idempotent"` // True if this was a replay
}

func (el *EffectLog) GetEnvelope() Envelope {
	return el.Envelope
}

func (el *EffectLog) SetEnvelope(e Envelope) {
	el.Envelope = e
}

func (el *EffectLog) Subject() string {
	return "effect." + el.Status + "." + el.ActionType
}

// NewEffectLog creates a new effect log for a decision
func NewEffectLog(decision *Decision, effectorID string) *EffectLog {
	return &EffectLog{
		Envelope: NewEnvelope(effectorID, "effector").
			WithCorrelation(decision.Envelope.CorrelationID, decision.Envelope.MessageID),
		DecisionID: decision.DecisionID,
		ProposalID: decision.ProposalID,
		TrackID:    decision.TrackID,
		ActionType: decision.ActionType,
		Status:     "pending",
		ExecutedAt: time.Now().UTC(),
	}
}
