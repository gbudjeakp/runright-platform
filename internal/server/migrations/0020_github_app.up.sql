-- GitHub App installations
CREATE TABLE IF NOT EXISTS github_app_installations (
    id SERIAL PRIMARY KEY,
    installation_id BIGINT UNIQUE NOT NULL,
    account_type VARCHAR(20) NOT NULL, -- 'User' or 'Organization'
    account_login VARCHAR(255) NOT NULL,
    account_id BIGINT NOT NULL,
    target_type VARCHAR(20) NOT NULL, -- 'all' or 'selected'
    suspended_at TIMESTAMPTZ,
    suspended_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Repositories accessible via each installation
CREATE TABLE IF NOT EXISTS github_app_repos (
    id SERIAL PRIMARY KEY,
    installation_id BIGINT NOT NULL REFERENCES github_app_installations(installation_id) ON DELETE CASCADE,
    repo_id BIGINT NOT NULL,
    repo_name VARCHAR(255) NOT NULL, -- full name: owner/repo
    private BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(installation_id, repo_id)
);

-- Workflow runs processed by the app
CREATE TABLE IF NOT EXISTS github_workflow_runs (
    id SERIAL PRIMARY KEY,
    installation_id BIGINT NOT NULL,
    run_id BIGINT UNIQUE NOT NULL,
    repo_name VARCHAR(255) NOT NULL,
    workflow_name VARCHAR(255),
    workflow_path VARCHAR(255),
    head_branch VARCHAR(255),
    head_sha VARCHAR(64),
    event VARCHAR(50),
    status VARCHAR(50),
    conclusion VARCHAR(50),
    pr_number INT,
    run_attempt INT DEFAULT 1,
    run_started_at TIMESTAMPTZ,
    run_completed_at TIMESTAMPTZ,
    artifact_downloaded BOOLEAN DEFAULT FALSE,
    metrics_processed BOOLEAN DEFAULT FALSE,
    comment_posted BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Jobs within workflow runs
CREATE TABLE IF NOT EXISTS github_workflow_jobs (
    id SERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES github_workflow_runs(run_id) ON DELETE CASCADE,
    job_id BIGINT UNIQUE NOT NULL,
    name VARCHAR(255),
    status VARCHAR(50),
    conclusion VARCHAR(50),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    runner_name VARCHAR(255),
    runner_label VARCHAR(255), -- e.g., ubuntu-latest, self-hosted
    metrics_json JSONB, -- parsed RunRight metrics if available
    recommendation_json JSONB, -- machine recommendation
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- GitHub App configuration (stored securely)
CREATE TABLE IF NOT EXISTS github_app_config (
    id SERIAL PRIMARY KEY,
    app_id BIGINT UNIQUE NOT NULL,
    app_slug VARCHAR(255),
    app_name VARCHAR(255),
    client_id VARCHAR(255),
    client_secret_encrypted BYTEA,
    pem_encrypted BYTEA, -- Private key for JWT signing
    webhook_secret_encrypted BYTEA,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_gh_installations_account ON github_app_installations(account_login);
CREATE INDEX idx_gh_repos_name ON github_app_repos(repo_name);
CREATE INDEX idx_gh_runs_repo ON github_workflow_runs(repo_name);
CREATE INDEX idx_gh_runs_status ON github_workflow_runs(status, conclusion);
CREATE INDEX idx_gh_runs_created ON github_workflow_runs(created_at DESC);
CREATE INDEX idx_gh_jobs_run ON github_workflow_jobs(run_id);
