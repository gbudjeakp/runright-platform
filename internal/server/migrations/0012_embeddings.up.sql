-- Enable pgvector extension (requires PostgreSQL 11+ with vector extension installed)
CREATE EXTENSION IF NOT EXISTS vector;

-- Job embeddings table for semantic search
CREATE TABLE IF NOT EXISTS job_embeddings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id TEXT NOT NULL,
    repository TEXT NOT NULL,
    
    -- The text that was embedded (for debugging/reindexing)
    embedded_text TEXT NOT NULL,
    
    -- Vector embedding (1536 dimensions for OpenAI, 768 for nomic-embed-text)
    -- Using 768 as default for local Ollama models
    embedding vector(768),
    
    -- Metadata for filtering
    start_time TIMESTAMP WITH TIME ZONE,
    duration_seconds INTEGER,
    cost_usd DECIMAL(10, 4),
    machine_type TEXT,
    
    -- Tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Prevent duplicate embeddings for the same job run
    UNIQUE(job_id, start_time)
);

-- Index for vector similarity search (using IVFFlat for speed)
-- Adjust lists parameter based on data size: sqrt(num_rows) is a good starting point
CREATE INDEX IF NOT EXISTS job_embeddings_vector_idx 
    ON job_embeddings 
    USING ivfflat (embedding vector_cosine_ops) 
    WITH (lists = 100);

-- Index for filtering by repository
CREATE INDEX IF NOT EXISTS job_embeddings_repository_idx ON job_embeddings(repository);

-- Index for time-based queries
CREATE INDEX IF NOT EXISTS job_embeddings_start_time_idx ON job_embeddings(start_time DESC);

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
