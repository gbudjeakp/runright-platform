-- Embeddings migration - pgvector tables skipped (not available on Fly.io unmanaged Postgres)
-- The job_embeddings table with vector type will be created via a separate migration
-- when pgvector is available. The embedding service gracefully degrades without it.

-- Table to track embedding job progress
CREATE TABLE IF NOT EXISTS embedding_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status TEXT NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed
    total_jobs INTEGER NOT NULL DEFAULT 0,
    processed_jobs INTEGER NOT NULL DEFAULT 0,
    failed_jobs INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Configuration for embedding provider
CREATE TABLE IF NOT EXISTS embedding_config (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1), -- singleton
    provider TEXT NOT NULL DEFAULT 'ollama', -- ollama, openai
    model TEXT NOT NULL DEFAULT 'nomic-embed-text',
    base_url TEXT DEFAULT 'http://localhost:11434',
    dimensions INTEGER NOT NULL DEFAULT 768,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Insert default config
INSERT INTO embedding_config (provider, model, dimensions) 
VALUES ('ollama', 'nomic-embed-text', 768)
ON CONFLICT (id) DO NOTHING;
