-- Migration 004: Add intervention_rules table for configurable human-in-the-loop requirements
-- This table allows operators to configure which parameter combinations require human approval

CREATE TABLE IF NOT EXISTS intervention_rules (
    rule_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Rule identification
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Matching criteria (conditions that trigger this rule)
    action_types TEXT[] NOT NULL DEFAULT '{}',           -- e.g., ARRAY['engage','intercept']
    threat_levels TEXT[] NOT NULL DEFAULT '{}',          -- e.g., ARRAY['high','critical']
    classifications TEXT[] NOT NULL DEFAULT '{}',        -- e.g., ARRAY['hostile','unknown']
    track_types TEXT[] NOT NULL DEFAULT '{}',            -- e.g., ARRAY['missile','aircraft']

    -- Priority-based matching
    min_priority INTEGER,                                -- Match if priority >= this value
    max_priority INTEGER,                                -- Match if priority <= this value

    -- Rule behavior
    requires_approval BOOLEAN NOT NULL DEFAULT true,     -- Whether human approval is required when matched
    auto_approve BOOLEAN NOT NULL DEFAULT false,         -- If true, auto-approve when matched (skip queue)

    -- Rule metadata
    enabled BOOLEAN NOT NULL DEFAULT true,
    evaluation_order INTEGER NOT NULL DEFAULT 100,       -- Lower = evaluated first

    -- Audit
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by VARCHAR(255),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_priority_range CHECK (min_priority IS NULL OR max_priority IS NULL OR min_priority <= max_priority),
    CONSTRAINT valid_evaluation_order CHECK (evaluation_order >= 0),
    CONSTRAINT unique_rule_name UNIQUE (name)
);

-- Indexes for efficient rule matching
CREATE INDEX IF NOT EXISTS idx_intervention_rules_enabled ON intervention_rules(enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_intervention_rules_evaluation_order ON intervention_rules(evaluation_order);
CREATE INDEX IF NOT EXISTS idx_intervention_rules_action_types ON intervention_rules USING GIN(action_types);
CREATE INDEX IF NOT EXISTS idx_intervention_rules_threat_levels ON intervention_rules USING GIN(threat_levels);
CREATE INDEX IF NOT EXISTS idx_intervention_rules_classifications ON intervention_rules USING GIN(classifications);

-- Trigger to auto-update updated_at timestamp
CREATE TRIGGER update_intervention_rules_updated_at
    BEFORE UPDATE ON intervention_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert default rules based on current hardcoded behavior
INSERT INTO intervention_rules (name, description, action_types, requires_approval, auto_approve, evaluation_order, created_by)
VALUES
    ('Kinetic Actions Always Require Approval',
     'Engage and intercept actions always require human approval regardless of other factors',
     ARRAY['engage', 'intercept'],
     true, false, 10, 'system'),

    ('High Priority Identification Requires Approval',
     'Identification actions with priority >= 6 require human approval',
     ARRAY['identify'],
     true, false, 20, 'system'),

    ('Passive Observation Auto-Approved',
     'Track, monitor, and ignore actions are auto-approved',
     ARRAY['track', 'monitor', 'ignore'],
     false, true, 30, 'system')
ON CONFLICT (name) DO NOTHING;

-- Comment on table
COMMENT ON TABLE intervention_rules IS 'Configurable rules for determining when human-in-the-loop intervention is required for action proposals';
