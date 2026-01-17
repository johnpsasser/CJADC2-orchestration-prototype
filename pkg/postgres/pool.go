// Package postgres provides PostgreSQL connection pooling and query helpers
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agile-defense/cjadc2/pkg/messages"
)

// Pool wraps pgxpool.Pool with domain-specific query methods
type Pool struct {
	*pgxpool.Pool
}

// Config holds PostgreSQL connection configuration
type Config struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string

	// Pool settings
	MaxConns     int32
	MinConns     int32
	MaxConnLife  time.Duration
	MaxConnIdle  time.Duration
	HealthCheck  time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		Host:        "localhost",
		Port:        5432,
		Database:    "cjadc2",
		User:        "cjadc2",
		Password:    "cjadc2",
		SSLMode:     "disable",
		MaxConns:    25,
		MinConns:    5,
		MaxConnLife: time.Hour,
		MaxConnIdle: 30 * time.Minute,
		HealthCheck: time.Minute,
	}
}

// ConnectionString builds a PostgreSQL connection string
func (c Config) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode,
	)
}

// NewPool creates a new PostgreSQL connection pool
func NewPool(ctx context.Context, cfg Config) (*Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLife
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdle
	poolCfg.HealthCheckPeriod = cfg.HealthCheck

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// NewPoolFromURL creates a pool from a connection URL
func NewPoolFromURL(ctx context.Context, url string) (*Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// TrackRow represents a track stored in the database
type TrackRow struct {
	TrackID        string          `json:"track_id"`
	ExternalID     string          `json:"external_track_id"`
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

// TrackFilter defines filter options for track queries
type TrackFilter struct {
	Classification string
	ThreatLevel    string
	Type           string
	Since          *time.Time
	Limit          int
	Offset         int
}

// ListTracks retrieves tracks with optional filtering
func (p *Pool) ListTracks(ctx context.Context, filter TrackFilter) ([]TrackRow, error) {
	query := `
		SELECT
			track_id, external_track_id, classification, type, threat_level,
			position_lat, position_lon, position_alt,
			velocity_speed, velocity_heading,
			confidence, sources, detection_count,
			first_seen, last_updated
		FROM tracks
		WHERE state = 'active'
	`
	args := []interface{}{}
	argNum := 1

	if filter.Classification != "" {
		query += fmt.Sprintf(" AND classification = $%d", argNum)
		args = append(args, filter.Classification)
		argNum++
	}

	if filter.ThreatLevel != "" {
		query += fmt.Sprintf(" AND threat_level = $%d", argNum)
		args = append(args, filter.ThreatLevel)
		argNum++
	}

	if filter.Type != "" {
		query += fmt.Sprintf(" AND type = $%d", argNum)
		args = append(args, filter.Type)
		argNum++
	}

	if filter.Since != nil {
		query += fmt.Sprintf(" AND last_updated >= $%d", argNum)
		args = append(args, *filter.Since)
		argNum++
	}

	query += " ORDER BY last_updated DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filter.Limit)
		argNum++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filter.Offset)
	}

	rows, err := p.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tracks: %w", err)
	}
	defer rows.Close()

	var tracks []TrackRow
	for rows.Next() {
		var t TrackRow
		var posLat, posLon float64
		var posAlt, velSpeed, velHeading *float64

		err := rows.Scan(
			&t.TrackID, &t.ExternalID, &t.Classification, &t.Type, &t.ThreatLevel,
			&posLat, &posLon, &posAlt,
			&velSpeed, &velHeading,
			&t.Confidence, &t.Sources, &t.DetectionCount,
			&t.FirstSeen, &t.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan track: %w", err)
		}

		// Build position JSON
		pos := map[string]interface{}{"lat": posLat, "lon": posLon}
		if posAlt != nil {
			pos["alt"] = *posAlt
		}
		t.Position, _ = json.Marshal(pos)

		// Build velocity JSON
		vel := map[string]interface{}{}
		if velSpeed != nil {
			vel["speed"] = *velSpeed
		}
		if velHeading != nil {
			vel["heading"] = *velHeading
		}
		t.Velocity, _ = json.Marshal(vel)

		tracks = append(tracks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tracks: %w", err)
	}

	return tracks, nil
}

// GetTrack retrieves a single track by ID
func (p *Pool) GetTrack(ctx context.Context, trackID string) (*TrackRow, error) {
	query := `
		SELECT
			track_id, external_track_id, classification, type, threat_level,
			position_lat, position_lon, position_alt,
			velocity_speed, velocity_heading,
			confidence, sources, detection_count,
			first_seen, last_updated
		FROM tracks
		WHERE external_track_id = $1
	`

	var t TrackRow
	var posLat, posLon float64
	var posAlt, velSpeed, velHeading *float64

	err := p.QueryRow(ctx, query, trackID).Scan(
		&t.TrackID, &t.ExternalID, &t.Classification, &t.Type, &t.ThreatLevel,
		&posLat, &posLon, &posAlt,
		&velSpeed, &velHeading,
		&t.Confidence, &t.Sources, &t.DetectionCount,
		&t.FirstSeen, &t.LastUpdated,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get track: %w", err)
	}

	// Build position JSON
	pos := map[string]interface{}{"lat": posLat, "lon": posLon}
	if posAlt != nil {
		pos["alt"] = *posAlt
	}
	t.Position, _ = json.Marshal(pos)

	// Build velocity JSON
	vel := map[string]interface{}{}
	if velSpeed != nil {
		vel["speed"] = *velSpeed
	}
	if velHeading != nil {
		vel["heading"] = *velHeading
	}
	t.Velocity, _ = json.Marshal(vel)

	return &t, nil
}

// UpsertTrack inserts or updates a track from a CorrelatedTrack message
func (p *Pool) UpsertTrack(ctx context.Context, track *messages.CorrelatedTrack) error {
	query := `
		INSERT INTO tracks (
			external_track_id, classification, type, threat_level,
			position_lat, position_lon, position_alt,
			velocity_speed, velocity_heading,
			confidence, sources, detection_count,
			first_seen, last_updated, state
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11, $12,
			$13, $14, 'active'
		)
		ON CONFLICT (external_track_id) DO UPDATE SET
			classification = EXCLUDED.classification,
			type = EXCLUDED.type,
			threat_level = EXCLUDED.threat_level,
			position_lat = EXCLUDED.position_lat,
			position_lon = EXCLUDED.position_lon,
			position_alt = EXCLUDED.position_alt,
			velocity_speed = EXCLUDED.velocity_speed,
			velocity_heading = EXCLUDED.velocity_heading,
			confidence = EXCLUDED.confidence,
			sources = EXCLUDED.sources,
			detection_count = tracks.detection_count + 1,
			last_updated = EXCLUDED.last_updated,
			state = 'active'
	`

	firstSeen := track.WindowStart
	if track.LastUpdated.Before(firstSeen) {
		firstSeen = track.LastUpdated
	}

	_, err := p.Exec(ctx, query,
		track.TrackID,
		track.Classification,
		track.Type,
		track.ThreatLevel,
		track.Position.Lat,
		track.Position.Lon,
		track.Position.Alt,
		track.Velocity.Speed,
		track.Velocity.Heading,
		track.Confidence,
		track.Sources,
		track.DetectionCount,
		firstSeen,
		track.LastUpdated,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert track: %w", err)
	}

	return nil
}

// DetectionRow represents a detection stored in the database
type DetectionRow struct {
	DetectionID   string          `json:"detection_id"`
	TrackID       string          `json:"track_id"`
	SensorID      string          `json:"sensor_id"`
	SensorType    string          `json:"sensor_type"`
	Position      json.RawMessage `json:"position"`
	Velocity      json.RawMessage `json:"velocity"`
	Confidence    float64         `json:"confidence"`
	Timestamp     time.Time       `json:"timestamp"`
}

// GetTrackHistory retrieves detection history for a track
func (p *Pool) GetTrackHistory(ctx context.Context, trackID string, limit int) ([]DetectionRow, error) {
	if limit <= 0 {
		limit = 100
	}

	// First get the internal UUID for the track
	var internalTrackID string
	err := p.QueryRow(ctx, "SELECT track_id FROM tracks WHERE external_track_id = $1", trackID).Scan(&internalTrackID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get track ID: %w", err)
	}

	query := `
		SELECT
			detection_id, sensor_id, sensor_type,
			position_lat, position_lon, position_alt,
			velocity_speed, velocity_heading,
			confidence, created_at
		FROM detections
		WHERE track_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := p.Query(ctx, query, internalTrackID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query detection history: %w", err)
	}
	defer rows.Close()

	var detections []DetectionRow
	for rows.Next() {
		var d DetectionRow
		var posLat, posLon float64
		var posAlt, velSpeed, velHeading *float64

		err := rows.Scan(
			&d.DetectionID, &d.SensorID, &d.SensorType,
			&posLat, &posLon, &posAlt,
			&velSpeed, &velHeading,
			&d.Confidence, &d.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan detection: %w", err)
		}

		d.TrackID = trackID

		// Build position JSON
		pos := map[string]interface{}{"lat": posLat, "lon": posLon}
		if posAlt != nil {
			pos["alt"] = *posAlt
		}
		d.Position, _ = json.Marshal(pos)

		// Build velocity JSON
		vel := map[string]interface{}{}
		if velSpeed != nil {
			vel["speed"] = *velSpeed
		}
		if velHeading != nil {
			vel["heading"] = *velHeading
		}
		d.Velocity, _ = json.Marshal(vel)

		detections = append(detections, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating detections: %w", err)
	}

	return detections, nil
}

// ProposalRow represents a proposal stored in the database
type ProposalRow struct {
	ProposalID     string          `json:"proposal_id"`
	TrackID        string          `json:"track_id"`
	ActionType     string          `json:"action_type"`
	Priority       int             `json:"priority"`
	ThreatLevel    string          `json:"threat_level"`
	Rationale      string          `json:"rationale"`
	Status         string          `json:"status"`
	ExpiresAt      time.Time       `json:"expires_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	PolicyDecision json.RawMessage `json:"policy_decision"`
}

// ProposalFilter defines filter options for proposal queries
type ProposalFilter struct {
	Status      string
	TrackID     string
	ActionType  string
	ThreatLevel string
	Limit       int
	Offset      int
}

// ListProposals retrieves proposals with optional filtering
func (p *Pool) ListProposals(ctx context.Context, filter ProposalFilter) ([]ProposalRow, error) {
	query := `
		SELECT
			p.proposal_id, p.track_id as external_track_id, p.action_type, p.priority,
			p.threat_level, p.rationale, p.status, p.expires_at,
			p.created_at, p.updated_at, p.policy_decision as policy_result
		FROM proposals p
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.Status != "" {
		query += fmt.Sprintf(" AND p.status = $%d", argNum)
		args = append(args, filter.Status)
		argNum++
	}

	if filter.TrackID != "" {
		query += fmt.Sprintf(" AND p.track_id = $%d", argNum)
		args = append(args, filter.TrackID)
		argNum++
	}

	if filter.ActionType != "" {
		query += fmt.Sprintf(" AND p.action_type = $%d", argNum)
		args = append(args, filter.ActionType)
		argNum++
	}

	if filter.ThreatLevel != "" {
		query += fmt.Sprintf(" AND p.threat_level = $%d", argNum)
		args = append(args, filter.ThreatLevel)
		argNum++
	}

	query += " ORDER BY p.priority DESC, p.created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filter.Limit)
		argNum++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filter.Offset)
	}

	rows, err := p.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query proposals: %w", err)
	}
	defer rows.Close()

	var proposals []ProposalRow
	for rows.Next() {
		var pr ProposalRow
		err := rows.Scan(
			&pr.ProposalID, &pr.TrackID, &pr.ActionType, &pr.Priority,
			&pr.ThreatLevel, &pr.Rationale, &pr.Status, &pr.ExpiresAt,
			&pr.CreatedAt, &pr.UpdatedAt, &pr.PolicyDecision,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proposal: %w", err)
		}
		proposals = append(proposals, pr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating proposals: %w", err)
	}

	return proposals, nil
}

// GetProposal retrieves a single proposal by ID
func (p *Pool) GetProposal(ctx context.Context, proposalID string) (*ProposalRow, error) {
	query := `
		SELECT
			p.proposal_id, p.track_id as external_track_id, p.action_type, p.priority,
			p.threat_level, p.rationale, p.status, p.expires_at,
			p.created_at, p.updated_at, p.policy_decision as policy_result
		FROM proposals p
		WHERE p.proposal_id = $1
	`

	var pr ProposalRow
	err := p.QueryRow(ctx, query, proposalID).Scan(
		&pr.ProposalID, &pr.TrackID, &pr.ActionType, &pr.Priority,
		&pr.ThreatLevel, &pr.Rationale, &pr.Status, &pr.ExpiresAt,
		&pr.CreatedAt, &pr.UpdatedAt, &pr.PolicyDecision,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get proposal: %w", err)
	}

	return &pr, nil
}

// UpdateProposalStatus updates a proposal's status
func (p *Pool) UpdateProposalStatus(ctx context.Context, proposalID, status string) error {
	query := `
		UPDATE proposals
		SET status = $2, updated_at = $3
		WHERE proposal_id = $1
	`
	_, err := p.Exec(ctx, query, proposalID, status, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update proposal status: %w", err)
	}
	return nil
}

// DecisionRow represents a decision stored in the database
type DecisionRow struct {
	DecisionID   string    `json:"decision_id"`
	ProposalID   string    `json:"proposal_id"`
	TrackID      string    `json:"track_id"`
	ActionType   string    `json:"action_type"`
	Approved     bool      `json:"approved"`
	ApprovedBy   string    `json:"approved_by"`
	ApprovedAt   time.Time `json:"approved_at"`
	Reason       string    `json:"reason"`
	Conditions   []string  `json:"conditions"`
	CreatedAt    time.Time `json:"created_at"`
}

// DecisionFilter defines filter options for decision queries
type DecisionFilter struct {
	ProposalID string
	TrackID    string
	Approved   *bool
	ApprovedBy string
	Since      *time.Time
	Limit      int
	Offset     int
}

// ListDecisions retrieves decisions with optional filtering
func (p *Pool) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRow, error) {
	query := `
		SELECT
			d.decision_id, d.proposal_id, d.track_id as external_track_id, d.action_type,
			d.approved, d.approved_by, d.approved_at, d.reason, d.conditions,
			d.created_at
		FROM decisions d
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.ProposalID != "" {
		query += fmt.Sprintf(" AND d.proposal_id = $%d", argNum)
		args = append(args, filter.ProposalID)
		argNum++
	}

	if filter.TrackID != "" {
		query += fmt.Sprintf(" AND d.track_id = $%d", argNum)
		args = append(args, filter.TrackID)
		argNum++
	}

	if filter.Approved != nil {
		query += fmt.Sprintf(" AND d.approved = $%d", argNum)
		args = append(args, *filter.Approved)
		argNum++
	}

	if filter.ApprovedBy != "" {
		query += fmt.Sprintf(" AND d.approved_by = $%d", argNum)
		args = append(args, filter.ApprovedBy)
		argNum++
	}

	if filter.Since != nil {
		query += fmt.Sprintf(" AND d.approved_at >= $%d", argNum)
		args = append(args, *filter.Since)
		argNum++
	}

	query += " ORDER BY d.approved_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filter.Limit)
		argNum++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filter.Offset)
	}

	rows, err := p.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query decisions: %w", err)
	}
	defer rows.Close()

	var decisions []DecisionRow
	for rows.Next() {
		var d DecisionRow
		var reason *string
		err := rows.Scan(
			&d.DecisionID, &d.ProposalID, &d.TrackID, &d.ActionType,
			&d.Approved, &d.ApprovedBy, &d.ApprovedAt, &reason, &d.Conditions,
			&d.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan decision: %w", err)
		}
		if reason != nil {
			d.Reason = *reason
		}
		decisions = append(decisions, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating decisions: %w", err)
	}

	return decisions, nil
}

