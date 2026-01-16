-- CJADC2 Initial Database Schema
-- PostgreSQL 16

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Enum types
CREATE TYPE track_classification AS ENUM ('friendly', 'hostile', 'unknown', 'neutral');
CREATE TYPE track_type AS ENUM ('aircraft', 'vessel', 'ground', 'missile', 'unknown');
CREATE TYPE threat_level AS ENUM ('low', 'medium', 'high', 'critical');
CREATE TYPE track_state AS ENUM ('active', 'stale', 'lost', 'merged');
CREATE TYPE proposal_status AS ENUM ('pending', 'queued', 'approved', 'denied', 'expired');
CREATE TYPE action_type AS ENUM ('engage', 'track', 'identify', 'ignore', 'intercept', 'monitor');
CREATE TYPE effect_status AS ENUM ('pending', 'executed', 'failed', 'simulated');

-- Tracks table - materialized view of track state
CREATE TABLE tracks (
    track_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    external_track_id VARCHAR(64) UNIQUE NOT NULL,
    classification track_classification NOT NULL DEFAULT 'unknown',
    type track_type NOT NULL DEFAULT 'unknown',
    confidence DECIMAL(4,3) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    position_lat DECIMAL(10,7) NOT NULL,
    position_lon DECIMAL(10,7) NOT NULL,
    position_alt DECIMAL(10,2),
    velocity_speed DECIMAL(10,2),
    velocity_heading DECIMAL(5,2),
    threat_level threat_level NOT NULL DEFAULT 'low',
    state track_state NOT NULL DEFAULT 'active',
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    detection_count INTEGER NOT NULL DEFAULT 1,
    sources JSONB NOT NULL DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tracks_state ON tracks(state);
CREATE INDEX idx_tracks_threat_level ON tracks(threat_level);
CREATE INDEX idx_tracks_last_updated ON tracks(last_updated);
CREATE INDEX idx_tracks_classification ON tracks(classification);
CREATE INDEX idx_tracks_external_id ON tracks(external_track_id);

-- Detections table - raw sensor data for audit/replay
CREATE TABLE detections (
    detection_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID UNIQUE NOT NULL,
    correlation_id UUID NOT NULL,
    track_id UUID REFERENCES tracks(track_id) ON DELETE SET NULL,
    sensor_id VARCHAR(64) NOT NULL,
    sensor_type VARCHAR(32) NOT NULL,
    position_lat DECIMAL(10,7) NOT NULL,
    position_lon DECIMAL(10,7) NOT NULL,
    position_alt DECIMAL(10,2),
    velocity_speed DECIMAL(10,2),
    velocity_heading DECIMAL(5,2),
    confidence DECIMAL(4,3) NOT NULL,
    raw_data BYTEA,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_detections_correlation_id ON detections(correlation_id);
CREATE INDEX idx_detections_track_id ON detections(track_id);
CREATE INDEX idx_detections_sensor_id ON detections(sensor_id);
CREATE INDEX idx_detections_created_at ON detections(created_at);

-- Proposals table - action proposals awaiting decision
CREATE TABLE proposals (
    proposal_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID UNIQUE,
    correlation_id TEXT,
    track_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    priority INTEGER NOT NULL CHECK (priority >= 1 AND priority <= 10),
    threat_level TEXT,
    rationale TEXT,
    constraints JSONB DEFAULT '[]',
    track_data JSONB,
    policy_decision JSONB,
    status TEXT NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_proposals_status ON proposals(status);
CREATE INDEX idx_proposals_track_id ON proposals(track_id);
CREATE INDEX idx_proposals_expires_at ON proposals(expires_at);
CREATE INDEX idx_proposals_priority ON proposals(priority DESC);
CREATE INDEX idx_proposals_correlation_id ON proposals(correlation_id);

-- Decisions table - human approval/denial of proposals
CREATE TABLE decisions (
    decision_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID UNIQUE,
    correlation_id TEXT,
    proposal_id UUID REFERENCES proposals(proposal_id),
    approved BOOLEAN NOT NULL,
    approved_by TEXT NOT NULL,
    approved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason TEXT,
    conditions JSONB DEFAULT '[]',
    action_type TEXT NOT NULL,
    track_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_decisions_proposal_id ON decisions(proposal_id);
CREATE INDEX idx_decisions_approved_by ON decisions(approved_by);
CREATE INDEX idx_decisions_approved ON decisions(approved);
CREATE INDEX idx_decisions_correlation_id ON decisions(correlation_id);

-- Effects table - executed actions with idempotency
CREATE TABLE effects (
    effect_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID UNIQUE,
    correlation_id TEXT,
    decision_id UUID REFERENCES decisions(decision_id),
    proposal_id UUID REFERENCES proposals(proposal_id),
    track_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    executed_at TIMESTAMPTZ,
    result TEXT,
    idempotent_key TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_effects_decision_id ON effects(decision_id);
CREATE INDEX idx_effects_status ON effects(status);
CREATE INDEX idx_effects_idempotent_key ON effects(idempotent_key);
CREATE INDEX idx_effects_correlation_id ON effects(correlation_id);

-- Audit log table - comprehensive audit trail
CREATE TABLE audit_log (
    log_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type VARCHAR(32) NOT NULL,
    entity_id UUID NOT NULL,
    action VARCHAR(32) NOT NULL,
    actor_id VARCHAR(128) NOT NULL,
    actor_type VARCHAR(32) NOT NULL,
    old_value JSONB,
    new_value JSONB,
    correlation_id UUID NOT NULL,
    policy_decision JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_actor ON audit_log(actor_id);
CREATE INDEX idx_audit_log_correlation_id ON audit_log(correlation_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

-- Stage metrics table - for observability dashboard
CREATE TABLE stage_metrics (
    metric_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    stage VARCHAR(32) NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    processed_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    p50_latency_ms DECIMAL(10,2),
    p95_latency_ms DECIMAL(10,2),
    p99_latency_ms DECIMAL(10,2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(stage, window_start)
);

CREATE INDEX idx_stage_metrics_stage ON stage_metrics(stage);
CREATE INDEX idx_stage_metrics_window ON stage_metrics(window_start, window_end);

-- Idempotency keys table - prevent duplicate processing
CREATE TABLE idempotency_keys (
    key_hash VARCHAR(64) PRIMARY KEY,
    message_id UUID NOT NULL,
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);

CREATE INDEX idx_idempotency_expires ON idempotency_keys(expires_at);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_tracks_updated_at
    BEFORE UPDATE ON tracks
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_proposals_updated_at
    BEFORE UPDATE ON proposals
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Function to clean up expired idempotency keys
CREATE OR REPLACE FUNCTION cleanup_expired_idempotency_keys()
RETURNS void AS $$
BEGIN
    DELETE FROM idempotency_keys WHERE expires_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- View for pending proposals queue (for UI)
CREATE VIEW pending_proposals_queue AS
SELECT
    p.proposal_id,
    p.track_id,
    p.track_id as external_track_id,
    p.threat_level,
    p.action_type,
    p.priority,
    p.rationale,
    p.policy_decision as policy_result,
    p.expires_at,
    p.created_at,
    p.track_data
FROM proposals p
WHERE p.status = 'pending'
  AND p.expires_at > NOW()
ORDER BY p.priority DESC, p.created_at ASC;

-- View for active tracks (for UI)
CREATE VIEW active_tracks_view AS
SELECT
    t.track_id,
    t.external_track_id,
    t.classification,
    t.type,
    t.confidence,
    t.position_lat,
    t.position_lon,
    t.position_alt,
    t.velocity_speed,
    t.velocity_heading,
    t.threat_level,
    t.state,
    t.first_seen,
    t.last_updated,
    t.detection_count,
    t.sources,
    (SELECT COUNT(*) FROM proposals p WHERE p.track_id = t.external_track_id AND p.status = 'pending') as pending_proposals
FROM tracks t
WHERE t.state = 'active'
ORDER BY t.threat_level DESC, t.last_updated DESC;

-- View for decision audit trail
CREATE VIEW decision_audit_trail AS
SELECT
    d.decision_id,
    d.approved,
    d.approved_by,
    d.approved_at,
    d.reason,
    p.proposal_id,
    d.action_type,
    p.priority,
    p.rationale,
    d.track_id as external_track_id,
    p.threat_level,
    e.effect_id,
    e.status as effect_status,
    e.executed_at,
    e.result as effect_result
FROM decisions d
JOIN proposals p ON d.proposal_id = p.proposal_id
LEFT JOIN effects e ON d.decision_id = e.decision_id
ORDER BY d.approved_at DESC;
