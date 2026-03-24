package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

const (
	// UI quota tiers (Marketing Portal)
	QuotaUIBasic = 50
	QuotaUIPro   = 200

	// Audit Optimization: Centralized Multiplier
	DevMultiplier = 3

	// API quota tiers (Developer Portal - 3x Multiplier as per Audit)
	QuotaAPIPro        = QuotaUIPro * DevMultiplier
	QuotaAPIUltra      = 1200 * DevMultiplier // Equivalent to 400 UI scans
	QuotaAPIEnterprise = 7700 * DevMultiplier // Equivalent to 2566 UI scans

	// Legacy compatibility (used in fulfillment logic)
	QuotaProMonthly   = QuotaUIPro
	QuotaProWeekly    = 50 // Basic/weekly
	QuotaUltraMonthly = QuotaAPIUltra
	QuotaEnterprise   = QuotaAPIEnterprise

	// Free tier limits
	FreeUIScans = 3
	FreeAPIUses = 10
)

type Payment struct {
	ID          string
	UserID      string
	MintAddress string
	Reference   string
	Status      string
	AmountSol   float64
	CreatedAt   time.Time
}

type PaymentRepository struct {
	db    *sql.DB
	redis *redis.Client
}

func NewPaymentRepository(db *sql.DB, rdb *redis.Client) *PaymentRepository {
	return &PaymentRepository{db: db, redis: rdb}
}

func (r *PaymentRepository) GetDB() *sql.DB {
	return r.db
}

func (r *PaymentRepository) GetRedis() *redis.Client {
	return r.redis
}

// SavePayment initializes a payment record in Pending state
func (r *PaymentRepository) SavePayment(ctx context.Context, p *Payment) error {
	query := `
		INSERT INTO payments (user_id, mint_address, reference_key, status, amount_sol)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id;
	`
	err := r.db.QueryRowContext(ctx, query, p.UserID, p.MintAddress, p.Reference, "pending", p.AmountSol).Scan(&p.ID)
	if err != nil {
		log.Error().Err(err).Str("user", p.UserID).Str("mint", p.MintAddress).Msg("Failed to save payment record")
		return err
	}
	return nil
}

// UpdatePaymentStatus transitions payment to Success/Failed and updates Redis cache
// It also triggers fulfillment for BUNDLE or SUBSCRIPTION purchases.
func (r *PaymentRepository) UpdatePaymentStatus(ctx context.Context, reference, status string) error {
	var userID, mint string
	var amount float64
	query := `
		UPDATE payments
		SET status = $1, updated_at = now()
		WHERE reference_key = $2
		RETURNING user_id, mint_address, amount_sol;
	`
	err := r.db.QueryRowContext(ctx, query, status, reference).Scan(&userID, &mint, &amount)
	if err != nil {
		return err
	}

	if status == "success" {
		// 1. Fulfillment Logic
		if mint == "BUNDLE_50" {
			if err := r.AddUserCredits(ctx, userID, 50); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: AddUserCredits 50")
				sentry.CaptureException(err)
			}
		} else if mint == "BUNDLE_100" {
			if err := r.AddUserCredits(ctx, userID, 100); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: AddUserCredits 100")
				sentry.CaptureException(err)
			}
		} else if mint == "SUB_MONTHLY_PRO" {
			if err := r.ActivateSubscription(ctx, userID, "SUB_MONTHLY_PRO", 30, QuotaProMonthly); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: ActivateSubscription Monthly")
				sentry.CaptureException(err)
			}
		} else if mint == "SUB_WEEKLY_PRO" {
			if err := r.ActivateSubscription(ctx, userID, "SUB_WEEKLY_PRO", 7, QuotaProWeekly); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: ActivateSubscription Weekly")
				sentry.CaptureException(err)
			}
		} else if mint == "SUB_ULTRA_PRO" {
			if err := r.ActivateSubscription(ctx, userID, "SUB_ULTRA_PRO", 30, QuotaUltraMonthly); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: ActivateSubscription Ultra")
				sentry.CaptureException(err)
			}
		}

		// 2. Cache the successful access in Redis for 1 hour
		if r.redis != nil {
			cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)
			r.redis.Set(ctx, cacheKey, "true", 1*time.Hour)
		}
	}

	return nil
}