// InsertDecision inserts a new decision
func (p *Pool) InsertDecision(ctx context.Context, decision *messages.Decision) error {
	query := `
		INSERT INTO decisions (
			decision_id, message_id, correlation_id, proposal_id,
			approved, approved_by, approved_at, reason, conditions,
			action_type, track_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := p.Exec(ctx, query,
		decision.DecisionID, decision.Envelope.MessageID, decision.Envelope.CorrelationID,
		decision.ProposalID, decision.Approved, decision.ApprovedBy, decision.ApprovedAt,
		decision.Reason, decision.Conditions,
		decision.ActionType, decision.TrackID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert decision: %w", err)
	}

	return nil
}

// EffectRow represents an effect log stored in the database
type EffectRow struct {
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

// EffectFilter defines filter options for effect queries
type EffectFilter struct {
	DecisionID string
	ProposalID string
	TrackID    string
	ActionType string
	Status     string
	Since      *time.Time
	Limit      int
	Offset     int
}

// ListEffects retrieves effects with optional filtering
func (p *Pool) ListEffects(ctx context.Context, filter EffectFilter) ([]EffectRow, error) {
	query := `
		SELECT
			e.effect_id, e.decision_id, e.proposal_id, e.track_id as external_track_id,
			e.action_type, e.status, e.executed_at, e.result, e.idempotent_key
		FROM effects e
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.DecisionID != "" {
		query += fmt.Sprintf(" AND e.decision_id = $%d", argNum)
		args = append(args, filter.DecisionID)
		argNum++
	}

	if filter.ProposalID != "" {
		query += fmt.Sprintf(" AND e.proposal_id = $%d", argNum)
		args = append(args, filter.ProposalID)
		argNum++
	}

	if filter.TrackID != "" {
		query += fmt.Sprintf(" AND e.track_id = $%d", argNum)
		args = append(args, filter.TrackID)
		argNum++
	}

	if filter.ActionType != "" {
		query += fmt.Sprintf(" AND e.action_type = $%d", argNum)
		args = append(args, filter.ActionType)
		argNum++
	}

	if filter.Status != "" {
		query += fmt.Sprintf(" AND e.status = $%d", argNum)
		args = append(args, filter.Status)
		argNum++
	}

	if filter.Since != nil {
		query += fmt.Sprintf(" AND e.executed_at >= $%d", argNum)
		args = append(args, *filter.Since)
		argNum++
	}

	query += " ORDER BY e.executed_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filter.Limit)
		argNum++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filter.Offset)
	}

	rows, err := p.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query effects: %w", err)
	}
	defer rows.Close()

	var effects []EffectRow
	for rows.Next() {
		var e EffectRow
		var result *string
		var executedAt *time.Time
		err := rows.Scan(
			&e.EffectID, &e.DecisionID, &e.ProposalID, &e.TrackID,
			&e.ActionType, &e.Status, &executedAt, &result, &e.IdempotentKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan effect: %w", err)
		}
		if result != nil {
			e.Result = *result
		}
		if executedAt != nil {
			e.ExecutedAt = *executedAt
		}
		effects = append(effects, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating effects: %w", err)
	}

	return effects, nil
}

