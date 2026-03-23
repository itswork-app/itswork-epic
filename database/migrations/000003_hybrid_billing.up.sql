-- PR-13.6: Hybrid Treasury System (Credits & Subscriptions)

-- TABLE: user_credits
CREATE TABLE IF NOT EXISTS user_credits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(255) UNIQUE NOT NULL, -- Clerk User ID
    balance INTEGER DEFAULT 0 CHECK (balance >= 0),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- TABLE: user_subscriptions
CREATE TABLE IF NOT EXISTS user_subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(255) UNIQUE NOT NULL, -- Clerk User ID
    plan_type VARCHAR(50) NOT NULL, -- monthly_pro, yearly_pro, etc.
    status VARCHAR(20) DEFAULT 'active', -- active, expired, cancelled
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for fast lookup by user_id
CREATE INDEX IF NOT EXISTS idx_user_credits_user_id ON user_credits(user_id);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_id ON user_subscriptions(user_id);

-- Enable Row-Level Security
ALTER TABLE user_credits ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_subscriptions ENABLE ROW LEVEL SECURITY;

-- Define Service Role Policies
CREATE POLICY user_credits_service_policy ON user_credits
    FOR ALL TO service_role USING (true) WITH CHECK (true);

CREATE POLICY user_subscriptions_service_policy ON user_subscriptions
    FOR ALL TO service_role USING (true) WITH CHECK (true);