// CheckAccess (Audit PR-FIX-V1) differentiates between checking eligibility and committing usage.
// Returns (grant, kind, error). kind is used by CommitUsage to finalize the charge.
func (r *PaymentRepository) CheckAccess(ctx context.Context, userID, mint string, isAPI bool) (bool, string, error) {
	_ = r.InitUserCredits(ctx, userID)

	// 1. Cache Check
	cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil && val == "true" {
			return true, "cache", nil
		}
	}

	// 2. FREE TIER CHECK
	kind := "ui"
	limit := int64(FreeUIScans)
	if isAPI {
		kind = "api"
		limit = int64(FreeAPIUses)
	}
	used := r.GetFreeUsage(ctx, userID, kind)
	if used < limit {
		return true, "free_" + kind, nil
	}

	// 3. SUBSCRIPTION CHECK
	if r.IsProSubscriber(ctx, userID) {
		remaining, _ := r.GetQuotaRemaining(ctx, userID)
		if remaining > 0 {
			return true, "subscription", nil
		}
	}

	if isAPI {
		return false, "", nil
	}

	// 4. CREDIT CHECK
	// Check balance without deducting
	var balance int
	err := r.db.QueryRowContext(ctx, "SELECT balance FROM user_credits WHERE user_id = $1", userID).Scan(&balance)
	if err == nil && balance > 0 {
		return true, "credit", nil
	}

	// 5. SINGLE PAYMENT CHECK
	var count int
	query := `SELECT COUNT(*) FROM payments WHERE user_id = $1 AND mint_address = $2 AND status = 'success'`
	err = r.db.QueryRowContext(ctx, query, userID, mint).Scan(&count)
	if err == nil && count > 0 {
		return true, "single_pay", nil
	}

	return false, "", nil
}

// CommitUsage (Audit PR-FIX-V1) performs the actual decrement/increment of counters.
// Should be called ONLY after successful analysis/work to ensure atomic quota recovery.
func (r *PaymentRepository) CommitUsage(ctx context.Context, userID, kind, mint string) {
	switch kind {
	case "cache":
		// No-op, already cached
	case "free_ui":
		r.IncrementFreeUsage(ctx, userID, "ui")
		r.cacheAccess(ctx, userID, mint)
	case "free_api":
		r.IncrementFreeUsage(ctx, userID, "api")
		r.cacheAccess(ctx, userID, mint)
	case "subscription":
		r.IncrementUsage(ctx, userID)
		r.cacheAccess(ctx, userID, mint)
	case "credit":
		_, _ = r.DeductCredit(ctx, userID)
		r.cacheAccess(ctx, userID, mint)
	case "single_pay":
		r.cacheAccess(ctx, userID, mint)
	}
}

// IsPaid - DEPRECATED in PR-FIX-V1. Use CheckAccess + CommitUsage for atomic recovery.
func (r *PaymentRepository) IsPaid(ctx context.Context, userID, mint string, isAPI bool) bool {
	grant, kind, _ := r.CheckAccess(ctx, userID, mint, isAPI)
	if grant {
		r.CommitUsage(ctx, userID, kind, mint)
	}
	return grant
}

// GetFreeUsage returns the number of free scans used by a user.
// kind: 'ui' or 'api'
func (r *PaymentRepository) GetFreeUsage(ctx context.Context, userID, kind string) int64 {
	// 1. Try Redis first
	redisKey := fmt.Sprintf("free:user:%s:%s", userID, kind)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, redisKey).Int64()
		if err == nil {
			return val
		}
	}

	// 2. DB Fallback
	col := "free_scans_used"
	if kind == "api" {
		col = "free_api_used"
	}
	var used int64
	query := fmt.Sprintf(`SELECT %s FROM users WHERE id = $1`, col)
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&used); err != nil {
		return 0
	}
	// Backfill Redis
	if r.redis != nil {
		r.redis.Set(ctx, redisKey, used, 30*24*time.Hour)
	}
	return used
}

// IncrementFreeUsage synchronously increments the free usage counter in Redis and async in DB.
// Audit PR-FIX-V1: Synchronous Redis increment prevents double-spending.
func (r *PaymentRepository) IncrementFreeUsage(ctx context.Context, userID, kind string) {
	// 1. Increment Redis synchronously (CRITICAL for double-spend prevention)
	redisKey := fmt.Sprintf("free:user:%s:%s", userID, kind)
	if r.redis != nil {
		err := r.redis.Incr(ctx, redisKey).Err()
		if err != nil {
			log.Error().Err(err).Str("user", userID).Msg("Failed to increment free usage in Redis")
		}
	}

	// 2. Async DB sync
	col := "free_scans_used"
	if kind == "api" {
		col = "free_api_used"
	}
	go func() {
		query := fmt.Sprintf(`UPDATE users SET %s = %s + 1 WHERE id = $1`, col, col)
		if _, err := r.db.ExecContext(context.Background(), query, userID); err != nil {
			log.Error().Err(err).Str("user", userID).Str("kind", kind).Msg("Failed to sync free usage to DB")
		}
	}()
}