// StageMetrics represents metrics for a pipeline stage
type StageMetrics struct {
	Stage           string  `json:"stage"`
	MessagesTotal   int64   `json:"messages_total"`
	MessagesSuccess int64   `json:"messages_success"`
	MessagesFailed  int64   `json:"messages_failed"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	P99LatencyMs    float64 `json:"p99_latency_ms"`
	LastUpdated     time.Time `json:"last_updated"`
}

// GetStageMetrics retrieves metrics for all pipeline stages
func (p *Pool) GetStageMetrics(ctx context.Context) ([]StageMetrics, error) {
	query := `
		SELECT
			stage,
			COALESCE(SUM(processed_count), 0) as messages_total,
			COALESCE(SUM(success_count), 0) as messages_success,
			COALESCE(SUM(failure_count), 0) as messages_failed,
			COALESCE(AVG(p50_latency_ms), 0) as avg_latency_ms,
			COALESCE(MAX(p99_latency_ms), 0) as p99_latency_ms,
			MAX(created_at) as last_updated
		FROM stage_metrics
		GROUP BY stage
		ORDER BY stage
	`

	rows, err := p.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query stage metrics: %w", err)
	}
	defer rows.Close()

	var metrics []StageMetrics
	for rows.Next() {
		var m StageMetrics
		err := rows.Scan(
			&m.Stage, &m.MessagesTotal, &m.MessagesSuccess, &m.MessagesFailed,
			&m.AvgLatencyMs, &m.P99LatencyMs, &m.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stage metrics: %w", err)
		}
		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stage metrics: %w", err)
	}

	return metrics, nil
}

// LatencyMetrics represents end-to-end latency metrics
type LatencyMetrics struct {
	Window        string    `json:"window"`
	AvgLatencyMs  float64   `json:"avg_latency_ms"`
	MinLatencyMs  float64   `json:"min_latency_ms"`
	MaxLatencyMs  float64   `json:"max_latency_ms"`
	P50LatencyMs  float64   `json:"p50_latency_ms"`
	P95LatencyMs  float64   `json:"p95_latency_ms"`
	P99LatencyMs  float64   `json:"p99_latency_ms"`
	SampleCount   int64     `json:"sample_count"`
	CalculatedAt  time.Time `json:"calculated_at"`
}

// GetLatencyMetrics retrieves end-to-end latency metrics calculated from decision/effect data
func (p *Pool) GetLatencyMetrics(ctx context.Context, window string) (*LatencyMetrics, error) {
	if window == "" {
		window = "1h"
	}

	// Map window to interval
	intervalMap := map[string]string{
		"1m":  "1 minute",
		"5m":  "5 minutes",
		"15m": "15 minutes",
		"1h":  "1 hour",
		"6h":  "6 hours",
		"24h": "24 hours",
	}
	interval, ok := intervalMap[window]
	if !ok {
		interval = "1 hour"
	}

	// Calculate latency percentiles from effects -> decisions -> proposals chain
	query := fmt.Sprintf(`
		SELECT
			COALESCE(AVG(latency_ms), 0) as avg_latency_ms,
			COALESCE(MIN(latency_ms), 0) as min_latency_ms,
			COALESCE(MAX(latency_ms), 0) as max_latency_ms,
			COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY latency_ms), 0) as p50_latency_ms,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) as p95_latency_ms,
			COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) as p99_latency_ms,
			COUNT(*) as sample_count
		FROM (
			SELECT EXTRACT(EPOCH FROM (e.executed_at - p.created_at)) * 1000 as latency_ms
			FROM effects e
			JOIN decisions d ON e.decision_id = d.decision_id
			JOIN proposals p ON d.proposal_id = p.proposal_id
			WHERE e.executed_at IS NOT NULL
			  AND e.created_at >= NOW() - INTERVAL '%s'
		) latencies
	`, interval)

	var m LatencyMetrics
	err := p.QueryRow(ctx, query).Scan(
		&m.AvgLatencyMs,
		&m.MinLatencyMs,
		&m.MaxLatencyMs,
		&m.P50LatencyMs,
		&m.P95LatencyMs,
		&m.P99LatencyMs,
		&m.SampleCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get latency metrics: %w", err)
	}

	m.Window = window
	m.CalculatedAt = time.Now().UTC()

	return &m, nil
}

// RealTimeStageMetrics represents metrics for a stage calculated from actual data
type RealTimeStageMetrics struct {
	Stage       string
	Processed   int64
	Succeeded   int64
	Failed      int64
	LatencyP50  float64
	LatencyP95  float64
	LatencyP99  float64
	LastUpdated time.Time
}

// GetRealTimeStageMetrics calculates stage metrics from actual table data
func (p *Pool) GetRealTimeStageMetrics(ctx context.Context) ([]RealTimeStageMetrics, error) {
	stages := []RealTimeStageMetrics{}

	// Get message count for the last 5 minutes - SUM of detection_count represents actual message throughput
	var messageCount int64
	var trackLastUpdated time.Time
	err := p.QueryRow(ctx, `
		SELECT COALESCE(SUM(detection_count), 0), COALESCE(MAX(last_updated), NOW())
		FROM tracks
		WHERE last_updated >= NOW() - INTERVAL '5 minutes'
	`).Scan(&messageCount, &trackLastUpdated)
	if err != nil {
		messageCount = 0
		trackLastUpdated = time.Now()
	}

	// Get proposal count for the planner stage
	var proposalCount int64
	var proposalLastUpdated time.Time
	err = p.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(MAX(created_at), NOW())
		FROM proposals
		WHERE created_at >= NOW() - INTERVAL '5 minutes'
	`).Scan(&proposalCount, &proposalLastUpdated)
	if err != nil {
		proposalCount = 0
		proposalLastUpdated = time.Now()
	}

	// Sensor stage - use message count (SUM of detection_count = total messages processed)
	sensor := RealTimeStageMetrics{
		Stage:       "sensor",
		Processed:   messageCount,
		Succeeded:   messageCount,
		Failed:      0,
		LastUpdated: trackLastUpdated,
	}
	stages = append(stages, sensor)

	// Classifier stage - same throughput as sensor
	classifier := RealTimeStageMetrics{
		Stage:       "classifier",
		Processed:   messageCount,
		Succeeded:   messageCount,
		Failed:      0,
		LastUpdated: trackLastUpdated,
	}
	stages = append(stages, classifier)

	// Correlator stage - same throughput (tracks are persisted after correlation)
	correlator := RealTimeStageMetrics{
		Stage:       "correlator",
		Processed:   messageCount,
		Succeeded:   messageCount,
		Failed:      0,
		LastUpdated: trackLastUpdated,
	}
	stages = append(stages, correlator)

	// Planner stage - evaluates all messages, creates proposals for some
	// Processed = messages evaluated, Succeeded = messages processed, Failed = 0 (no failures)
	// Note: proposalCount is the output, not a success metric
	planner := RealTimeStageMetrics{
		Stage:       "planner",
		Processed:   messageCount,
		Succeeded:   messageCount,
		Failed:      0,
		LastUpdated: proposalLastUpdated,
	}
	stages = append(stages, planner)

	// Authorizer stage - receives proposals from planner
	// Processed = proposals received (matches planner output)
	// Succeeded = approved decisions, Failed = denied + expired, Pending = awaiting decision
	var authSucceeded, authFailed int64
	var authLastUpdated time.Time
	var authP50, authP95, authP99 float64
	err = p.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'approved' THEN 1 ELSE 0 END), 0) as succeeded,
			COALESCE(SUM(CASE WHEN status IN ('denied', 'expired') THEN 1 ELSE 0 END), 0) as failed,
			COALESCE(MAX(created_at), NOW()) as last_updated
		FROM proposals
		WHERE created_at >= NOW() - INTERVAL '5 minutes'
	`).Scan(&authSucceeded, &authFailed, &authLastUpdated)
	if err != nil {
		authSucceeded, authFailed = 0, 0
		authLastUpdated = time.Now()
	}

	// Calculate authorizer latency (proposal creation to decision)
	err = p.QueryRow(ctx, `
		SELECT
			COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY latency_ms), 0) as p50,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) as p95,
			COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) as p99
		FROM (
			SELECT EXTRACT(EPOCH FROM (d.approved_at - p.created_at)) * 1000 as latency_ms
			FROM decisions d
			JOIN proposals p ON d.proposal_id = p.proposal_id
			WHERE d.approved_at >= NOW() - INTERVAL '5 minutes'
		) latencies
	`).Scan(&authP50, &authP95, &authP99)
	if err != nil {
		authP50, authP95, authP99 = 0, 0, 0
	}

	authorizer := RealTimeStageMetrics{
		Stage:       "authorizer",
		Processed:   proposalCount, // Use proposalCount to match planner output
		Succeeded:   authSucceeded,
		Failed:      authFailed,
		LatencyP50:  authP50,
		LatencyP95:  authP95,
		LatencyP99:  authP99,
		LastUpdated: authLastUpdated,
	}
	stages = append(stages, authorizer)

	// Effector stage - effects executed with latency from decision to execution
	var effProcessed, effSucceeded, effFailed int64
	var effLastUpdated time.Time
	var effP50, effP95, effP99 float64
	err = p.QueryRow(ctx, `
		SELECT
			COUNT(*) as processed,
			COALESCE(SUM(CASE WHEN status = 'executed' THEN 1 ELSE 0 END), 0) as succeeded,
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) as failed,
			COALESCE(MAX(created_at), NOW()) as last_updated
		FROM effects
		WHERE created_at >= NOW() - INTERVAL '5 minutes'
	`).Scan(&effProcessed, &effSucceeded, &effFailed, &effLastUpdated)
	if err != nil {
		effProcessed, effSucceeded, effFailed = 0, 0, 0
		effLastUpdated = time.Now()
	}

	// Calculate effector latency (decision to effect execution)
	err = p.QueryRow(ctx, `
		SELECT
			COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY latency_ms), 0) as p50,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) as p95,
			COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) as p99
		FROM (
			SELECT EXTRACT(EPOCH FROM (e.executed_at - d.approved_at)) * 1000 as latency_ms
			FROM effects e
			JOIN decisions d ON e.decision_id = d.decision_id
			WHERE e.executed_at IS NOT NULL
			  AND e.created_at >= NOW() - INTERVAL '5 minutes'
		) latencies
	`).Scan(&effP50, &effP95, &effP99)
	if err != nil {
		effP50, effP95, effP99 = 0, 0, 0
	}

	effector := RealTimeStageMetrics{
		Stage:       "effector",
		Processed:   authSucceeded, // Effector receives approved decisions from authorizer
		Succeeded:   effSucceeded,
		Failed:      effFailed,
		LatencyP50:  effP50,
		LatencyP95:  effP95,
		LatencyP99:  effP99,
		LastUpdated: effLastUpdated,
	}
	stages = append(stages, effector)

	return stages, nil
}

// GetMessagesPerMinute calculates current message throughput rate
func (p *Pool) GetMessagesPerMinute(ctx context.Context) (float64, error) {
	// Calculate per-track detection rate and sum across all active tracks
	// Each track's rate = detection_count / track_age_seconds * 60
	// This gives the actual messages/minute based on observed behavior
	query := `
		SELECT COALESCE(SUM(
			detection_count::float / GREATEST(EXTRACT(EPOCH FROM (NOW() - first_seen)), 1) * 60
		), 0) as messages_per_minute
		FROM tracks
		WHERE last_updated >= NOW() - INTERVAL '1 minute'
		  AND first_seen IS NOT NULL
		  AND detection_count > 0
	`
	var rate float64
	err := p.QueryRow(ctx, query).Scan(&rate)
	if err != nil {
		return 0, fmt.Errorf("failed to get messages per minute: %w", err)
	}
	return rate, nil
}

// GetEndToEndLatencyMetrics returns real-time E2E latency percentiles
// Measures decision pipeline latency (proposal → effect) when available,
// falls back to track processing latency (first_seen → last_updated) otherwise
func (p *Pool) GetEndToEndLatencyMetrics(ctx context.Context) (p50, p95, p99 float64, err error) {
	// First try to get decision pipeline latency (proposal → effect)
	query := `
		SELECT
			COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY latency_ms), 0) as p50,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) as p95,
			COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) as p99
		FROM (
			SELECT EXTRACT(EPOCH FROM (e.executed_at - p.created_at)) * 1000 as latency_ms
			FROM effects e
			JOIN decisions d ON e.decision_id = d.decision_id
			JOIN proposals p ON d.proposal_id = p.proposal_id
			WHERE e.executed_at IS NOT NULL
			  AND e.created_at >= NOW() - INTERVAL '5 minutes'
		) latencies
	`
	err = p.QueryRow(ctx, query).Scan(&p50, &p95, &p99)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get E2E latency: %w", err)
	}

	// If no decision latency data, use track processing latency as fallback
	if p50 == 0 && p95 == 0 && p99 == 0 {
		trackQuery := `
			SELECT
				COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY latency_ms), 0) as p50,
				COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) as p95,
				COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) as p99
			FROM (
				SELECT EXTRACT(EPOCH FROM (last_updated - first_seen)) * 1000 as latency_ms
				FROM tracks
				WHERE last_updated >= NOW() - INTERVAL '5 minutes'
				  AND last_updated > first_seen
			) latencies
		`
		err = p.QueryRow(ctx, trackQuery).Scan(&p50, &p95, &p99)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to get track processing latency: %w", err)
		}
	}

	return p50, p95, p99, nil
}

// AuditEntry represents an audit trail entry for the frontend
type AuditEntry struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	ActionType string `json:"action_type"`
	UserID     string `json:"user_id"`
	TrackID    string `json:"track_id"`
	ProposalID string `json:"proposal_id"`
	DecisionID string `json:"decision_id"`
	EffectID   string `json:"effect_id"`
	Status     string `json:"status"`
	Details    string `json:"details"`
}

// AuditFilter defines filter options for audit queries
type AuditFilter struct {
	ActionType string
	UserID     string
	TrackID    string
	Limit      int
	Offset     int
}

// ListAuditEntries retrieves audit entries by querying the decision_audit_trail view
func (p *Pool) ListAuditEntries(ctx context.Context, filter AuditFilter) ([]AuditEntry, error) {
	// Query the decision_audit_trail view and map to AuditEntry format
	query := `
		SELECT
			d.decision_id,
			d.approved,
			d.approved_by,
			d.approved_at,
			d.reason,
			p.proposal_id,
			p.action_type,
			p.rationale,
			p.track_id as external_track_id,
			p.threat_level,
			e.effect_id,
			e.status as effect_status,
			e.executed_at
		FROM decisions d
		JOIN proposals p ON d.proposal_id = p.proposal_id
		LEFT JOIN effects e ON d.decision_id = e.decision_id
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.ActionType != "" {
		query += fmt.Sprintf(" AND p.action_type = $%d", argNum)
		args = append(args, filter.ActionType)
		argNum++
	}

	if filter.UserID != "" {
		query += fmt.Sprintf(" AND d.approved_by = $%d", argNum)
		args = append(args, filter.UserID)
		argNum++
	}

	if filter.TrackID != "" {
		query += fmt.Sprintf(" AND p.track_id = $%d", argNum)
		args = append(args, filter.TrackID)
		argNum++
	}

	query += " ORDER BY d.approved_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filter.Limit)
		argNum++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filter.Offset)
	}

	rows, err := p.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit entries: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var (
			decisionID    string
			approved      bool
			approvedBy    string
			approvedAt    time.Time
			reason        *string
			proposalID    string
			actionType    string
			rationale     *string
			trackID       string
			threatLevel   *string
			effectID      *string
			effectStatus  *string
			executedAt    *time.Time
		)

		err := rows.Scan(
			&decisionID, &approved, &approvedBy, &approvedAt, &reason,
			&proposalID, &actionType, &rationale, &trackID, &threatLevel,
			&effectID, &effectStatus, &executedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit entry: %w", err)
		}

		// Determine status based on decision and effect
		status := "proposed"
		if approved {
			status = "approved"
			if effectID != nil && effectStatus != nil {
				switch *effectStatus {
				case "executed":
					status = "executed"
				case "failed":
					status = "failed"
				case "pending":
					status = "approved"
				}
			}
		} else {
			status = "denied"
		}

		// Build details string
		details := ""
		if rationale != nil {
			details = *rationale
		}
		if reason != nil && *reason != "" {
			details = *reason
		}

		entry := AuditEntry{
			ID:         decisionID,
			Timestamp:  approvedAt.Format("2006-01-02T15:04:05Z07:00"),
			ActionType: actionType,
			UserID:     approvedBy,
			TrackID:    trackID,
			ProposalID: proposalID,
			DecisionID: decisionID,
			Status:     status,
			Details:    details,
		}

		if effectID != nil {
			entry.EffectID = *effectID
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit entries: %w", err)
	}

	return entries, nil
}

