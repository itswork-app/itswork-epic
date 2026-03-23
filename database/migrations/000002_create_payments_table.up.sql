-- PR-13: Solana Pay Integration & Data Persistence

-- Add reason column to token_analysis for detailed AI reporting
ALTER TABLE token_analysis ADD COLUMN IF NOT EXISTS reason TEXT;

-- TABLE: payments
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(255) NOT NULL, -- Clerk User ID
    mint_address VARCHAR(255) NOT NULL,
    reference_key VARCHAR(255) UNIQUE NOT NULL, -- Solana Pay reference (Finalized on-chain)
    status VARCHAR(20) DEFAULT 'pending', -- pending, success, failed
    amount_sol NUMERIC(18, 9) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Optimize for access control checks (IsPaid)
CREATE INDEX IF NOT EXISTS idx_payments_user_mint ON payments(user_id, mint_address);
CREATE INDEX IF NOT EXISTS idx_payments_reference ON payments(reference_key);

-- Row-Level Security Enforcements
ALTER TABLE payments ENABLE ROW LEVEL SECURITY;

CREATE POLICY payments_service_policy ON payments
    FOR ALL
    TO service_role
    USING (true)
    WITH CHECK (true);
