// Package tests contains comprehensive tests for the CJADC2 platform
package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDB provides a simple in-memory mock for database operations
type MockDB struct {
	mu              sync.RWMutex
	idempotencyKeys map[string]*IdempotencyKey
	detections      map[string]*DetectionRecord
	effects         map[string]*EffectRecord
	messageIDs      map[string]bool // Track seen message IDs
}

// IdempotencyKey represents an idempotency key record
type IdempotencyKey struct {
	KeyHash   string
	MessageID string
	Result    json.RawMessage
	CreatedAt time.Time
	ExpiresAt time.Time
}

// DetectionRecord represents a detection in the database
type DetectionRecord struct {
	DetectionID   string
	MessageID     string
	CorrelationID string
	TrackID       string
	SensorID      string
	CreatedAt     time.Time
}

// EffectRecord represents an effect in the database
type EffectRecord struct {
	EffectID      string
	MessageID     string
	DecisionID    string
	ProposalID    string
	IdempotentKey string
	Status        string
	ExecutedAt    time.Time
	CreatedAt     time.Time
}

// NewMockDB creates a new mock database
func NewMockDB() *MockDB {
	return &MockDB{
		idempotencyKeys: make(map[string]*IdempotencyKey),
		detections:      make(map[string]*DetectionRecord),
		effects:         make(map[string]*EffectRecord),
		messageIDs:      make(map[string]bool),
	}
}

// InsertIdempotencyKey inserts an idempotency key or returns an error if it exists
func (db *MockDB) InsertIdempotencyKey(key *IdempotencyKey) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.idempotencyKeys[key.KeyHash]; exists {
		return fmt.Errorf("duplicate key: idempotency key '%s' already exists", key.KeyHash)
	}

	db.idempotencyKeys[key.KeyHash] = key
	return nil
}

// GetIdempotencyKey retrieves an idempotency key
func (db *MockDB) GetIdempotencyKey(keyHash string) (*IdempotencyKey, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	key, exists := db.idempotencyKeys[keyHash]
	if !exists {
		return nil, nil
	}

	// Check expiration
	if key.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}

	return key, nil
}

// InsertDetection inserts a detection record with uniqueness check on message_id
func (db *MockDB) InsertDetection(det *DetectionRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Check for duplicate message_id
	if db.messageIDs[det.MessageID] {
		return fmt.Errorf("duplicate key: message_id '%s' already exists", det.MessageID)
	}

	db.detections[det.DetectionID] = det
	db.messageIDs[det.MessageID] = true
	return nil
}

// GetDetectionByMessageID retrieves a detection by message ID
func (db *MockDB) GetDetectionByMessageID(messageID string) (*DetectionRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, det := range db.detections {
		if det.MessageID == messageID {
			return det, nil
		}
	}
	return nil, nil
}

// InsertEffect inserts an effect record with uniqueness check on idempotent_key
func (db *MockDB) InsertEffect(effect *EffectRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Check for duplicate idempotent_key
	for _, e := range db.effects {
		if e.IdempotentKey == effect.IdempotentKey {
			return fmt.Errorf("duplicate key: idempotent_key '%s' already exists", effect.IdempotentKey)
		}
	}

	db.effects[effect.EffectID] = effect
	return nil
}

// GetEffectByIdempotentKey retrieves an effect by idempotent key
func (db *MockDB) GetEffectByIdempotentKey(idempotentKey string) (*EffectRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, effect := range db.effects {
		if effect.IdempotentKey == idempotentKey {
			return effect, nil
		}
	}
	return nil, nil
}

// CountDetections returns the number of detections
func (db *MockDB) CountDetections() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.detections)
}

// CountEffects returns the number of effects
func (db *MockDB) CountEffects() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.effects)
}

// IdempotencyHandler handles idempotent operations
type IdempotencyHandler struct {
	db *MockDB
}

// NewIdempotencyHandler creates a new idempotency handler
func NewIdempotencyHandler(db *MockDB) *IdempotencyHandler {
	return &IdempotencyHandler{db: db}
}

