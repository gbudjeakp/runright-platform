CREATE TABLE IF NOT EXISTS notification_destination_secrets (
	destination_id TEXT PRIMARY KEY,
	webhook_url    TEXT NOT NULL,
	updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
