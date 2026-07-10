-- Alert rules for monitoring CI/CD metrics
CREATE TABLE IF NOT EXISTS alert_rules (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    repository TEXT,
    job_id TEXT,
    condition_type TEXT NOT NULL CHECK (condition_type IN ('cost_threshold', 'waste_threshold', 'cpu_threshold', 'memory_threshold', 'gpu_threshold', 'duration_threshold')),
    threshold_value NUMERIC NOT NULL,
    channel TEXT NOT NULL DEFAULT 'slack' CHECK (channel IN ('slack', 'email', 'webhook')),
    destination TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_triggered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_alert_rules_team ON alert_rules(team_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_repository ON alert_rules(repository);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_alert_rules_condition ON alert_rules(condition_type);