// ProcessMessage processes a message idempotently using message_id
func (h *IdempotencyHandler) ProcessMessage(ctx context.Context, messageID string, process func() (interface{}, error)) (interface{}, bool, error) {
	// Check if we've already processed this message
	existing, err := h.db.GetDetectionByMessageID(messageID)
	if err != nil {
		return nil, false, err
	}

	if existing != nil {
		// Return cached result - message was already processed
		return existing, true, nil
	}

	// Process the message
	result, err := process()
	if err != nil {
		return nil, false, err
	}

	return result, false, nil
}

// ExecuteEffect executes an effect idempotently using idempotent_key
func (h *IdempotencyHandler) ExecuteEffect(ctx context.Context, idempotentKey string, execute func() (*EffectRecord, error)) (*EffectRecord, bool, error) {
	// Check if effect was already executed
	existing, err := h.db.GetEffectByIdempotentKey(idempotentKey)
	if err != nil {
		return nil, false, err
	}

	if existing != nil {
		// Return existing effect - skip execution
		return existing, true, nil
	}

	// Execute the effect
	result, err := execute()
	if err != nil {
		return nil, false, err
	}

	return result, false, nil
}

// TestIdempotentMessageProcessing tests that processing the same message ID twice doesn't create duplicates
func TestIdempotentMessageProcessing(t *testing.T) {
	db := NewMockDB()

	messageID := uuid.New().String()
	correlationID := uuid.New().String()

	// First insert should succeed
	det1 := &DetectionRecord{
		DetectionID:   uuid.New().String(),
		MessageID:     messageID,
		CorrelationID: correlationID,
		TrackID:       "track-001",
		SensorID:      "sensor-001",
		CreatedAt:     time.Now(),
	}

	err := db.InsertDetection(det1)
	require.NoError(t, err)
	assert.Equal(t, 1, db.CountDetections())

	// Second insert with same message ID should fail
	det2 := &DetectionRecord{
		DetectionID:   uuid.New().String(), // Different detection ID
		MessageID:     messageID,           // Same message ID
		CorrelationID: correlationID,
		TrackID:       "track-001",
		SensorID:      "sensor-001",
		CreatedAt:     time.Now(),
	}

	err = db.InsertDetection(det2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")

	// Count should still be 1
	assert.Equal(t, 1, db.CountDetections())
}

// TestIdempotentEffectExecution tests that effect execution with same idempotent_key is skipped
func TestIdempotentEffectExecution(t *testing.T) {
	db := NewMockDB()

	idempotentKey := "effect:decision-001:prop-001"

	// First effect execution should succeed
	effect1 := &EffectRecord{
		EffectID:      uuid.New().String(),
		MessageID:     uuid.New().String(),
		DecisionID:    "decision-001",
		ProposalID:    "prop-001",
		IdempotentKey: idempotentKey,
		Status:        "executed",
		ExecutedAt:    time.Now(),
		CreatedAt:     time.Now(),
	}

	err := db.InsertEffect(effect1)
	require.NoError(t, err)
	assert.Equal(t, 1, db.CountEffects())

	// Second effect with same idempotent_key should fail
	effect2 := &EffectRecord{
		EffectID:      uuid.New().String(),
		MessageID:     uuid.New().String(),
		DecisionID:    "decision-001",
		ProposalID:    "prop-001",
		IdempotentKey: idempotentKey, // Same idempotent key
		Status:        "executed",
		ExecutedAt:    time.Now(),
		CreatedAt:     time.Now(),
	}

	err = db.InsertEffect(effect2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")

	// Count should still be 1
	assert.Equal(t, 1, db.CountEffects())
}

// TestIdempotencyKeyExpiration tests that expired idempotency keys are not returned
func TestIdempotencyKeyExpiration(t *testing.T) {
	db := NewMockDB()

	// Insert expired key
	expiredKey := &IdempotencyKey{
		KeyHash:   "expired-hash",
		MessageID: uuid.New().String(),
		Result:    json.RawMessage(`{"status": "processed"}`),
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour), // Expired
	}

	err := db.InsertIdempotencyKey(expiredKey)
	require.NoError(t, err)

	// Retrieval should return nil (expired)
	retrieved, err := db.GetIdempotencyKey(expiredKey.KeyHash)
	require.NoError(t, err)
	assert.Nil(t, retrieved)

	// Insert valid key
	validKey := &IdempotencyKey{
		KeyHash:   "valid-hash",
		MessageID: uuid.New().String(),
		Result:    json.RawMessage(`{"status": "processed"}`),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // Not expired
	}

	err = db.InsertIdempotencyKey(validKey)
	require.NoError(t, err)

	// Retrieval should return the key
	retrieved, err = db.GetIdempotencyKey(validKey.KeyHash)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, validKey.KeyHash, retrieved.KeyHash)
}

