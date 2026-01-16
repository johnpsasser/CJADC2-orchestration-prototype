package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// DecisionHandler handles decision-related HTTP requests
type DecisionHandler struct {
	db     *postgres.Pool
	logger zerolog.Logger
}

// NewDecisionHandler creates a new DecisionHandler
func NewDecisionHandler(db *postgres.Pool, logger zerolog.Logger) *DecisionHandler {
	return &DecisionHandler{
		db:     db,
		logger: logger.With().Str("handler", "decisions").Logger(),
	}
}

// Routes returns the decision routes
func (h *DecisionHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListDecisions)

	return r
}

// DecisionListResponse represents the response for listing decisions
type DecisionListResponse struct {
	Decisions     []DecisionAuditResponse `json:"decisions"`
	Total         int                     `json:"total"`
	Limit         int                     `json:"limit"`
	Offset        int                     `json:"offset"`
	CorrelationID string                  `json:"correlation_id"`
}

// DecisionAuditResponse represents a decision in API responses with audit trail
type DecisionAuditResponse struct {
	DecisionID string    `json:"decision_id"`
	ProposalID string    `json:"proposal_id"`
	TrackID    string    `json:"track_id"`
	ActionType string    `json:"action_type"`
	Approved   bool      `json:"approved"`
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
	Reason     string    `json:"reason,omitempty"`
	Conditions []string  `json:"conditions,omitempty"`

	// Audit fields
	CorrelationID string    `json:"correlation_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListDecisions handles GET /api/v1/decisions
func (h *DecisionHandler) ListDecisions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	filter := postgres.DecisionFilter{
		ProposalID: r.URL.Query().Get("proposal_id"),
		TrackID:    r.URL.Query().Get("track_id"),
		ApprovedBy: r.URL.Query().Get("approved_by"),
	}

	if approvedStr := r.URL.Query().Get("approved"); approvedStr != "" {
		approved := approvedStr == "true"
		filter.Approved = &approved
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

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filter.Since = &since
		}
	}

	decisions, err := h.db.ListDecisions(ctx, filter)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to list decisions")
		WriteError(w, http.StatusInternalServerError, "Failed to list decisions", correlationID)
		return
	}

	response := DecisionListResponse{
		Decisions:     make([]DecisionAuditResponse, 0, len(decisions)),
		Total:         len(decisions),
		Limit:         filter.Limit,
		Offset:        filter.Offset,
		CorrelationID: correlationID,
	}

	for _, d := range decisions {
		response.Decisions = append(response.Decisions, DecisionAuditResponse{
			DecisionID:    d.DecisionID,
			ProposalID:    d.ProposalID,
			TrackID:       d.TrackID,
			ActionType:    d.ActionType,
			Approved:      d.Approved,
			ApprovedBy:    d.ApprovedBy,
			ApprovedAt:    d.ApprovedAt,
			Reason:        d.Reason,
			Conditions:    d.Conditions,
			CorrelationID: correlationID,
			CreatedAt:     d.CreatedAt,
		})
	}

	WriteJSON(w, http.StatusOK, response)
}

// EffectHandler handles effect-related HTTP requests
type EffectHandler struct {
	db     *postgres.Pool
	logger zerolog.Logger
}

// NewEffectHandler creates a new EffectHandler
func NewEffectHandler(db *postgres.Pool, logger zerolog.Logger) *EffectHandler {
	return &EffectHandler{
		db:     db,
		logger: logger.With().Str("handler", "effects").Logger(),
	}
}

// Routes returns the effect routes
func (h *EffectHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListEffects)

	return r
}

// EffectListResponse represents the response for listing effects
type EffectListResponse struct {
	Effects       []EffectResponse `json:"effects"`
	Total         int              `json:"total"`
	Limit         int              `json:"limit"`
	Offset        int              `json:"offset"`
	CorrelationID string           `json:"correlation_id"`
}

// EffectResponse represents an effect in API responses
type EffectResponse struct {
	EffectID      string    `json:"effect_id"`
	DecisionID    string    `json:"decision_id"`
	ProposalID    string    `json:"proposal_id"`
	TrackID       string    `json:"track_id"`
	ActionType    string    `json:"action_type"`
	Status        string    `json:"status"`
	ExecutedAt    time.Time `json:"executed_at"`
	Result        string    `json:"result"`
	IdempotentKey string    `json:"idempotent_key"`
}

// ListEffects handles GET /api/v1/effects
func (h *EffectHandler) ListEffects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	filter := postgres.EffectFilter{
		DecisionID: r.URL.Query().Get("decision_id"),
		ProposalID: r.URL.Query().Get("proposal_id"),
		TrackID:    r.URL.Query().Get("track_id"),
		ActionType: r.URL.Query().Get("action_type"),
		Status:     r.URL.Query().Get("status"),
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

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filter.Since = &since
		}
	}

	effects, err := h.db.ListEffects(ctx, filter)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to list effects")
		WriteError(w, http.StatusInternalServerError, "Failed to list effects", correlationID)
		return
	}

	response := EffectListResponse{
		Effects:       make([]EffectResponse, 0, len(effects)),
		Total:         len(effects),
		Limit:         filter.Limit,
		Offset:        filter.Offset,
		CorrelationID: correlationID,
	}

	for _, e := range effects {
		response.Effects = append(response.Effects, EffectResponse{
			EffectID:      e.EffectID,
			DecisionID:    e.DecisionID,
			ProposalID:    e.ProposalID,
			TrackID:       e.TrackID,
			ActionType:    e.ActionType,
			Status:        e.Status,
			ExecutedAt:    e.ExecutedAt,
			Result:        e.Result,
			IdempotentKey: e.IdempotentKey,
		})
	}

	WriteJSON(w, http.StatusOK, response)
}
