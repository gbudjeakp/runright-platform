CREATE TABLE IF NOT EXISTS user_settings (
  id SERIAL PRIMARY KEY,
  otel_endpoint TEXT NOT NULL DEFAULT '',
  allowed_machine_ids TEXT[] NOT NULL DEFAULT '{}',
  allowed_series TEXT[] NOT NULL DEFAULT '{}',
  allowed_families TEXT[] NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ensure only one row exists
INSERT INTO user_settings (id) VALUES (1) ON CONFLICT DO NOTHING;
