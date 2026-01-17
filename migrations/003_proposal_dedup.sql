-- Migration: Proposal De-duplication
-- Adds hit_count column and unique constraint to consolidate proposals per track

-- Add hit_count column to track how many sensor hits contributed to this proposal
ALTER TABLE proposals ADD COLUMN IF NOT EXISTS hit_count INTEGER NOT NULL DEFAULT 1;

-- Add last_hit_at to track when the most recent sensor hit occurred
ALTER TABLE proposals ADD COLUMN IF NOT EXISTS last_hit_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Create partial unique index: only one pending proposal per track_id
-- This enforces de-duplication at the database level
CREATE UNIQUE INDEX IF NOT EXISTS idx_proposals_track_pending_unique
  ON proposals(track_id)
  WHERE status = 'pending';

-- Update the pending_proposals_queue view to include hit_count
DROP VIEW IF EXISTS pending_proposals_queue;
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
    p.updated_at,
    p.track_data,
    p.hit_count,
    p.last_hit_at
FROM proposals p
WHERE p.status = 'pending'
  AND p.expires_at > NOW()
ORDER BY p.priority DESC, p.created_at ASC;
