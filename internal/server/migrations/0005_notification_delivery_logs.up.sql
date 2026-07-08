CREATE TABLE IF NOT EXISTS notification_delivery_logs (
	id             SERIAL PRIMARY KEY,
	rule_id        TEXT NOT NULL,
	destination_id TEXT NOT NULL,
	channel        TEXT NOT NULL,          -- 'slack' | 'teams' | 'webhook'
	job_id         TEXT NOT NULL DEFAULT '',
	repository     TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL,          -- 'delivered' | 'failed'
	error_message  TEXT NOT NULL DEFAULT '',
	sent_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ndl_rule_id  ON notification_delivery_logs(rule_id);
CREATE INDEX IF NOT EXISTS idx_ndl_sent_at  ON notification_delivery_logs(sent_at);
