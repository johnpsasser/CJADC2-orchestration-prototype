package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// InterventionRuleHandler handles intervention rule-related HTTP requests
type InterventionRuleHandler struct {
	db     *postgres.Pool
	logger zerolog.Logger
}

// NewInterventionRuleHandler creates a new InterventionRuleHandler
func NewInterventionRuleHandler(db *postgres.Pool, logger zerolog.Logger) *InterventionRuleHandler {
	return &InterventionRuleHandler{
		db:     db,
		logger: logger.With().Str("handler", "intervention_rules").Logger(),
	}
}

// Routes returns the intervention rule routes
func (h *InterventionRuleHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListInterventionRules)
	r.Get("/{ruleId}", h.GetInterventionRule)
	r.Post("/", h.CreateInterventionRule)
	r.Put("/{ruleId}", h.UpdateInterventionRule)
	r.Delete("/{ruleId}", h.DeleteInterventionRule)

	return r
}

// InterventionRuleResponse represents an intervention rule in API responses
type InterventionRuleResponse struct {
	RuleID           string    `json:"rule_id"`
	Name             string    `json:"name"`
	Description      *string   `json:"description,omitempty"`
	ActionTypes      []string  `json:"action_types"`
	ThreatLevels     []string  `json:"threat_levels"`
	Classifications  []string  `json:"classifications"`
	TrackTypes       []string  `json:"track_types"`
	MinPriority      *int      `json:"min_priority,omitempty"`
	MaxPriority      *int      `json:"max_priority,omitempty"`
	RequiresApproval bool      `json:"requires_approval"`
	AutoApprove      bool      `json:"auto_approve"`
	Enabled          bool      `json:"enabled"`
	EvaluationOrder  int       `json:"evaluation_order"`
	CreatedBy        *string   `json:"created_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedBy        *string   `json:"updated_by,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// InterventionRuleListResponse represents the response for listing intervention rules
type InterventionRuleListResponse struct {
	Rules         []InterventionRuleResponse `json:"rules"`
	Total         int                        `json:"total"`
	Limit         int                        `json:"limit"`
	Offset        int                        `json:"offset"`
	CorrelationID string                     `json:"correlation_id"`
}

// InterventionRuleDetailResponse represents the detailed response for a single intervention rule
type InterventionRuleDetailResponse struct {
	Rule          InterventionRuleResponse `json:"rule"`
	CorrelationID string                   `json:"correlation_id"`
}

// CreateInterventionRuleRequest represents the request body for creating an intervention rule
type CreateInterventionRuleRequest struct {
	Name             string   `json:"name"`
	Description      *string  `json:"description,omitempty"`
	ActionTypes      []string `json:"action_types"`
	ThreatLevels     []string `json:"threat_levels"`
	Classifications  []string `json:"classifications"`
	TrackTypes       []string `json:"track_types"`
	MinPriority      *int     `json:"min_priority,omitempty"`
	MaxPriority      *int     `json:"max_priority,omitempty"`
	RequiresApproval bool     `json:"requires_approval"`
	AutoApprove      bool     `json:"auto_approve"`
	Enabled          bool     `json:"enabled"`
	EvaluationOrder  int      `json:"evaluation_order"`
	CreatedBy        *string  `json:"created_by,omitempty"`
}

// UpdateInterventionRuleRequest represents the request body for updating an intervention rule
type UpdateInterventionRuleRequest struct {
	Name             string   `json:"name"`
	Description      *string  `json:"description,omitempty"`
	ActionTypes      []string `json:"action_types"`
	ThreatLevels     []string `json:"threat_levels"`
	Classifications  []string `json:"classifications"`
	TrackTypes       []string `json:"track_types"`
	MinPriority      *int     `json:"min_priority,omitempty"`
	MaxPriority      *int     `json:"max_priority,omitempty"`
	RequiresApproval bool     `json:"requires_approval"`
	AutoApprove      bool     `json:"auto_approve"`
	Enabled          bool     `json:"enabled"`
	EvaluationOrder  int      `json:"evaluation_order"`
	UpdatedBy        *string  `json:"updated_by,omitempty"`
}

// toResponse converts a database row to an API response
func toInterventionRuleResponse(r postgres.InterventionRuleRow) InterventionRuleResponse {
	return InterventionRuleResponse{
		RuleID:           r.RuleID,
		Name:             r.Name,
		Description:      r.Description,
		ActionTypes:      ensureSlice(r.ActionTypes),
		ThreatLevels:     ensureSlice(r.ThreatLevels),
		Classifications:  ensureSlice(r.Classifications),
		TrackTypes:       ensureSlice(r.TrackTypes),
		MinPriority:      r.MinPriority,
		MaxPriority:      r.MaxPriority,
		RequiresApproval: r.RequiresApproval,
		AutoApprove:      r.AutoApprove,
		Enabled:          r.Enabled,
		EvaluationOrder:  r.EvaluationOrder,
		CreatedBy:        r.CreatedBy,
		CreatedAt:        r.CreatedAt,
		UpdatedBy:        r.UpdatedBy,
		UpdatedAt:        r.UpdatedAt,
	}
}

// ensureSlice returns an empty slice if the input is nil
func ensureSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ListInterventionRules handles GET /api/v1/intervention-rules
func (h *InterventionRuleHandler) ListInterventionRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	filter := postgres.InterventionRuleFilter{
		ActionType: r.URL.Query().Get("action_type"),
	}

	// Parse enabled filter
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := strings.ToLower(enabledStr) == "true"
		filter.Enabled = &enabled
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

	rules, err := h.db.ListInterventionRules(ctx, filter)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to list intervention rules")
		WriteError(w, http.StatusInternalServerError, "Failed to list intervention rules", correlationID)
		return
	}

	response := InterventionRuleListResponse{
		Rules:         make([]InterventionRuleResponse, 0, len(rules)),
		Total:         len(rules),
		Limit:         filter.Limit,
		Offset:        filter.Offset,
		CorrelationID: correlationID,
	}

	for _, r := range rules {
		response.Rules = append(response.Rules, toInterventionRuleResponse(r))
	}

	WriteJSON(w, http.StatusOK, response)
}

