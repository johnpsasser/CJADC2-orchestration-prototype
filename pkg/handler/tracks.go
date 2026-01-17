// Package handler provides HTTP handlers for the CJADC2 API Gateway
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/agile-defense/cjadc2/pkg/postgres"
)

// TrackHandler handles track-related HTTP requests
type TrackHandler struct {
	db     *postgres.Pool
	logger zerolog.Logger
}

// NewTrackHandler creates a new TrackHandler
func NewTrackHandler(db *postgres.Pool, logger zerolog.Logger) *TrackHandler {
	return &TrackHandler{
		db:     db,
		logger: logger.With().Str("handler", "tracks").Logger(),
	}
}

// Routes returns the track routes
func (h *TrackHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListTracks)
	r.Get("/{trackId}", h.GetTrack)
	r.Get("/{trackId}/history", h.GetTrackHistory)

	return r
}

// TrackListResponse represents the response for listing tracks
type TrackListResponse struct {
	Tracks        []TrackResponse `json:"tracks"`
	Total         int             `json:"total"`
	Limit         int             `json:"limit"`
	Offset        int             `json:"offset"`
	CorrelationID string          `json:"correlation_id"`
}

// TrackResponse represents a single track in API responses
type TrackResponse struct {
	TrackID        string          `json:"track_id"`
	Classification string          `json:"classification"`
	Type           string          `json:"type"`
	ThreatLevel    string          `json:"threat_level"`
	Position       json.RawMessage `json:"position"`
	Velocity       json.RawMessage `json:"velocity"`
	Confidence     float64         `json:"confidence"`
	Sources        []string        `json:"sources"`
	DetectionCount int             `json:"detection_count"`
	FirstSeen      time.Time       `json:"first_seen"`
	LastUpdated    time.Time       `json:"last_updated"`
}

// ListTracks handles GET /api/v1/tracks
func (h *TrackHandler) ListTracks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)

	filter := postgres.TrackFilter{
		Classification: r.URL.Query().Get("classification"),
		ThreatLevel:    r.URL.Query().Get("threat_level"),
		Type:           r.URL.Query().Get("type"),
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
	} else {
		// Default: only return tracks updated within the last 60 seconds
		// This ensures stale tracks are automatically filtered out
		defaultSince := time.Now().Add(-60 * time.Second)
		filter.Since = &defaultSince
	}

	tracks, err := h.db.ListTracks(ctx, filter)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to list tracks")
		WriteError(w, http.StatusInternalServerError, "Failed to list tracks", correlationID)
		return
	}

	response := TrackListResponse{
		Tracks:        make([]TrackResponse, 0, len(tracks)),
		Total:         len(tracks),
		Limit:         filter.Limit,
		Offset:        filter.Offset,
		CorrelationID: correlationID,
	}

	for _, t := range tracks {
		response.Tracks = append(response.Tracks, TrackResponse{
			TrackID:        t.ExternalID,
			Classification: t.Classification,
			Type:           t.Type,
			ThreatLevel:    t.ThreatLevel,
			Position:       t.Position,
			Velocity:       t.Velocity,
			Confidence:     t.Confidence,
			Sources:        t.Sources,
			DetectionCount: t.DetectionCount,
			FirstSeen:      t.FirstSeen,
			LastUpdated:    t.LastUpdated,
		})
	}

	WriteJSON(w, http.StatusOK, response)
}

// TrackDetailResponse represents the detailed response for a single track
type TrackDetailResponse struct {
	Track         TrackResponse `json:"track"`
	CorrelationID string        `json:"correlation_id"`
}

// GetTrack handles GET /api/v1/tracks/{trackId}
func (h *TrackHandler) GetTrack(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	trackID := chi.URLParam(r, "trackId")

	if trackID == "" {
		WriteError(w, http.StatusBadRequest, "Track ID is required", correlationID)
		return
	}

	track, err := h.db.GetTrack(ctx, trackID)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("track_id", trackID).Msg("Failed to get track")
		WriteError(w, http.StatusInternalServerError, "Failed to get track", correlationID)
		return
	}

	if track == nil {
		WriteError(w, http.StatusNotFound, "Track not found", correlationID)
		return
	}

	response := TrackDetailResponse{
		Track: TrackResponse{
			TrackID:        track.ExternalID,
			Classification: track.Classification,
			Type:           track.Type,
			ThreatLevel:    track.ThreatLevel,
			Position:       track.Position,
			Velocity:       track.Velocity,
			Confidence:     track.Confidence,
			Sources:        track.Sources,
			DetectionCount: track.DetectionCount,
			FirstSeen:      track.FirstSeen,
			LastUpdated:    track.LastUpdated,
		},
		CorrelationID: correlationID,
	}

	WriteJSON(w, http.StatusOK, response)
}

// DetectionHistoryResponse represents the response for track detection history
type DetectionHistoryResponse struct {
	TrackID       string              `json:"track_id"`
	Detections    []DetectionResponse `json:"detections"`
	Total         int                 `json:"total"`
	CorrelationID string              `json:"correlation_id"`
}

// DetectionResponse represents a detection in API responses
type DetectionResponse struct {
	SensorID   string          `json:"sensor_id"`
	SensorType string          `json:"sensor_type"`
	Position   json.RawMessage `json:"position"`
	Velocity   json.RawMessage `json:"velocity"`
	Confidence float64         `json:"confidence"`
	Timestamp  time.Time       `json:"timestamp"`
}

// GetTrackHistory handles GET /api/v1/tracks/{trackId}/history
func (h *TrackHandler) GetTrackHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := GetCorrelationID(ctx)
	trackID := chi.URLParam(r, "trackId")

	if trackID == "" {
		WriteError(w, http.StatusBadRequest, "Track ID is required", correlationID)
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Verify track exists
	track, err := h.db.GetTrack(ctx, trackID)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("track_id", trackID).Msg("Failed to get track")
		WriteError(w, http.StatusInternalServerError, "Failed to get track", correlationID)
		return
	}

	if track == nil {
		WriteError(w, http.StatusNotFound, "Track not found", correlationID)
		return
	}

	detections, err := h.db.GetTrackHistory(ctx, trackID, limit)
	if err != nil {
		h.logger.Error().Err(err).Str("correlation_id", correlationID).Str("track_id", trackID).Msg("Failed to get track history")
		WriteError(w, http.StatusInternalServerError, "Failed to get track history", correlationID)
		return
	}

	response := DetectionHistoryResponse{
		TrackID:       trackID,
		Detections:    make([]DetectionResponse, 0, len(detections)),
		Total:         len(detections),
		CorrelationID: correlationID,
	}

	for _, d := range detections {
		response.Detections = append(response.Detections, DetectionResponse{
			SensorID:   d.SensorID,
			SensorType: d.SensorType,
			Position:   d.Position,
			Velocity:   d.Velocity,
			Confidence: d.Confidence,
			Timestamp:  d.Timestamp,
		})
	}

	WriteJSON(w, http.StatusOK, response)
}
