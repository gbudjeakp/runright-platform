-- Repository ownership: maps a repository to a named team and the
-- destination IDs that team should receive alerts on.
-- destination_ids is a JSONB array of notification destination IDs
-- (matching IDs in notification_settings.slack/teams/webhooks.destinations).

CREATE TABLE IF NOT EXISTS repository_ownership (
	repository      TEXT NOT NULL,
	team_name       TEXT NOT NULL,
	destination_ids JSONB NOT NULL DEFAULT '[]',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (repository, team_name)
);

CREATE INDEX IF NOT EXISTS idx_repo_ownership_repo ON repository_ownership(repository);