// TestIdempotencyHandlerProcessMessage tests the ProcessMessage method
func TestIdempotencyHandlerProcessMessage(t *testing.T) {
	db := NewMockDB()
	handler := NewIdempotencyHandler(db)

	messageID := uuid.New().String()
	processCount := 0

	// First call should process
	result1, wasSkipped1, err := handler.ProcessMessage(context.Background(), messageID, func() (interface{}, error) {
		processCount++
		det := &DetectionRecord{
			DetectionID:   uuid.New().String(),
			MessageID:     messageID,
			CorrelationID: uuid.New().String(),
			TrackID:       "track-001",
			SensorID:      "sensor-001",
			CreatedAt:     time.Now(),
		}
		err := db.InsertDetection(det)
		return det, err
	})

	require.NoError(t, err)
	assert.False(t, wasSkipped1)
	assert.NotNil(t, result1)
	assert.Equal(t, 1, processCount)

	// Second call should return cached result
	result2, wasSkipped2, err := handler.ProcessMessage(context.Background(), messageID, func() (interface{}, error) {
		processCount++
		return nil, nil
	})

	require.NoError(t, err)
	assert.True(t, wasSkipped2)
	assert.NotNil(t, result2)
	assert.Equal(t, 1, processCount) // Should not have been incremented
}

// TestIdempotencyHandlerExecuteEffect tests the ExecuteEffect method
func TestIdempotencyHandlerExecuteEffect(t *testing.T) {
	db := NewMockDB()
	handler := NewIdempotencyHandler(db)

	idempotentKey := "effect:decision-001:prop-001"
	executeCount := 0

	// First call should execute
	result1, wasSkipped1, err := handler.ExecuteEffect(context.Background(), idempotentKey, func() (*EffectRecord, error) {
		executeCount++
		effect := &EffectRecord{
			EffectID:      uuid.New().String(),
			MessageID:     uuid.New().String(),
			DecisionID:    "decision-001",
			ProposalID:    "prop-001",
			IdempotentKey: idempotentKey,
			Status:        "executed",
			ExecutedAt:    time.Now(),
			CreatedAt:     time.Now(),
		}
		err := db.InsertEffect(effect)
		return effect, err
	})

	require.NoError(t, err)
	assert.False(t, wasSkipped1)
	assert.NotNil(t, result1)
	assert.Equal(t, 1, executeCount)

	// Second call should return cached result and skip execution
	result2, wasSkipped2, err := handler.ExecuteEffect(context.Background(), idempotentKey, func() (*EffectRecord, error) {
		executeCount++
		return nil, nil
	})

	require.NoError(t, err)
	assert.True(t, wasSkipped2)
	assert.NotNil(t, result2)
	assert.Equal(t, 1, executeCount) // Should not have been incremented
}