// CountActiveTracks returns the count of active tracks
func (p *Pool) CountActiveTracks(ctx context.Context) (int64, error) {
	var count int64
	err := p.QueryRow(ctx, "SELECT COUNT(*) FROM tracks WHERE state = 'active'").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active tracks: %w", err)
	}
	return count, nil
}

// CountPendingProposals returns the count of pending proposals
func (p *Pool) CountPendingProposals(ctx context.Context) (int64, error) {
	var count int64
	err := p.QueryRow(ctx, "SELECT COUNT(*) FROM proposals WHERE status = 'pending' AND expires_at > NOW()").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending proposals: %w", err)
	}
	return count, nil
}

// CountTotalDetections returns the total count of unique detection messages ever processed
func (p *Pool) CountTotalDetections(ctx context.Context) (int64, error) {
	var count int64
	err := p.QueryRow(ctx, `SELECT COUNT(*) FROM detections`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count detections: %w", err)
	}
	return count, nil
}

// IncrementCounter atomically increments a named counter and returns the new value
func (p *Pool) IncrementCounter(ctx context.Context, counterName string, increment int64) (int64, error) {
	var newValue int64
	err := p.QueryRow(ctx, `SELECT increment_counter($1, $2)`, counterName, increment).Scan(&newValue)
	if err != nil {
		return 0, fmt.Errorf("increment counter %s: %w", counterName, err)
	}
	return newValue, nil
}

