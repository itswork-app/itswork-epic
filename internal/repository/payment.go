package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
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
			_ = r.AddUserCredits(ctx, userID, 50)
		} else if mint == "BUNDLE_100" {
			_ = r.AddUserCredits(ctx, userID, 100)
		} else if mint == "SUB_MONTHLY_PRO" {
			_ = r.ActivateSubscription(ctx, userID, "active")
		}

		// 2. Cache the successful access in Redis for 1 hour
		if r.redis != nil {
			cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)
			r.redis.Set(ctx, cacheKey, "true", 1*time.Hour)
		}
	}

	return nil
}

// IsPaid checks if a user has access to full analysis for a specific mint using hybrid logic
func (r *PaymentRepository) IsPaid(ctx context.Context, userID, mint string) bool {
	// Lazy-init user credits row if not exists
	_ = r.InitUserCredits(ctx, userID)

	cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)

	// 1. Redis Check (Stateless Optimization - represents any previously verified access)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil && val == "true" {
			log.Info().Str("user", userID).Str("mint", mint).Msg("Access granted via Cache")
			return true
		}
	}

	// STEP 1: Check active subscription
	if r.IsProSubscriber(ctx, userID) {
		r.cacheAccess(ctx, userID, mint)
		log.Info().Str("user", userID).Str("mint", mint).Str("usage_type", "SUBSCRIPTION").Msg("Access granted")
		return true
	}

	// STEP 2: Check and deduct credit
	deducted, err := r.DeductCredit(ctx, userID)
	if err == nil && deducted {
		r.cacheAccess(ctx, userID, mint)
		log.Info().Str("user", userID).Str("mint", mint).Str("usage_type", "CREDIT").Msg("Access granted")
		return true
	}

	// STEP 3: Check Postgres payments (Eceran)
	var count int
	query := `SELECT COUNT(*) FROM payments WHERE user_id = $1 AND mint_address = $2 AND status = 'success'`
	err = r.db.QueryRowContext(ctx, query, userID, mint).Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Database query for payment status failed")
		return false
	}

	isPaid := count > 0

	// Backfill cache if paid
	if isPaid {
		r.cacheAccess(ctx, userID, mint)
		log.Info().Str("user", userID).Str("mint", mint).Str("usage_type", "SINGLE_PAY").Msg("Access granted")
	}

	return isPaid
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

// ActivateSubscription activates or extends a user subscription
func (r *PaymentRepository) ActivateSubscription(ctx context.Context, userID, status string) error {
	query := `
		INSERT INTO user_subscriptions (user_id, status, expires_at)
		VALUES ($1, $2, now() + interval '30 days')
		ON CONFLICT (user_id) DO UPDATE
		SET status = $2, expires_at = CASE 
			WHEN user_subscriptions.expires_at > now() THEN user_subscriptions.expires_at + interval '30 days'
			ELSE now() + interval '30 days'
		END, updated_at = now()
	`
	_, err := r.db.ExecContext(ctx, query, userID, status)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Failed to activate subscription")
	}
	return err
}
