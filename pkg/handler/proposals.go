package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/agile-defense/cjadc2/pkg/messages"
	"github.com/agile-defense/cjadc2/pkg/opa"
	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// ProposalHandler handles proposal-related HTTP requests
type ProposalHandler struct {
	db     *postgres.Pool
	nc     *nats.Conn
	opa    *opa.Client
	logger zerolog.Logger
}

// NewProposalHandler creates a new ProposalHandler
func NewProposalHandler(db *postgres.Pool, nc *nats.Conn, opaClient *opa.Client, logger zerolog.Logger) *ProposalHandler {
	return &ProposalHandler{
		db:     db,
		nc:     nc,
		opa:    opaClient,
		logger: logger.With().Str("handler", "proposals").Logger(),
	}
}

// Routes returns the proposal routes
func (h *ProposalHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListProposals)
	r.Get("/{proposalId}", h.GetProposal)
	r.Post("/{proposalId}/decide", h.DecideProposal)

	return r
}

// ProposalListResponse represents the response for listing proposals
type ProposalListResponse struct {
	Proposals     []ProposalResponse `json:"proposals"`
	Total         int                `json:"total"`
	Limit         int                `json:"limit"`
	Offset        int                `json:"offset"`
	CorrelationID string             `json:"correlation_id"`
}

// ProposalResponse represents a single proposal in API responses
type ProposalResponse struct {
	ProposalID     string          `json:"proposal_id"`
	TrackID        string          `json:"track_id"`
	ActionType     string          `json:"action_type"`
	Priority       int             `json:"priority"`
	ThreatLevel    string          `json:"threat_level"`
	Rationale      string          `json:"rationale"`
	Status         string          `json:"status"`
	ExpiresAt      time.Time       `json:"expires_at"`
	CreatedAt      time.Time       `json:"created_at"`
	PolicyDecision json.RawMessage `json:"policy_decision,omitempty"`
}

// ListProposals handles GET /api/v1/proposals
func (h *ProposalHandler) ListProposals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	filter := postgres.ProposalFilter{
		Status:      r.URL.Query().Get("status"),
		TrackID:     r.URL.Query().Get("track_id"),
		ActionType:  r.URL.Query().Get("action_type"),
		ThreatLevel: r.URL.Query().Get("threat_level"),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	proposals, err := h.db.ListProposals(ctx, filter)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to list proposals")
		WriteError(w, http.StatusInternalServerError, "Failed to list proposals", correlationID)
		return
	}

	response := ProposalListResponse{
		Proposals:     make([]ProposalResponse, 0, len(proposals)),
		Total:         len(proposals),
		Limit:         filter.Limit,
		Offset:        filter.Offset,
		CorrelationID: correlationID,
	}

	for _, p := range proposals {
		response.Proposals = append(response.Proposals, ProposalResponse{
			ProposalID:     p.ProposalID,
			TrackID:        p.TrackID,
			ActionType:     p.ActionType,
			Priority:       p.Priority,
			ThreatLevel:    p.ThreatLevel,
			Rationale:      p.Rationale,
			Status:         p.Status,
			ExpiresAt:      p.ExpiresAt,
			CreatedAt:      p.CreatedAt,
			PolicyDecision: p.PolicyDecision,
		})
	}

	WriteJSON(w, http.StatusOK, response)
}

// ProposalDetailResponse represents the detailed response for a single proposal
type ProposalDetailResponse struct {
	Proposal      ProposalResponse `json:"proposal"`
	CorrelationID string           `json:"correlation_id"`
}

// GetProposal handles GET /api/v1/proposals/{proposalId}
func (h *ProposalHandler) GetProposal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	proposalID := chi.URLParam(r, "proposalId")

	if proposalID == "" {
		WriteError(w, http.StatusBadRequest, "Proposal ID is required", correlationID)
		return
	}

	proposal, err := h.db.GetProposal(ctx, proposalID)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("proposal_id", proposalID).Msg("Failed to get proposal")
		WriteError(w, http.StatusInternalServerError, "Failed to get proposal", correlationID)
		return
	}

	if proposal == nil {
		WriteError(w, http.StatusNotFound, "Proposal not found", correlationID)
		return
	}

	response := ProposalDetailResponse{
		Proposal: ProposalResponse{
			ProposalID:     proposal.ProposalID,
			TrackID:        proposal.TrackID,
			ActionType:     proposal.ActionType,
			Priority:       proposal.Priority,
			ThreatLevel:    proposal.ThreatLevel,
			Rationale:      proposal.Rationale,
			Status:         proposal.Status,
			ExpiresAt:      proposal.ExpiresAt,
			CreatedAt:      proposal.CreatedAt,
			PolicyDecision: proposal.PolicyDecision,
		},
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusOK, response)
}

