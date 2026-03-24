-- Add quota columns to user_subscriptions
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS quota_limit INT DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS current_usage INT DEFAULT 0;

-- Optional: Initialize defaults for existing Pro subscriptions if needed
-- UPDATE user_subscriptions SET quota_limit = 5000 WHERE plan_id = 'SUB_MONTHLY_PRO';
-- UPDATE user_subscriptions SET quota_limit = 1000 WHERE plan_id = 'SUB_WEEKLY_PRO';
