-- PR-NEXUS-ELITE: Fair Transition & Golden Alpha Infrastructure
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS pending_plan VARCHAR(50);
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS plan_tier INT DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS carry_over_quota INT DEFAULT 0;

-- Backfill existing tiers
UPDATE user_subscriptions SET plan_tier = 2 WHERE plan_type = 'SUB_MONTHLY_PRO';
UPDATE user_subscriptions SET plan_tier = 1 WHERE plan_type = 'SUB_WEEKLY_PRO';
UPDATE user_subscriptions SET plan_tier = 3 WHERE plan_type = 'SUB_ULTRA_PRO';
