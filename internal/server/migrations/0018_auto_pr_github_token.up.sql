-- Store a per-team GitHub PAT so the platform can open PRs without
-- requiring a server-level GITHUB_TOKEN environment variable.
ALTER TABLE auto_pr_settings
    ADD COLUMN IF NOT EXISTS github_token TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS github_token_updated_at TIMESTAMPTZ;
