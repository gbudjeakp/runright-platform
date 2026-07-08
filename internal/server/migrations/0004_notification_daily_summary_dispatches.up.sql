CREATE TABLE IF NOT EXISTS notification_daily_summary_dispatches (
	rule_id      TEXT NOT NULL,
	scope_key    TEXT NOT NULL,
	summary_date DATE NOT NULL,
	sent_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (rule_id, scope_key, summary_date)
);
