package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// Context keys for request-scoped values
type contextKey string

const (
	correlationIDKey contextKey = "correlation_id"
	userIDKey        contextKey = "user_id"
)

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// GetCorrelationID retrieves the correlation ID from the context
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey).(string); ok {
		return id
	}
	return uuid.New().String()
}

// WithUserID adds a user ID to the context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID retrieves the user ID from the context
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// ErrorResponse represents a structured error response
type ErrorResponse struct {
	Error         string `json:"error"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id"`
}

// WriteJSON writes a JSON response with the given status code
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// WriteError writes a JSON error response
func WriteError(w http.ResponseWriter, status int, message, correlationID string) {
	errorType := "internal_error"
	switch status {
	case http.StatusBadRequest:
		errorType = "bad_request"
	case http.StatusNotFound:
		errorType = "not_found"
	case http.StatusUnauthorized:
		errorType = "unauthorized"
	case http.StatusForbidden:
		errorType = "forbidden"
	case http.StatusConflict:
		errorType = "conflict"
	case http.StatusUnprocessableEntity:
		errorType = "validation_error"
	}

	WriteJSON(w, status, ErrorResponse{
		Error:         errorType,
		Message:       message,
		CorrelationID: correlationID,
	})
}

// DecodeJSON decodes JSON from the request body
func DecodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Success       bool        `json:"success"`
	Message       string      `json:"message,omitempty"`
	Data          interface{} `json:"data,omitempty"`
	CorrelationID string      `json:"correlation_id"`
}

// WriteSuccess writes a generic success response
func WriteSuccess(w http.ResponseWriter, status int, message string, data interface{}, correlationID string) {
	WriteJSON(w, status, SuccessResponse{
		Success:       true,
		Message:       message,
		Data:          data,
		CorrelationID: correlationID,
	})
}
