CREATE TABLE IF NOT EXISTS notification_settings (
	id         INT PRIMARY KEY CHECK (id = 1),
	settings   JSONB NOT NULL DEFAULT '{}'::jsonb,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