// IsProSubscriber checks if the user has a 'active' subscription that hasn't expired.
// Uses Redis for quick lookup to reduce DB load.
func (r *PaymentRepository) IsProSubscriber(ctx context.Context, userID string) bool {
	subCacheKey := fmt.Sprintf("sub_active:%s", userID)

	// 1. Redis Check
	if r.redis != nil {
		val, err := r.redis.Get(ctx, subCacheKey).Result()
		if err == nil {
			return val == "true"
		}
	}

	// 2. Postgres Check
	var count int
	query := `
		SELECT COUNT(*) 
		FROM user_subscriptions 
		WHERE user_id = $1 AND status = 'active' AND expires_at > now()
	`
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Failed to check subscription in DB")
		return false
	}

	isActive := count > 0

	// 3. Backfill Cache (5 minutes TTL for subscriptions)
	if r.redis != nil {
		statusStr := "false"
		if isActive {
			statusStr = "true"
		}
		r.redis.Set(ctx, subCacheKey, statusStr, 5*time.Minute)
	}

	return isActive
}

// DeductCredit removes 1 credit from the user's balance atomically
func (r *PaymentRepository) DeductCredit(ctx context.Context, userID string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Atomic update with check
	query := `
		UPDATE user_credits
		SET balance = balance - 1, updated_at = now()
		WHERE user_id = $1 AND balance > 0
		RETURNING balance
	`
	var newBalance int
	err = tx.QueryRowContext(ctx, query, userID).Scan(&newBalance)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // Inefficient balance or user not found
		}
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	log.Info().Str("user", userID).Int("new_balance", newBalance).Msg("Credit deducted")
	return true, nil
}

// cacheAccess is a helper to update Redis cache
func (r *PaymentRepository) cacheAccess(ctx context.Context, userID, mint string) {
	if r.redis != nil {
		cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)
		r.redis.Set(ctx, cacheKey, "true", 1*time.Hour)
	}
}

// InitUserCredits ensures a user has a credit row in the database
func (r *PaymentRepository) InitUserCredits(ctx context.Context, userID string) error {
	query := `INSERT INTO user_credits (user_id, balance) VALUES ($1, 0) ON CONFLICT (user_id) DO NOTHING`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

// AddUserCredits adds credits to a user balance atomically
func (r *PaymentRepository) AddUserCredits(ctx context.Context, userID string, amount int) error {
	query := `
		INSERT INTO user_credits (user_id, balance) 
		VALUES ($1, $2) 
		ON CONFLICT (user_id) DO UPDATE 
		SET balance = user_credits.balance + $2, updated_at = now()
	`
	_, err := r.db.ExecContext(ctx, query, userID, amount)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Int("amount", amount).Msg("Failed to add user credits")
	}
	return err
}

const (
	TierWeeklyPro  = 1
	TierMonthlyPro = 2
	TierUltraPro   = 3
	TierEnterprise = 4
)

func getTier(plan string) int {
	switch plan {
	case "SUB_WEEKLY_PRO":
		return TierWeeklyPro
	case "SUB_MONTHLY_PRO":
		return TierMonthlyPro
	case "SUB_ULTRA_PRO":
		return TierUltraPro
	case "SUB_ENTERPRISE":
		return TierEnterprise
	default:
		return 0
	}
}

