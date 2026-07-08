ALTER TABLE notification_delivery_logs ADD COLUMN IF NOT EXISTS attempts      INT  NOT NULL DEFAULT 1;
ALTER TABLE notification_delivery_logs ADD COLUMN IF NOT EXISTS max_attempts  INT  NOT NULL DEFAULT 4;
ALTER TABLE notification_delivery_logs ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
ALTER TABLE notification_delivery_logs ADD COLUMN IF NOT EXISTS payload       TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ndl_retry ON notification_delivery_logs (next_retry_at) WHERE status = 'failed' AND next_retry_at IS NOT NULL;