// GetCounter returns the current value of a named counter
func (p *Pool) GetCounter(ctx context.Context, counterName string) (int64, error) {
	var value int64
	err := p.QueryRow(ctx, `SELECT counter_value FROM system_counters WHERE counter_name = $1`, counterName).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("get counter %s: %w", counterName, err)
	}
	return value, nil
}

// ClearAllResult contains the counts of deleted records per table
type ClearAllResult struct {
	Effects    int64
	Decisions  int64
	Proposals  int64
	Detections int64
	Tracks     int64
}

// ClearAll deletes all data from the database tables in the correct order
// to respect foreign key constraints. Uses a transaction for atomicity.
// Returns the counts of deleted records per table.
func (p *Pool) ClearAll(ctx context.Context) (*ClearAllResult, error) {
	tx, err := p.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result := &ClearAllResult{}

	// Delete in order respecting foreign key constraints:
	// effects -> decisions -> proposals -> detections -> tracks
	var tag pgconn.CommandTag

	tag, err = tx.Exec(ctx, "DELETE FROM effects")
	if err != nil {
		return nil, fmt.Errorf("failed to delete from effects: %w", err)
	}
	result.Effects = tag.RowsAffected()

	tag, err = tx.Exec(ctx, "DELETE FROM decisions")
	if err != nil {
		return nil, fmt.Errorf("failed to delete from decisions: %w", err)
	}
	result.Decisions = tag.RowsAffected()

	tag, err = tx.Exec(ctx, "DELETE FROM proposals")
	if err != nil {
		return nil, fmt.Errorf("failed to delete from proposals: %w", err)
	}
	result.Proposals = tag.RowsAffected()

	tag, err = tx.Exec(ctx, "DELETE FROM detections")
	if err != nil {
		return nil, fmt.Errorf("failed to delete from detections: %w", err)
	}
	result.Detections = tag.RowsAffected()

	tag, err = tx.Exec(ctx, "DELETE FROM tracks")
	if err != nil {
		return nil, fmt.Errorf("failed to delete from tracks: %w", err)
	}
	result.Tracks = tag.RowsAffected()

	// Reset the messages_processed counter to 0
	_, err = tx.Exec(ctx, "UPDATE system_counters SET counter_value = 0, last_updated = NOW() WHERE counter_name = 'messages_processed'")
	if err != nil {
		return nil, fmt.Errorf("failed to reset messages_processed counter: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}

// Health checks if the database connection is healthy
func (p *Pool) Health(ctx context.Context) error {
	return p.Ping(ctx)
}