// GetInterventionRule handles GET /api/v1/intervention-rules/{ruleId}
func (h *InterventionRuleHandler) GetInterventionRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	ruleID := chi.URLParam(r, "ruleId")

	if ruleID == "" {
		WriteError(w, http.StatusBadRequest, "Rule ID is required", correlationID)
		return
	}

	rule, err := h.db.GetInterventionRule(ctx, ruleID)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("rule_id", ruleID).Msg("Failed to get intervention rule")
		WriteError(w, http.StatusInternalServerError, "Failed to get intervention rule", correlationID)
		return
	}

	if rule == nil {
		WriteError(w, http.StatusNotFound, "Intervention rule not found", correlationID)
		return
	}

	response := InterventionRuleDetailResponse{
		Rule:          toInterventionRuleResponse(*rule),
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusOK, response)
}

// CreateInterventionRule handles POST /api/v1/intervention-rules
func (h *InterventionRuleHandler) CreateInterventionRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	var req CreateInterventionRuleRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body", correlationID)
		return
	}

	// Validate required fields
	if req.Name == "" {
		WriteError(w, http.StatusBadRequest, "Name is required", correlationID)
		return
	}

	// Validate priority range
	if req.MinPriority != nil && req.MaxPriority != nil && *req.MinPriority > *req.MaxPriority {
		WriteError(w, http.StatusBadRequest, "min_priority must be less than or equal to max_priority", correlationID)
		return
	}

	// Get user ID from request or context
	createdBy := req.CreatedBy
	if createdBy == nil {
		userID := GetUserID(ctx)
		if userID != "" {
			createdBy = &userID
		}
	}

	rule := &postgres.InterventionRuleRow{
		RuleID:           uuid.New().String(),
		Name:             req.Name,
		Description:      req.Description,
		ActionTypes:      ensureSlice(req.ActionTypes),
		ThreatLevels:     ensureSlice(req.ThreatLevels),
		Classifications:  ensureSlice(req.Classifications),
		TrackTypes:       ensureSlice(req.TrackTypes),
		MinPriority:      req.MinPriority,
		MaxPriority:      req.MaxPriority,
		RequiresApproval: req.RequiresApproval,
		AutoApprove:      req.AutoApprove,
		Enabled:          req.Enabled,
		EvaluationOrder:  req.EvaluationOrder,
		CreatedBy:        createdBy,
		UpdatedBy:        createdBy,
	}

	if err := h.db.CreateInterventionRule(ctx, rule); err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("rule_name", req.Name).Msg("Failed to create intervention rule")
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "unique_rule_name") || strings.Contains(err.Error(), "duplicate key") {
			WriteError(w, http.StatusConflict, "A rule with this name already exists", correlationID)
			return
		}
		WriteError(w, http.StatusInternalServerError, "Failed to create intervention rule", correlationID)
		return
	}

	h.logger.Info().
		Str("correlation_id", correlationID).
		Str("rule_id", rule.RuleID).
		Str("rule_name", rule.Name).
		Msg("Created intervention rule")

	response := InterventionRuleDetailResponse{
		Rule:          toInterventionRuleResponse(*rule),
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusCreated, response)
}

