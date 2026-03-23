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
func (r *PaymentRepository) UpdatePaymentStatus(ctx context.Context, reference, status string) error {
	var userID, mint string
	query := `
		UPDATE payments
		SET status = $1, updated_at = now()
		WHERE reference_key = $2
		RETURNING user_id, mint_address;
	`
	err := r.db.QueryRowContext(ctx, query, status, reference).Scan(&userID, &mint)
	if err != nil {
		return err
	}

	// Stateless: Cache the successful payment in Redis for 1 hour to bypass DB on analysis requests
	if status == "success" && r.redis != nil {
		cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)
		r.redis.Set(ctx, cacheKey, "true", 1*time.Hour)
	}

	return nil
}

// IsPaid checks if a user has access to full analysis for a specific mint using hybrid logic
func (r *PaymentRepository) IsPaid(ctx context.Context, userID, mint string) bool {
	cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)

	// 1. Redis Check (Stateless Optimization - represents any previously verified access)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil && val == "true" {
			return true
		}
	}

	// STEP 1: Check active subscription
	if r.HasActiveSubscription(ctx, userID) {
		r.cacheAccess(ctx, userID, mint)
		return true
	}

	// STEP 3: Check if already paid eceran (Done before credit deduction to save user credits)
	// Although instructions say Step 2 is credit, usually we check if they already bought it first.
	// But I will stick to instructions if they are strict.
	// Re-reading: "STEP 3: Cek apakah koin ini sudah pernah dibayar eceran... Jika ya, Unlock."
	// I'll swap STEP 2 and 3 if it makes more sense, but the prompt is explicit about the order.
	// I will follow the prompt's order: Sub -> Credit -> Eceran.

	// STEP 2: Check and deduct credit
	deducted, err := r.DeductCredit(ctx, userID)
	if err == nil && deducted {
		r.cacheAccess(ctx, userID, mint)
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
	}

	return isPaid
}

// HasActiveSubscription checks if the user has a 'active' subscription that hasn't expired
func (r *PaymentRepository) HasActiveSubscription(ctx context.Context, userID string) bool {
	var count int
	query := `
		SELECT COUNT(*)
		FROM user_subscriptions
		WHERE user_id = $1 AND status = 'active' AND expires_at > now()
	`
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Failed to check subscription")
		return false
	}
	return count > 0
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