// DecisionRequest represents the request body for deciding on a proposal
type DecisionRequest struct {
	Approved   bool     `json:"approved"`
	ApprovedBy string   `json:"approved_by"`
	Reason     string   `json:"reason,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

// DecisionResponse represents the response for a decision
type DecisionResponse struct {
	DecisionID    string    `json:"decision_id"`
	ProposalID    string    `json:"proposal_id"`
	Approved      bool      `json:"approved"`
	ApprovedBy    string    `json:"approved_by"`
	ApprovedAt    time.Time `json:"approved_at"`
	Reason        string    `json:"reason,omitempty"`
	CorrelationID string    `json:"correlation_id"`
}

// DecideProposal handles POST /api/v1/proposals/{proposalId}/decide
func (h *ProposalHandler) DecideProposal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	proposalID := chi.URLParam(r, "proposalId")

	if proposalID == "" {
		WriteError(w, http.StatusBadRequest, "Proposal ID is required", correlationID)
		return
	}

	var req DecisionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body", correlationID)
		return
	}

	// Get the proposal
	proposal, err := h.db.GetProposal(ctx, proposalID)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("proposal_id", proposalID).Msg("Failed to get proposal")
		WriteError(w, http.StatusInternalServerError, "Failed to get proposal", correlationID)
		return
	}

	if proposal == nil {
		WriteError(w, http.StatusNotFound, "Proposal not found", correlationID)
		return
	}

	// Check if proposal is still pending
	if proposal.Status != "pending" {
		WriteError(w, http.StatusConflict, "Proposal is not pending", correlationID)
		return
	}

	// Check if proposal has expired
	if time.Now().UTC().After(proposal.ExpiresAt) {
		WriteError(w, http.StatusConflict, "Proposal has expired", correlationID)
		return
	}

	// Get user ID from request or context (set by auth middleware)
	userID := req.ApprovedBy
	if userID == "" {
		userID = GetUserID(ctx)
	}
	if userID == "" {
		WriteError(w, http.StatusBadRequest, "approved_by is required", correlationID)
		return
	}

	// Create the decision
	decision := &messages.Decision{
		Envelope: messages.NewEnvelope("api-gateway", "authorizer").
			WithCorrelation(correlationID, proposal.ProposalID),
		DecisionID: uuid.New().String(),
		ProposalID: proposalID,
		TrackID:    proposal.TrackID,
		ActionType: proposal.ActionType,
		Approved:   req.Approved,
		ApprovedBy: userID,
		ApprovedAt: time.Now().UTC(),
		Reason:     req.Reason,
		Conditions: req.Conditions,
	}

	// Store decision in database
	if err := h.db.InsertDecision(ctx, decision); err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("proposal_id", proposalID).Msg("Failed to insert decision")
		WriteError(w, http.StatusInternalServerError, "Failed to save decision", correlationID)
		return
	}

	// Update proposal status
	newStatus := "denied"
	if req.Approved {
		newStatus = "approved"
	}
	if err := h.db.UpdateProposalStatus(ctx, proposalID, newStatus); err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("proposal_id", proposalID).Msg("Failed to update proposal status")
		// Don't return error - decision was saved
	}

	// Publish decision to NATS
	if h.nc != nil {
		subject := decision.Subject()
		data, err := json.Marshal(decision)
		if err != nil {
			h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to marshal decision")
		} else {
			if err := h.nc.Publish(subject, data); err != nil {
				h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("subject", subject).Msg("Failed to publish decision")
			} else {
				h.logger.Info().
					Str("correlation_id", correlationID).
					Str("decision_id", decision.DecisionID).
					Str("proposal_id", proposalID).
					Bool("approved", req.Approved).
					Str("subject", subject).
					Msg("Decision published")
			}
		}
	}

	response := DecisionResponse{
		DecisionID:    decision.DecisionID,
		ProposalID:    proposalID,
		Approved:      decision.Approved,
		ApprovedBy:    decision.ApprovedBy,
		ApprovedAt:    decision.ApprovedAt,
		Reason:        decision.Reason,
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusCreated, response)
}