// UpdateInterventionRule handles PUT /api/v1/intervention-rules/{ruleId}
func (h *InterventionRuleHandler) UpdateInterventionRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	ruleID := chi.URLParam(r, "ruleId")

	if ruleID == "" {
		WriteError(w, http.StatusBadRequest, "Rule ID is required", correlationID)
		return
	}

	var req UpdateInterventionRuleRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body", correlationID)
		return
	}

	// Validate required fields
	if req.Name == "" {
		WriteError(w, http.StatusBadRequest, "Name is required", correlationID)
		return
	}

	// Validate priority range
	if req.MinPriority != nil && req.MaxPriority != nil && *req.MinPriority > *req.MaxPriority {
		WriteError(w, http.StatusBadRequest, "min_priority must be less than or equal to max_priority", correlationID)
		return
	}

	// Check if rule exists
	existingRule, err := h.db.GetInterventionRule(ctx, ruleID)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("rule_id", ruleID).Msg("Failed to get intervention rule")
		WriteError(w, http.StatusInternalServerError, "Failed to get intervention rule", correlationID)
		return
	}

	if existingRule == nil {
		WriteError(w, http.StatusNotFound, "Intervention rule not found", correlationID)
		return
	}

	// Get user ID from request or context
	updatedBy := req.UpdatedBy
	if updatedBy == nil {
		userID := GetUserID(ctx)
		if userID != "" {
			updatedBy = &userID
		}
	}

	rule := &postgres.InterventionRuleRow{
		RuleID:           ruleID,
		Name:             req.Name,
		Description:      req.Description,
		ActionTypes:      ensureSlice(req.ActionTypes),
		ThreatLevels:     ensureSlice(req.ThreatLevels),
		Classifications:  ensureSlice(req.Classifications),
		TrackTypes:       ensureSlice(req.TrackTypes),
		MinPriority:      req.MinPriority,
		MaxPriority:      req.MaxPriority,
		RequiresApproval: req.RequiresApproval,
		AutoApprove:      req.AutoApprove,
		Enabled:          req.Enabled,
		EvaluationOrder:  req.EvaluationOrder,
		UpdatedBy:        updatedBy,
		CreatedBy:        existingRule.CreatedBy,
		CreatedAt:        existingRule.CreatedAt,
	}

	if err := h.db.UpdateInterventionRule(ctx, rule); err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("rule_id", ruleID).Msg("Failed to update intervention rule")
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "unique_rule_name") || strings.Contains(err.Error(), "duplicate key") {
			WriteError(w, http.StatusConflict, "A rule with this name already exists", correlationID)
			return
		}
		WriteError(w, http.StatusInternalServerError, "Failed to update intervention rule", correlationID)
		return
	}

	h.logger.Info().
		Str("correlation_id", correlationID).
		Str("rule_id", rule.RuleID).
		Str("rule_name", rule.Name).
		Msg("Updated intervention rule")

	response := InterventionRuleDetailResponse{
		Rule:          toInterventionRuleResponse(*rule),
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusOK, response)
}

// DeleteInterventionRule handles DELETE /api/v1/intervention-rules/{ruleId}
func (h *InterventionRuleHandler) DeleteInterventionRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	ruleID := chi.URLParam(r, "ruleId")

	if ruleID == "" {
		WriteError(w, http.StatusBadRequest, "Rule ID is required", correlationID)
		return
	}

	if err := h.db.DeleteInterventionRule(ctx, ruleID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, "Intervention rule not found", correlationID)
			return
		}
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("rule_id", ruleID).Msg("Failed to delete intervention rule")
		WriteError(w, http.StatusInternalServerError, "Failed to delete intervention rule", correlationID)
		return
	}

	h.logger.Info().
		Str("correlation_id", correlationID).
		Str("rule_id", ruleID).
		Msg("Deleted intervention rule")

	WriteSuccess(w, http.StatusOK, "Intervention rule deleted successfully", nil, correlationID)
}
