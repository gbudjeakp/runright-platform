-- Conversation memory compaction: stores rolling summaries so long conversations
-- don't lose context and don't waste tokens re-sending old messages verbatim.
ALTER TABLE assistant_conversations ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
ALTER TABLE assistant_conversations ADD COLUMN IF NOT EXISTS last_compacted_at TIMESTAMPTZ;
