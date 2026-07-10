-- Label to instance mappings for auto PR recommendations
CREATE TABLE IF NOT EXISTS label_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    repository TEXT NOT NULL DEFAULT '*',
    label TEXT NOT NULL,
    provider TEXT NOT NULL,
    instance_type TEXT NOT NULL,
    vcpus INT NOT NULL,
    memory_gib NUMERIC(10,2) NOT NULL,
    cost_per_hour NUMERIC(10,4) NOT NULL,
    is_gpu BOOLEAN DEFAULT FALSE,
    gpu_type TEXT,
    gpu_count INT DEFAULT 0,
    gpu_memory_gib NUMERIC(10,2) DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, repository, label)
);

CREATE INDEX idx_label_mappings_team ON label_mappings(team_id);
CREATE INDEX idx_label_mappings_gpu ON label_mappings(is_gpu) WHERE is_gpu = TRUE;

-- Auto-PR settings per team
CREATE TABLE IF NOT EXISTS auto_pr_settings (
    team_id TEXT PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT FALSE,
    min_savings_percent NUMERIC(5,2) DEFAULT 20,
    min_monthly_savings NUMERIC(10,2) DEFAULT 10,
    require_consecutive_runs INT DEFAULT 3,
    gpu_prs_enabled BOOLEAN DEFAULT TRUE,
    gpu_min_savings_percent NUMERIC(5,2) DEFAULT 15,
    exclude_repositories JSONB DEFAULT '[]',
    exclude_job_patterns JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- PR recommendations
CREATE TABLE IF NOT EXISTS pr_recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    repository TEXT NOT NULL,
    job_id TEXT NOT NULL,
    workflow_file TEXT,
    current_label TEXT NOT NULL,
    current_vcpus INT NOT NULL,
    current_memory_gib NUMERIC(10,2) NOT NULL,
    current_cost_per_hour NUMERIC(10,4) NOT NULL,
    recommended_label TEXT NOT NULL,
    recommended_vcpus INT NOT NULL,
    recommended_memory_gib NUMERIC(10,2) NOT NULL,
    recommended_cost_per_hour NUMERIC(10,4) NOT NULL,
    p95_cpu_percent NUMERIC(5,2),
    p95_mem_percent NUMERIC(5,2),
    run_count INT DEFAULT 1,
    consecutive_underutilized INT DEFAULT 1,
    is_gpu_job BOOLEAN DEFAULT FALSE,
    current_gpu_type TEXT,
    recommended_gpu_type TEXT,
    p95_gpu_util_percent NUMERIC(5,2),
    p95_gpu_mem_percent NUMERIC(5,2),
    savings_percent NUMERIC(5,2),
    monthly_savings_usd NUMERIC(10,2),
    status TEXT DEFAULT 'pending',
    pr_url TEXT,
    pr_number INT,
    dismissed_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, repository, job_id)
);

CREATE INDEX idx_pr_recommendations_status ON pr_recommendations(status);
CREATE INDEX idx_pr_recommendations_gpu ON pr_recommendations(is_gpu_job) WHERE is_gpu_job = TRUE;

-- PR history
CREATE TABLE IF NOT EXISTS pr_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recommendation_id UUID REFERENCES pr_recommendations(id) ON DELETE SET NULL,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    repository TEXT NOT NULL,
    job_id TEXT NOT NULL,
    pr_number INT NOT NULL,
    pr_url TEXT NOT NULL,
    old_label TEXT NOT NULL,
    new_label TEXT NOT NULL,
    savings_percent NUMERIC(5,2),
    monthly_savings_usd NUMERIC(10,2),
    is_gpu_job BOOLEAN DEFAULT FALSE,
    status TEXT DEFAULT 'open',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    merged_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ
);

CREATE INDEX idx_pr_history_status ON pr_history(status);