// ActivateSubscription activates or extends a user subscription with quota.
// PR-SUBSCRIPTION-PRESTIGE: Implements Smart Upgrade (Carry-over) and Queued Downgrade.
func (r *PaymentRepository) ActivateSubscription(ctx context.Context, userID, planType string, durationDays, quota int) error {
	if durationDays <= 0 {
		log.Error().Str("user", userID).Int("duration", durationDays).Msg("Invalid subscription duration")
		sentry.CaptureMessage(fmt.Sprintf("Invalid subscription duration for user %s: %d", userID, durationDays))
		return fmt.Errorf("invalid subscription duration: %d", durationDays)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Fetch current subscription to detect transition type
	var oldPlan, oldStatus string
	var oldTier, oldLimit, oldUsage int
	var oldExpiry time.Time
	queryFetch := `
		SELECT plan_type, COALESCE(plan_tier, '0')::INT, status, quota_limit, current_usage, expires_at 
		FROM user_subscriptions 
		WHERE user_id = $1
	`
	row := tx.QueryRowContext(ctx, queryFetch, userID)
	err = row.Scan(&oldPlan, &oldTier, &oldStatus, &oldLimit, &oldUsage, &oldExpiry)

	newTier := getTier(planType)
	now := time.Now()

	if err == sql.ErrNoRows {
		// NEW SUBSCRIPTION
		queryInsert := `
			INSERT INTO user_subscriptions (user_id, plan_type, plan_tier, status, expires_at, quota_limit, current_usage)
			VALUES ($1, $2, $3, 'active', now() + interval '1 day' * $4, $5, 0)
		`
		_, err = tx.ExecContext(ctx, queryInsert, userID, planType, newTier, durationDays, quota)
	} else if err == nil {
		// EXISTING SUBSCRIPTION
		if newTier > oldTier && oldExpiry.After(now) {
			// UPGRADE: Carry over unused quota
			leftover := oldLimit - oldUsage
			if leftover < 0 {
				leftover = 0
			}
			queryUpgrade := `
				UPDATE user_subscriptions 
				SET plan_type = $1, 
				    plan_tier = $2, 
				    status = 'active', 
				    expires_at = now() + interval '1 day' * $3, 
				    quota_limit = $4 + $5, 
				    carry_over_quota = $5,
				    current_usage = 0,
				    updated_at = now()
				WHERE user_id = $6
			`
			_, err = tx.ExecContext(ctx, queryUpgrade, planType, newTier, durationDays, quota, leftover, userID)
		} else if newTier < oldTier && oldExpiry.After(now) {
			// QUEUED DOWNGRADE: Store in pending_plan
			queryDowngrade := `
				UPDATE user_subscriptions 
				SET pending_plan = $1, updated_at = now()
				WHERE user_id = $2
			`
			_, err = tx.ExecContext(ctx, queryDowngrade, planType, userID)
		} else {
			// RENEWAL or EXTENSION (Same tier or old plan expired)
			queryRenew := `
				UPDATE user_subscriptions 
				SET plan_type = $1, 
				    plan_tier = $2, 
				    status = 'active', 
				    expires_at = CASE 
						WHEN expires_at > now() THEN expires_at + interval '1 day' * $3
						ELSE now() + interval '1 day' * $3
					END,
				    quota_limit = $4,
				    current_usage = 0, -- Reset usage for new period
				    updated_at = now()
				WHERE user_id = $5
			`
			_, err = tx.ExecContext(ctx, queryRenew, planType, newTier, durationDays, quota, userID)
		}
	}

	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Failed to update subscription in DB")
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Reset Redis usage counter on activation/renewal
	if r.redis != nil {
		usageKey := fmt.Sprintf("usage:user:%s", userID)
		r.redis.Del(ctx, usageKey)
	}

	return nil
}

// IncrementUsage increments the Redis usage counter for a user
func (r *PaymentRepository) IncrementUsage(ctx context.Context, userID string) {
	if r.redis == nil {
		return
	}
	usageKey := fmt.Sprintf("usage:user:%s", userID)
	r.redis.Incr(ctx, usageKey)
}

// GetQuotaRemaining returns the remaining scans for a Pro user
func (r *PaymentRepository) GetQuotaRemaining(ctx context.Context, userID string) (int64, error) {
	// 1. Get Limit from Postgres (Cached in redis sub_limit:<userID> for 5m)
	limitKey := fmt.Sprintf("sub_limit:%s", userID)
	var limit int64

	if r.redis != nil {
		val, err := r.redis.Get(ctx, limitKey).Int64()
		if err == nil {
			limit = val
		}
	}

	if limit == 0 {
		query := `SELECT quota_limit FROM user_subscriptions WHERE user_id = $1 AND status = 'active'`
		err := r.db.QueryRowContext(ctx, query, userID).Scan(&limit)
		if err != nil {
			return 0, err
		}
		if r.redis != nil {
			r.redis.Set(ctx, limitKey, limit, 5*time.Minute)
		}
	}

	// 2. Get Current Usage from Redis
	usageKey := fmt.Sprintf("usage:user:%s", userID)
	var usage int64
	if r.redis != nil {
		usage, _ = r.redis.Get(ctx, usageKey).Int64()
	}

	remaining := limit - usage
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}
