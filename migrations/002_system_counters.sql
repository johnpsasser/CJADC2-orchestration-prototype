-- Migration 002: Add system_counters table for persistent metrics
-- This table tracks cumulative message counts that persist across restarts

CREATE TABLE IF NOT EXISTS system_counters (
    counter_name VARCHAR(64) PRIMARY KEY,
    counter_value BIGINT NOT NULL DEFAULT 0,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Initialize the messages_processed counter
INSERT INTO system_counters (counter_name, counter_value, last_updated)
VALUES ('messages_processed', 0, NOW())
ON CONFLICT (counter_name) DO NOTHING;

-- Function to atomically increment a counter
CREATE OR REPLACE FUNCTION increment_counter(p_counter_name VARCHAR(64), p_increment BIGINT DEFAULT 1)
RETURNS BIGINT AS $$
DECLARE
    new_value BIGINT;
BEGIN
    UPDATE system_counters
    SET counter_value = counter_value + p_increment,
        last_updated = NOW()
    WHERE counter_name = p_counter_name
    RETURNING counter_value INTO new_value;

    IF new_value IS NULL THEN
        INSERT INTO system_counters (counter_name, counter_value, last_updated)
        VALUES (p_counter_name, p_increment, NOW())
        RETURNING counter_value INTO new_value;
    END IF;

    RETURN new_value;
END;
$$ LANGUAGE plpgsql;

-- Index for fast counter lookups
CREATE INDEX IF NOT EXISTS idx_system_counters_name ON system_counters(counter_name);
