-- Enable UUID extension for unique decentralized identifiers
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ==========================================
-- TABLE: wallets
-- ==========================================
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    address VARCHAR(255) UNIQUE NOT NULL,
    archetype_score INTEGER DEFAULT 0,
    tags TEXT[],
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- TABLE: token_analysis
-- ==========================================
CREATE TABLE token_analysis (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    mint_address VARCHAR(255) NOT NULL,
    creator_id UUID REFERENCES wallets(id) ON DELETE SET NULL,
    rug_score INTEGER DEFAULT 0,
    verdict VARCHAR(50) NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create Indexes for low-latency queries
CREATE INDEX idx_wallets_address ON wallets(address);
CREATE INDEX idx_token_analysis_mint ON token_analysis(mint_address);
CREATE INDEX idx_token_analysis_creator ON token_analysis(creator_id);

-- ==========================================
-- ROW-LEVEL SECURITY (RLS) IMPLEMENTATION
-- In strict compliance with SECURITY_POLICY.md
-- ==========================================

-- 1. Enable RLS on all sensitive tables
ALTER TABLE wallets ENABLE ROW LEVEL SECURITY;
ALTER TABLE token_analysis ENABLE ROW LEVEL SECURITY;

-- 2. Define the service role policies (Assume external 'service_role' usage)
-- Only the backend service role is permitted to bypass RLS natively
CREATE POLICY wallets_service_policy ON wallets
    FOR ALL
    TO service_role
    USING (true)
    WITH CHECK (true);

CREATE POLICY token_analysis_service_policy ON token_analysis
    FOR ALL
    TO service_role
    USING (true)
    WITH CHECK (true);
