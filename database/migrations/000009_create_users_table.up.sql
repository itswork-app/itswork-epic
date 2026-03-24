-- PR-NEXUS-AUTH-JOURNEY: User Role & Onboarding Infrastructure

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY, -- Clerk User ID
    role VARCHAR(50) DEFAULT 'unassigned', -- trader, developer
    free_scans_used INT DEFAULT 0,
    free_api_used INT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Ensure RLS is enabled
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Service role policy (Fall back to current_user for local dev)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
        CREATE POLICY users_service_policy ON users FOR ALL TO service_role USING (true) WITH CHECK (true);
    ELSE
        CREATE POLICY users_local_policy ON users FOR ALL TO CURRENT_USER USING (true) WITH CHECK (true);
    END IF;
END $$;
