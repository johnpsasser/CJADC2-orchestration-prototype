package handler

import (
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

// ClassifierHandler handles classifier control requests
type ClassifierHandler struct {
	classifierURL string
	client        *http.Client
	logger        zerolog.Logger
}

// NewClassifierHandler creates a new ClassifierHandler
func NewClassifierHandler(classifierURL string, logger zerolog.Logger) *ClassifierHandler {
	return &ClassifierHandler{
		classifierURL: classifierURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger.With().Str("handler", "classifier").Logger(),
	}
}

// Routes returns the classifier routes
func (h *ClassifierHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/config", h.GetConfig)
	r.Patch("/config", h.PatchConfig)
	return r
}

// GetConfig proxies GET /api/v1/classifier/config to the classifier agent
func (h *ClassifierHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.Get(h.classifierURL + "/api/v1/config")
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to reach classifier agent")
		WriteError(w, http.StatusBadGateway, "Failed to reach classifier agent", "")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// PatchConfig proxies PATCH /api/v1/classifier/config to the classifier agent
func (h *ClassifierHandler) PatchConfig(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest("PATCH", h.classifierURL+"/api/v1/config", r.Body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to create request", "")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to reach classifier agent")
		WriteError(w, http.StatusBadGateway, "Failed to reach classifier agent", "")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
