CREATE TABLE IF NOT EXISTS jobs (
	id               SERIAL PRIMARY KEY,
	job_id           TEXT NOT NULL,
	run_id           TEXT,
	start_time       TIMESTAMPTZ NOT NULL,
	end_time         TIMESTAMPTZ NOT NULL,
	duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
	summary          JSONB NOT NULL DEFAULT '{}',
	recommendations  JSONB NOT NULL DEFAULT '[]',
	status           TEXT NOT NULL DEFAULT 'completed',
	repository       TEXT,
	created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS run_id TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'completed';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS repository TEXT;

CREATE TABLE IF NOT EXISTS job_metadata (
	job_id        TEXT NOT NULL,
	repository    TEXT NOT NULL DEFAULT '',
	snoozed_until TIMESTAMPTZ,
	snooze_reason TEXT NOT NULL DEFAULT '',
	archived      BOOLEAN NOT NULL DEFAULT false,
	archived_at   TIMESTAMPTZ,
	stale_days    INT NOT NULL DEFAULT 30,
	PRIMARY KEY (job_id, repository)
);

CREATE TABLE IF NOT EXISTS policy_rules (
	repository         TEXT NOT NULL DEFAULT '',
	job_id             TEXT NOT NULL DEFAULT '',
	max_cost_per_hour  DOUBLE PRECISION NOT NULL,
	enabled            BOOLEAN NOT NULL DEFAULT true,
	updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (repository, job_id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_job_id     ON jobs(job_id);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_repository ON jobs(repository) WHERE repository IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_run_id ON jobs(run_id) WHERE run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_policy_rules_repository ON policy_rules(repository);