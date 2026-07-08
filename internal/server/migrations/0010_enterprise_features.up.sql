-- Teams/Organizations
CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    avatar_url TEXT DEFAULT '',
    billing_email TEXT DEFAULT '',
    plan TEXT NOT NULL DEFAULT 'free',
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_teams_slug ON teams(slug);

-- Team Members with expanded RBAC
CREATE TABLE IF NOT EXISTS team_members (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_email TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    permissions JSONB NOT NULL DEFAULT '[]',
    invited_by TEXT,
    invited_at TIMESTAMPTZ,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(team_id, user_email)
);

CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_email ON team_members(user_email);

-- Team invitations
CREATE TABLE IF NOT EXISTS team_invitations (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    token TEXT NOT NULL UNIQUE,
    invited_by TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_team_invitations_token ON team_invitations(token);
CREATE INDEX IF NOT EXISTS idx_team_invitations_email ON team_invitations(email);

-- API Keys Management
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    user_email TEXT NOT NULL,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    scopes JSONB NOT NULL DEFAULT '["read"]',
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_team ON api_keys(team_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_email);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

-- Audit Logging
CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT REFERENCES teams(id) ON DELETE SET NULL,
    actor_email TEXT NOT NULL,
    actor_ip TEXT,
    actor_user_agent TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    resource_name TEXT,
    details JSONB DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'success',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_team ON audit_logs(team_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_email);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);

-- Roles and Permissions
CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    permissions JSONB NOT NULL DEFAULT '[]',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(team_id, name)
);

-- Insert default system roles
INSERT INTO roles (id, team_id, name, description, permissions, is_system) VALUES
    ('role-owner', NULL, 'owner', 'Full access to all resources and settings', '["*"]', true),
    ('role-admin', NULL, 'admin', 'Manage team settings, members, and resources', '["team:manage","members:manage","resources:manage","billing:view"]', true),
    ('role-billing', NULL, 'billing', 'Manage billing and view cost reports', '["billing:manage","reports:view","analytics:view"]', true),
    ('role-developer', NULL, 'developer', 'View and manage jobs, repos, and policies', '["jobs:view","jobs:manage","repos:view","repos:manage","policies:view"]', true),
    ('role-viewer', NULL, 'viewer', 'Read-only access to dashboards and reports', '["jobs:view","repos:view","reports:view","analytics:view"]', true)
ON CONFLICT DO NOTHING;

-- Scheduled Reports
CREATE TABLE IF NOT EXISTS scheduled_reports (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    created_by TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    report_type TEXT NOT NULL,
    schedule TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    config JSONB NOT NULL DEFAULT '{}',
    recipients JSONB NOT NULL DEFAULT '[]',
    format TEXT NOT NULL DEFAULT 'pdf',
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scheduled_reports_team ON scheduled_reports(team_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_reports_next_run ON scheduled_reports(next_run_at) WHERE enabled = true;

-- Report history
CREATE TABLE IF NOT EXISTS report_runs (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    report_id TEXT NOT NULL REFERENCES scheduled_reports(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    file_url TEXT,
    file_size INTEGER,
    recipients_notified INTEGER DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_report_runs_report ON report_runs(report_id);
CREATE INDEX IF NOT EXISTS idx_report_runs_status ON report_runs(status);

-- Usage Analytics aggregations
CREATE TABLE IF NOT EXISTS usage_analytics (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    period_type TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(team_id, period_start, period_type)
);

CREATE INDEX IF NOT EXISTS idx_usage_analytics_team_period ON usage_analytics(team_id, period_start DESC);

-- Link existing tables to teams (optional team_id)
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS team_id TEXT REFERENCES teams(id) ON DELETE SET NULL;
ALTER TABLE policy_rules ADD COLUMN IF NOT EXISTS team_id TEXT REFERENCES teams(id) ON DELETE SET NULL;
ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS team_id TEXT REFERENCES teams(id) ON DELETE SET NULL;
