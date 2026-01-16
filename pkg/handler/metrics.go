package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// MetricsHandler handles metrics-related HTTP requests
type MetricsHandler struct {
	db     *postgres.Pool
	logger zerolog.Logger
}

// NewMetricsHandler creates a new MetricsHandler
func NewMetricsHandler(db *postgres.Pool, logger zerolog.Logger) *MetricsHandler {
	return &MetricsHandler{
		db:     db,
		logger: logger.With().Str("handler", "metrics").Logger(),
	}
}

// Routes returns the metrics routes
func (h *MetricsHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.GetCurrentMetrics)
	r.Get("/stages", h.GetStageMetrics)
	r.Get("/latency", h.GetLatencyMetrics)

	return r
}

// StageMetricsResponse represents the response for stage metrics
type StageMetricsResponse struct {
	Stages        []StageMetricResponse `json:"stages"`
	CorrelationID string                `json:"correlation_id"`
}

// StageMetricResponse represents metrics for a single pipeline stage
type StageMetricResponse struct {
	Stage           string  `json:"stage"`
	MessagesTotal   int64   `json:"messages_total"`
	MessagesSuccess int64   `json:"messages_success"`
	MessagesFailed  int64   `json:"messages_failed"`
	SuccessRate     float64 `json:"success_rate"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	P99LatencyMs    float64 `json:"p99_latency_ms"`
}

// GetStageMetrics handles GET /api/v1/metrics/stages
func (h *MetricsHandler) GetStageMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	metrics, err := h.db.GetStageMetrics(ctx)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to get stage metrics")
		WriteError(w, http.StatusInternalServerError, "Failed to get stage metrics", correlationID)
		return
	}

	response := StageMetricsResponse{
		Stages:        make([]StageMetricResponse, 0, len(metrics)),
		CorrelationID: correlationID,
	}

	for _, m := range metrics {
		successRate := float64(0)
		if m.MessagesTotal > 0 {
			successRate = float64(m.MessagesSuccess) / float64(m.MessagesTotal) * 100
		}

		response.Stages = append(response.Stages, StageMetricResponse{
			Stage:           m.Stage,
			MessagesTotal:   m.MessagesTotal,
			MessagesSuccess: m.MessagesSuccess,
			MessagesFailed:  m.MessagesFailed,
			SuccessRate:     successRate,
			AvgLatencyMs:    m.AvgLatencyMs,
			P99LatencyMs:    m.P99LatencyMs,
		})
	}

	// If no metrics in database, return default stages with zero values
	if len(response.Stages) == 0 {
		defaultStages := []string{"sensor", "classifier", "correlator", "planner", "authorizer", "effector"}
		for _, stage := range defaultStages {
			response.Stages = append(response.Stages, StageMetricResponse{
				Stage:           stage,
				MessagesTotal:   0,
				MessagesSuccess: 0,
				MessagesFailed:  0,
				SuccessRate:     0,
				AvgLatencyMs:    0,
				P99LatencyMs:    0,
			})
		}
	}

	WriteJSON(w, http.StatusOK, response)
}

// LatencyMetricsResponse represents the response for latency metrics
type LatencyMetricsResponse struct {
	Window        string  `json:"window"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	MinLatencyMs  float64 `json:"min_latency_ms"`
	MaxLatencyMs  float64 `json:"max_latency_ms"`
	P50LatencyMs  float64 `json:"p50_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	P99LatencyMs  float64 `json:"p99_latency_ms"`
	SampleCount   int64   `json:"sample_count"`
	CorrelationID string  `json:"correlation_id"`
}

// GetLatencyMetrics handles GET /api/v1/metrics/latency
func (h *MetricsHandler) GetLatencyMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}

	// Validate window parameter
	validWindows := map[string]bool{
		"1m":  true,
		"5m":  true,
		"15m": true,
		"1h":  true,
		"6h":  true,
		"24h": true,
	}

	if !validWindows[window] {
		WriteError(w, http.StatusBadRequest, "Invalid window parameter. Valid values: 1m, 5m, 15m, 1h, 6h, 24h", correlationID)
		return
	}

	metrics, err := h.db.GetLatencyMetrics(ctx, window)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to get latency metrics")
		WriteError(w, http.StatusInternalServerError, "Failed to get latency metrics", correlationID)
		return
	}

	// Return default metrics if none found
	if metrics == nil {
		response := LatencyMetricsResponse{
			Window:        window,
			AvgLatencyMs:  0,
			MinLatencyMs:  0,
			MaxLatencyMs:  0,
			P50LatencyMs:  0,
			P95LatencyMs:  0,
			P99LatencyMs:  0,
			SampleCount:   0,
			CorrelationID: correlationID,
		}
		WriteJSON(w, http.StatusOK, response)
		return
	}

	response := LatencyMetricsResponse{
		Window:        metrics.Window,
		AvgLatencyMs:  metrics.AvgLatencyMs,
		MinLatencyMs:  metrics.MinLatencyMs,
		MaxLatencyMs:  metrics.MaxLatencyMs,
		P50LatencyMs:  metrics.P50LatencyMs,
		P95LatencyMs:  metrics.P95LatencyMs,
		P99LatencyMs:  metrics.P99LatencyMs,
		SampleCount:   metrics.SampleCount,
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusOK, response)
}

// SystemMetricsResponse represents overall system metrics (matches frontend SystemMetrics type)
type SystemMetricsResponse struct {
	Stages                 []FrontendStageMetrics `json:"stages"`
	EndToEndLatencyP50Ms   float64                `json:"end_to_end_latency_p50_ms"`
	EndToEndLatencyP95Ms   float64                `json:"end_to_end_latency_p95_ms"`
	EndToEndLatencyP99Ms   float64                `json:"end_to_end_latency_p99_ms"`
	MessagesPerMinute      float64                `json:"messages_per_minute"`
	ActiveTracks           int64                  `json:"active_tracks"`
	PendingProposals       int64                  `json:"pending_proposals"`
	Timestamp              string                 `json:"timestamp"`
}

// FrontendStageMetrics matches the frontend StageMetrics type
type FrontendStageMetrics struct {
	Stage        string  `json:"stage"`
	Processed    int64   `json:"processed"`
	Succeeded    int64   `json:"succeeded"`
	Failed       int64   `json:"failed"`
	LatencyP50Ms float64 `json:"latency_p50_ms"`
	LatencyP95Ms float64 `json:"latency_p95_ms"`
	LatencyP99Ms float64 `json:"latency_p99_ms"`
	LastUpdated  string  `json:"last_updated"`
}

// GetCurrentMetrics handles GET /api/v1/metrics - returns aggregated system metrics
func (h *MetricsHandler) GetCurrentMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	// Get real-time stage metrics from actual table data
	stageMetrics, err := h.db.GetRealTimeStageMetrics(ctx)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to get stage metrics")
		WriteError(w, http.StatusInternalServerError, "Failed to get stage metrics", correlationID)
		return
	}

	// Get active tracks count
	activeTracks, err := h.db.CountActiveTracks(ctx)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to count active tracks")
		WriteError(w, http.StatusInternalServerError, "Failed to count active tracks", correlationID)
		return
	}

	// Get pending proposals count
	pendingProposals, err := h.db.CountPendingProposals(ctx)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to count pending proposals")
		WriteError(w, http.StatusInternalServerError, "Failed to count pending proposals", correlationID)
		return
	}

	// Get real messages per minute from recent proposal activity
	messagesPerMinute, err := h.db.GetMessagesPerMinute(ctx)
	if err != nil {
		h.logger.Warn().Err(err).Str("correlation_id", correlationID).Msg("Failed to get messages per minute, using 0")
		messagesPerMinute = 0
	}

	// Get real E2E latency percentiles from decision/effect data
	e2eP50, e2eP95, e2eP99, err := h.db.GetEndToEndLatencyMetrics(ctx)
	if err != nil {
		h.logger.Warn().Err(err).Str("correlation_id", correlationID).Msg("Failed to get E2E latency, using 0")
		e2eP50, e2eP95, e2eP99 = 0, 0, 0
	}

	// Build response with stage metrics
	stages := make([]FrontendStageMetrics, 0, len(stageMetrics))
	for _, m := range stageMetrics {
		lastUpdated := ""
		if !m.LastUpdated.IsZero() {
			lastUpdated = m.LastUpdated.Format("2006-01-02T15:04:05Z07:00")
		}
		stages = append(stages, FrontendStageMetrics{
			Stage:        m.Stage,
			Processed:    m.Processed,
			Succeeded:    m.Succeeded,
			Failed:       m.Failed,
			LatencyP50Ms: m.LatencyP50,
			LatencyP95Ms: m.LatencyP95,
			LatencyP99Ms: m.LatencyP99,
			LastUpdated:  lastUpdated,
		})
	}

	// Get timestamp - use current time
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")

	response := SystemMetricsResponse{
		Stages:               stages,
		EndToEndLatencyP50Ms: e2eP50,
		EndToEndLatencyP95Ms: e2eP95,
		EndToEndLatencyP99Ms: e2eP99,
		MessagesPerMinute:    messagesPerMinute,
		ActiveTracks:         activeTracks,
		PendingProposals:     pendingProposals,
		Timestamp:            timestamp,
	}

	WriteJSON(w, http.StatusOK, response)
}
