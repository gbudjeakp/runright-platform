-- Safeguards against recommendation flood and inaccurate signals.
--
-- min_data_days: the N consecutive runs must span at least this many calendar
--   days before a recommendation is created.  Prevents an incident-day spike
--   (GitHub outage, cloud provider issue) from filling the queue with bad recs
--   when many jobs complete in a short burst.  0 = disabled (useful in dev/test).
--
-- max_recs_per_scan: hard cap on new recommendations created per background
--   scan cycle.  Even if hundreds of jobs qualify, at most this many new rows
--   are inserted per pass so the queue never explodes in one run.
ALTER TABLE auto_pr_settings
    ADD COLUMN IF NOT EXISTS min_data_days     INT NOT NULL DEFAULT 7,
    ADD COLUMN IF NOT EXISTS max_recs_per_scan  INT NOT NULL DEFAULT 20;