// TestConcurrentIdempotencyChecks tests concurrent access to idempotency checks
func TestConcurrentIdempotencyChecks(t *testing.T) {
	db := NewMockDB()

	messageID := uuid.New().String()
	successCount := 0
	errorCount := 0
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Launch 10 concurrent insert attempts for the same message ID
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			det := &DetectionRecord{
				DetectionID:   uuid.New().String(),
				MessageID:     messageID,
				CorrelationID: uuid.New().String(),
				TrackID:       "track-001",
				SensorID:      fmt.Sprintf("sensor-%03d", index),
				CreatedAt:     time.Now(),
			}

			err := db.InsertDetection(det)

			mu.Lock()
			defer mu.Unlock()

			if err == nil {
				successCount++
			} else {
				errorCount++
			}
		}(i)
	}

	wg.Wait()

	// Only one should succeed
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 9, errorCount)
	assert.Equal(t, 1, db.CountDetections())
}

// TestConcurrentEffectExecution tests concurrent effect execution attempts
func TestConcurrentEffectExecution(t *testing.T) {
	db := NewMockDB()

	idempotentKey := "effect:decision-concurrent:prop-concurrent"
	successCount := 0
	errorCount := 0
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Launch 10 concurrent effect execution attempts for the same idempotent key
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			effect := &EffectRecord{
				EffectID:      uuid.New().String(),
				MessageID:     uuid.New().String(),
				DecisionID:    "decision-concurrent",
				ProposalID:    "prop-concurrent",
				IdempotentKey: idempotentKey,
				Status:        "executed",
				ExecutedAt:    time.Now(),
				CreatedAt:     time.Now(),
			}

			err := db.InsertEffect(effect)

			mu.Lock()
			defer mu.Unlock()

			if err == nil {
				successCount++
			} else {
				errorCount++
			}
		}(i)
	}

	wg.Wait()

	// Only one should succeed
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 9, errorCount)
	assert.Equal(t, 1, db.CountEffects())
}

