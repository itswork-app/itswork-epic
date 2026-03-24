-- Migration: 000004_create_api_keys_table
-- Description: Table to store Bot API Keys for programmatic access.
-- Master Blueprint Read & Verified.

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    api_key_hash TEXT NOT NULL UNIQUE,
    label TEXT,
    status TEXT NOT NULL DEFAULT 'active', -- 'active', 'revoked'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup by hash (used in middleware/caching)
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(api_key_hash);
-- Index for user-based management
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);

-- Master Blueprint Read & Verified.
