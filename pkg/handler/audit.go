package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// AuditHandler handles audit-related HTTP requests
type AuditHandler struct {
	db     *postgres.Pool
	logger zerolog.Logger
}

// NewAuditHandler creates a new AuditHandler
func NewAuditHandler(db *postgres.Pool, logger zerolog.Logger) *AuditHandler {
	return &AuditHandler{
		db:     db,
		logger: logger.With().Str("handler", "audit").Logger(),
	}
}

// Routes returns the audit routes
func (h *AuditHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.GetAuditEntries)

	return r
}

// AuditEntryResponse represents a single audit entry matching frontend AuditEntry type
type AuditEntryResponse struct {
	ID         string  `json:"id"`
	Timestamp  string  `json:"timestamp"`
	ActionType string  `json:"action_type"`
	UserID     *string `json:"user_id,omitempty"`
	TrackID    string  `json:"track_id"`
	ProposalID *string `json:"proposal_id,omitempty"`
	DecisionID *string `json:"decision_id,omitempty"`
	EffectID   *string `json:"effect_id,omitempty"`
	Status     string  `json:"status"`
	Details    string  `json:"details"`
}

// AuditEntriesResponse represents the response for audit entries
type AuditEntriesResponse struct {
	Entries       []AuditEntryResponse `json:"entries"`
	Total         int                  `json:"total"`
	CorrelationID string               `json:"correlation_id"`
}

// GetAuditEntries handles GET /api/v1/audit
func (h *AuditHandler) GetAuditEntries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	// Parse query parameters
	filter := postgres.AuditFilter{
		Limit: 100, // Default limit
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}

	if actionType := r.URL.Query().Get("action_type"); actionType != "" {
		filter.ActionType = actionType
	}

	if userID := r.URL.Query().Get("user_id"); userID != "" {
		filter.UserID = userID
	}

	if trackID := r.URL.Query().Get("track_id"); trackID != "" {
		filter.TrackID = trackID
	}

	// Query audit entries
	entries, err := h.db.ListAuditEntries(ctx, filter)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to get audit entries")
		WriteError(w, http.StatusInternalServerError, "Failed to get audit entries", correlationID)
		return
	}

	// Convert to response format
	responseEntries := make([]AuditEntryResponse, 0, len(entries))
	for _, e := range entries {
		entry := AuditEntryResponse{
			ID:         e.ID,
			Timestamp:  e.Timestamp,
			ActionType: e.ActionType,
			TrackID:    e.TrackID,
			Status:     e.Status,
			Details:    e.Details,
		}

		if e.UserID != "" {
			entry.UserID = &e.UserID
		}
		if e.ProposalID != "" {
			entry.ProposalID = &e.ProposalID
		}
		if e.DecisionID != "" {
			entry.DecisionID = &e.DecisionID
		}
		if e.EffectID != "" {
			entry.EffectID = &e.EffectID
		}

		responseEntries = append(responseEntries, entry)
	}

	// Return the entries array directly (frontend expects AuditEntry[])
	WriteJSON(w, http.StatusOK, responseEntries)
}
