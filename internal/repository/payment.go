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

	// API quota tiers (Developer Portal)
	QuotaAPIPro        = 600
	QuotaAPIUltra      = 3600
	QuotaAPIEnterprise = 23100

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
			if err := r.ActivateSubscription(ctx, userID, "active", 30, QuotaProMonthly); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: ActivateSubscription Monthly")
				sentry.CaptureException(err)
			}
		} else if mint == "SUB_WEEKLY_PRO" {
			if err := r.ActivateSubscription(ctx, userID, "active", 7, QuotaProWeekly); err != nil {
				log.Error().Err(err).Str("user", userID).Msg("Fulfillment failed: ActivateSubscription Weekly")
				sentry.CaptureException(err)
			}
		} else if mint == "SUB_ULTRA_PRO" {
			if err := r.ActivateSubscription(ctx, userID, "active", 30, QuotaUltraMonthly); err != nil {
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

// IsPaid checks if a user has paid or eligible access for a specific mint.
// isAPI=true enforces the Developer Portal rules (no single-payment fallback).
// isAPI=false enforces the Marketing Portal rules (free 3 scans, sub, single pay).
func (r *PaymentRepository) IsPaid(ctx context.Context, userID, mint string, isAPI bool) bool {
	_ = r.InitUserCredits(ctx, userID)

	cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil && val == "true" {
			log.Info().Str("user", userID).Str("mint", mint).Msg("Access granted via Cache")
			return true
		}
	}

	// FREE TIER CHECK
	freeCacheKind := "ui"
	freeLimit := int64(FreeUIScans)
	if isAPI {
		freeCacheKind = "api"
		freeLimit = int64(FreeAPIUses)
	}
	used := r.GetFreeUsage(ctx, userID, freeCacheKind)
	if used < freeLimit {
		r.IncrementFreeUsage(ctx, userID, freeCacheKind)
		r.cacheAccess(ctx, userID, mint)
		log.Info().Str("user", userID).Str("mint", mint).Int64("used", used).Str("kind", freeCacheKind).Msg("Access granted via Free Tier")
		return true
	}

	// SUBSCRIPTION CHECK (both UI and API)
	if r.IsProSubscriber(ctx, userID) {
		remaining, _ := r.GetQuotaRemaining(ctx, userID)
		if remaining > 0 {
			r.IncrementUsage(ctx, userID)
			r.cacheAccess(ctx, userID, mint)
			log.Info().Str("user", userID).Str("mint", mint).Int64("remaining", remaining).Str("usage_type", "SUBSCRIPTION").Msg("Access granted")
			return true
		}
		log.Warn().Str("user", userID).Msg("Subscription quota exhausted")
	}

	// API path: no single-payment fallback — only subscription allowed
	if isAPI {
		return false
	}

	// UI ONLY: Credit deduction fallback
	deducted, err := r.DeductCredit(ctx, userID)
	if err == nil && deducted {
		r.cacheAccess(ctx, userID, mint)
		log.Info().Str("user", userID).Str("mint", mint).Str("usage_type", "CREDIT").Msg("Access granted")
		return true
	}

	// UI ONLY: Single payment lookup (eceran $0.50)
	var count int
	query := `SELECT COUNT(*) FROM payments WHERE user_id = $1 AND mint_address = $2 AND status = 'success'`
	err = r.db.QueryRowContext(ctx, query, userID, mint).Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Database query for payment status failed")
		return false
	}
	isPaid := count > 0
	if isPaid {
		r.cacheAccess(ctx, userID, mint)
		log.Info().Str("user", userID).Str("mint", mint).Str("usage_type", "SINGLE_PAY").Msg("Access granted")
	}
	return isPaid
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

// IncrementFreeUsage atomically increments the free usage counter in Redis and DB.
// kind: 'ui' or 'api'
func (r *PaymentRepository) IncrementFreeUsage(ctx context.Context, userID, kind string) {
	// 1. Increment Redis atomically
	redisKey := fmt.Sprintf("free:user:%s:%s", userID, kind)
	if r.redis != nil {
		r.redis.Incr(ctx, redisKey)
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

// ActivateSubscription activates or extends a user subscription with quota
func (r *PaymentRepository) ActivateSubscription(ctx context.Context, userID, status string, durationDays, quota int) error {
	if durationDays <= 0 {
		log.Error().Str("user", userID).Int("duration", durationDays).Msg("Invalid subscription duration")
		sentry.CaptureMessage(fmt.Sprintf("Invalid subscription duration for user %s: %d", userID, durationDays))
		return fmt.Errorf("invalid subscription duration: %d", durationDays)
	}

	query := `
		INSERT INTO user_subscriptions (user_id, status, expires_at, quota_limit, current_usage)
		VALUES ($1, $2, now() + interval '1 day' * $3, $4, 0)
		ON CONFLICT (user_id) DO UPDATE
		SET status = $2, 
			quota_limit = $4,
			current_usage = 0,
			expires_at = CASE 
				WHEN user_subscriptions.expires_at > now() THEN user_subscriptions.expires_at + interval '1 day' * $3
				ELSE now() + interval '1 day' * $3
			END, 
			updated_at = now()
	`
	_, err := r.db.ExecContext(ctx, query, userID, status, durationDays, quota)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Failed to activate subscription")
	}

	// Reset Redis usage counter on activation/renewal
	if r.redis != nil {
		usageKey := fmt.Sprintf("usage:user:%s", userID) // simplified period for now as per instructions
		r.redis.Del(ctx, usageKey)
	}

	return err
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
