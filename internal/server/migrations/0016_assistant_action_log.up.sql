-- Assistant action log for audit trail of AI-initiated actions
CREATE TABLE IF NOT EXISTS assistant_action_log (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    arguments TEXT NOT NULL,
    result TEXT NOT NULL,
    success BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Foreign key to conversations (optional - action might outlive conversation)
    CONSTRAINT fk_conversation FOREIGN KEY (conversation_id) 
        REFERENCES assistant_conversations(id) ON DELETE SET NULL
);

-- Index for querying actions by user
CREATE INDEX IF NOT EXISTS idx_action_log_user ON assistant_action_log(user_id);

-- Index for querying actions by conversation
CREATE INDEX IF NOT EXISTS idx_action_log_conversation ON assistant_action_log(conversation_id);

-- Index for querying actions by tool
CREATE INDEX IF NOT EXISTS idx_action_log_tool ON assistant_action_log(tool_name);

-- Index for time-based queries (audit)
CREATE INDEX IF NOT EXISTS idx_action_log_created ON assistant_action_log(created_at DESC);