// TestIdempotencyKeyUniqueness tests uniqueness of idempotency keys
func TestIdempotencyKeyUniqueness(t *testing.T) {
	db := NewMockDB()

	keyHash := "unique-key-hash"

	// First insert should succeed
	key1 := &IdempotencyKey{
		KeyHash:   keyHash,
		MessageID: uuid.New().String(),
		Result:    json.RawMessage(`{"result": 1}`),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := db.InsertIdempotencyKey(key1)
	require.NoError(t, err)

	// Second insert with same key hash should fail
	key2 := &IdempotencyKey{
		KeyHash:   keyHash,
		MessageID: uuid.New().String(),
		Result:    json.RawMessage(`{"result": 2}`),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err = db.InsertIdempotencyKey(key2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")
}

// TestDifferentIdempotencyKeys tests that different keys can coexist
func TestDifferentIdempotencyKeys(t *testing.T) {
	db := NewMockDB()

	// Insert multiple different keys
	keys := []string{"key-1", "key-2", "key-3", "key-4", "key-5"}

	for _, keyHash := range keys {
		key := &IdempotencyKey{
			KeyHash:   keyHash,
			MessageID: uuid.New().String(),
			Result:    json.RawMessage(fmt.Sprintf(`{"key": "%s"}`, keyHash)),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		err := db.InsertIdempotencyKey(key)
		require.NoError(t, err)
	}

	// All keys should be retrievable
	for _, keyHash := range keys {
		retrieved, err := db.GetIdempotencyKey(keyHash)
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, keyHash, retrieved.KeyHash)
	}
}

// TestEffectIdempotentKeyGeneration tests generation of idempotent keys for effects
func TestEffectIdempotentKeyGeneration(t *testing.T) {
	tests := []struct {
		name       string
		decisionID string
		proposalID string
		trackID    string
		expected   string
	}{
		{
			name:       "basic key generation",
			decisionID: "decision-001",
			proposalID: "proposal-001",
			trackID:    "track-001",
			expected:   "effect:decision-001:proposal-001:track-001",
		},
		{
			name:       "with UUIDs",
			decisionID: "d-550e8400-e29b-41d4-a716-446655440000",
			proposalID: "p-550e8400-e29b-41d4-a716-446655440001",
			trackID:    "t-550e8400-e29b-41d4-a716-446655440002",
			expected:   "effect:d-550e8400-e29b-41d4-a716-446655440000:p-550e8400-e29b-41d4-a716-446655440001:t-550e8400-e29b-41d4-a716-446655440002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := generateEffectIdempotentKey(tt.decisionID, tt.proposalID, tt.trackID)
			assert.Equal(t, tt.expected, key)
		})
	}
}

// generateEffectIdempotentKey generates an idempotent key for an effect
func generateEffectIdempotentKey(decisionID, proposalID, trackID string) string {
	return fmt.Sprintf("effect:%s:%s:%s", decisionID, proposalID, trackID)
}

// TestMessageIDUniquenessAcrossTypes tests message ID uniqueness is enforced
func TestMessageIDUniquenessAcrossTypes(t *testing.T) {
	db := NewMockDB()

	messageID := uuid.New().String()

	// Insert a detection with the message ID
	det := &DetectionRecord{
		DetectionID:   uuid.New().String(),
		MessageID:     messageID,
		CorrelationID: uuid.New().String(),
		TrackID:       "track-001",
		SensorID:      "sensor-001",
		CreatedAt:     time.Now(),
	}

	err := db.InsertDetection(det)
	require.NoError(t, err)

	// Attempting to insert another detection with same message ID should fail
	det2 := &DetectionRecord{
		DetectionID:   uuid.New().String(),
		MessageID:     messageID,
		CorrelationID: uuid.New().String(),
		TrackID:       "track-002",
		SensorID:      "sensor-002",
		CreatedAt:     time.Now(),
	}

	err = db.InsertDetection(det2)
	assert.Error(t, err)
}

// TestIdempotencyWithRetry tests that retry attempts respect idempotency
func TestIdempotencyWithRetry(t *testing.T) {
	db := NewMockDB()
	handler := NewIdempotencyHandler(db)

	idempotentKey := "effect:retry-test"
	attemptCount := 0

	// Simulate multiple retry attempts
	for i := 0; i < 5; i++ {
		result, wasSkipped, err := handler.ExecuteEffect(context.Background(), idempotentKey, func() (*EffectRecord, error) {
			attemptCount++
			effect := &EffectRecord{
				EffectID:      uuid.New().String(),
				MessageID:     uuid.New().String(),
				DecisionID:    "decision-retry",
				ProposalID:    "prop-retry",
				IdempotentKey: idempotentKey,
				Status:        "executed",
				ExecutedAt:    time.Now(),
				CreatedAt:     time.Now(),
			}
			return effect, db.InsertEffect(effect)
		})

		require.NoError(t, err)
		assert.NotNil(t, result)

		if i == 0 {
			// First attempt should execute
			assert.False(t, wasSkipped)
		} else {
			// Subsequent attempts should be skipped
			assert.True(t, wasSkipped)
		}
	}

	// Only one attempt should have executed
	assert.Equal(t, 1, attemptCount)
	assert.Equal(t, 1, db.CountEffects())
}

// TestIdempotencyKeyResult tests that result is properly stored and retrieved
func TestIdempotencyKeyResult(t *testing.T) {
	db := NewMockDB()

	expectedResult := map[string]interface{}{
		"status":  "success",
		"trackID": "track-001",
		"action":  "engaged",
	}
	resultJSON, err := json.Marshal(expectedResult)
	require.NoError(t, err)

	key := &IdempotencyKey{
		KeyHash:   "result-test-key",
		MessageID: uuid.New().String(),
		Result:    resultJSON,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err = db.InsertIdempotencyKey(key)
	require.NoError(t, err)

	// Retrieve and verify result
	retrieved, err := db.GetIdempotencyKey(key.KeyHash)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	var actualResult map[string]interface{}
	err = json.Unmarshal(retrieved.Result, &actualResult)
	require.NoError(t, err)

	assert.Equal(t, expectedResult["status"], actualResult["status"])
	assert.Equal(t, expectedResult["trackID"], actualResult["trackID"])
	assert.Equal(t, expectedResult["action"], actualResult["action"])
}

// MockDBWithConstraints provides more realistic PostgreSQL constraint behavior
type MockDBWithConstraints struct {
	MockDB
	constraintViolations map[string]string // Map of constraint name to column
}

// NewMockDBWithConstraints creates a mock DB that simulates PostgreSQL constraints
func NewMockDBWithConstraints() *MockDBWithConstraints {
	return &MockDBWithConstraints{
		MockDB: *NewMockDB(),
		constraintViolations: map[string]string{
			"detections_message_id_key":    "message_id",
			"effects_idempotent_key_key":   "idempotent_key",
			"idempotency_keys_pkey":        "key_hash",
			"proposals_message_id_key":     "message_id",
			"decisions_message_id_key":     "message_id",
		},
	}
}

// TestDatabaseConstraintEnforcement tests constraint error handling
func TestDatabaseConstraintEnforcement(t *testing.T) {
	tests := []struct {
		name               string
		constraintName     string
		constraintColumn   string
		duplicateValue     string
		expectedErrorPart  string
	}{
		{
			name:              "detection message_id unique constraint",
			constraintName:    "detections_message_id_key",
			constraintColumn:  "message_id",
			duplicateValue:    "msg-001",
			expectedErrorPart: "duplicate key",
		},
		{
			name:              "effect idempotent_key unique constraint",
			constraintName:    "effects_idempotent_key_key",
			constraintColumn:  "idempotent_key",
			duplicateValue:    "effect:dec:prop",
			expectedErrorPart: "duplicate key",
		},
		{
			name:              "idempotency_keys primary key constraint",
			constraintName:    "idempotency_keys_pkey",
			constraintColumn:  "key_hash",
			duplicateValue:    "hash-123",
			expectedErrorPart: "duplicate key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test verifies that the mock correctly simulates constraint violations
			db := NewMockDB()

			switch tt.constraintColumn {
			case "message_id":
				det1 := &DetectionRecord{
					DetectionID:   uuid.New().String(),
					MessageID:     tt.duplicateValue,
					CorrelationID: uuid.New().String(),
					TrackID:       "track-001",
					SensorID:      "sensor-001",
					CreatedAt:     time.Now(),
				}
				err := db.InsertDetection(det1)
				require.NoError(t, err)

				det2 := &DetectionRecord{
					DetectionID:   uuid.New().String(),
					MessageID:     tt.duplicateValue,
					CorrelationID: uuid.New().String(),
					TrackID:       "track-002",
					SensorID:      "sensor-002",
					CreatedAt:     time.Now(),
				}
				err = db.InsertDetection(det2)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorPart)

			case "idempotent_key":
				effect1 := &EffectRecord{
					EffectID:      uuid.New().String(),
					MessageID:     uuid.New().String(),
					DecisionID:    "dec-001",
					ProposalID:    "prop-001",
					IdempotentKey: tt.duplicateValue,
					Status:        "executed",
					ExecutedAt:    time.Now(),
					CreatedAt:     time.Now(),
				}
				err := db.InsertEffect(effect1)
				require.NoError(t, err)

				effect2 := &EffectRecord{
					EffectID:      uuid.New().String(),
					MessageID:     uuid.New().String(),
					DecisionID:    "dec-002",
					ProposalID:    "prop-002",
					IdempotentKey: tt.duplicateValue,
					Status:        "executed",
					ExecutedAt:    time.Now(),
					CreatedAt:     time.Now(),
				}
				err = db.InsertEffect(effect2)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorPart)

			case "key_hash":
				key1 := &IdempotencyKey{
					KeyHash:   tt.duplicateValue,
					MessageID: uuid.New().String(),
					Result:    json.RawMessage(`{}`),
					CreatedAt: time.Now(),
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				err := db.InsertIdempotencyKey(key1)
				require.NoError(t, err)

				key2 := &IdempotencyKey{
					KeyHash:   tt.duplicateValue,
					MessageID: uuid.New().String(),
					Result:    json.RawMessage(`{}`),
					CreatedAt: time.Now(),
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				err = db.InsertIdempotencyKey(key2)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorPart)
			}
		})
	}
}

// TestIdempotencyAcrossChain tests idempotency handling across the message processing chain
func TestIdempotencyAcrossChain(t *testing.T) {
	db := NewMockDB()

	// Simulate a message flowing through the pipeline with the same correlation ID
	correlationID := uuid.New().String()
	detectionMessageID := uuid.New().String()
	trackMessageID := uuid.New().String()
	proposalMessageID := uuid.New().String()
	decisionMessageID := uuid.New().String()
	effectMessageID := uuid.New().String()

	// 1. Detection stage - first attempt
	det1 := &DetectionRecord{
		DetectionID:   uuid.New().String(),
		MessageID:     detectionMessageID,
		CorrelationID: correlationID,
		TrackID:       "track-001",
		SensorID:      "sensor-001",
		CreatedAt:     time.Now(),
	}
	err := db.InsertDetection(det1)
	require.NoError(t, err)

	// 2. Detection stage - retry attempt (should fail idempotently)
	det1Retry := &DetectionRecord{
		DetectionID:   uuid.New().String(),
		MessageID:     detectionMessageID, // Same message ID
		CorrelationID: correlationID,
		TrackID:       "track-001",
		SensorID:      "sensor-001",
		CreatedAt:     time.Now(),
	}
	err = db.InsertDetection(det1Retry)
	assert.Error(t, err)

	// Verify only one detection exists
	assert.Equal(t, 1, db.CountDetections())

	// 3. Effect stage - with unique idempotent key
	idempotentKey := generateEffectIdempotentKey(decisionMessageID, proposalMessageID, "track-001")

	effect1 := &EffectRecord{
		EffectID:      uuid.New().String(),
		MessageID:     effectMessageID,
		DecisionID:    decisionMessageID,
		ProposalID:    proposalMessageID,
		IdempotentKey: idempotentKey,
		Status:        "executed",
		ExecutedAt:    time.Now(),
		CreatedAt:     time.Now(),
	}
	err = db.InsertEffect(effect1)
	require.NoError(t, err)

	// 4. Effect stage - retry attempt (should fail idempotently)
	effect1Retry := &EffectRecord{
		EffectID:      uuid.New().String(),
		MessageID:     uuid.New().String(), // Different message ID
		DecisionID:    decisionMessageID,
		ProposalID:    proposalMessageID,
		IdempotentKey: idempotentKey, // Same idempotent key
		Status:        "executed",
		ExecutedAt:    time.Now(),
		CreatedAt:     time.Now(),
	}
	err = db.InsertEffect(effect1Retry)
	assert.Error(t, err)

	// Verify only one effect exists
	assert.Equal(t, 1, db.CountEffects())

	// Suppressing unused variable warnings for message IDs used to show the chain
	_ = trackMessageID
}

// ErrDuplicateKey simulates a PostgreSQL duplicate key error
type ErrDuplicateKey struct {
	Constraint string
	Column     string
	Value      string
}

func (e *ErrDuplicateKey) Error() string {
	return fmt.Sprintf("duplicate key value violates unique constraint %q (column: %s, value: %s)",
		e.Constraint, e.Column, e.Value)
}

// IsDuplicateKeyError checks if an error is a duplicate key violation
func IsDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ErrDuplicateKey)
	return ok || err == sql.ErrNoRows // sql.ErrNoRows is used as a placeholder
}

// TestIsDuplicateKeyError tests detection of duplicate key errors
func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		isDupKey  bool
	}{
		{
			name:     "duplicate key error",
			err:      &ErrDuplicateKey{Constraint: "test_pkey", Column: "id", Value: "123"},
			isDupKey: true,
		},
		{
			name:     "nil error",
			err:      nil,
			isDupKey: false,
		},
		{
			name:     "other error",
			err:      fmt.Errorf("connection refused"),
			isDupKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDuplicateKeyError(tt.err)
			assert.Equal(t, tt.isDupKey, result)
		})
	}
}
