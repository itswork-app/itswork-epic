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

// IsPaid checks if a user has access to full analysis for a specific mint
func (r *PaymentRepository) IsPaid(ctx context.Context, userID, mint string) bool {
	cacheKey := fmt.Sprintf("payment_verified:%s:%s", userID, mint)

	// 1. Redis Check (Stateless Optimization)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil && val == "true" {
			return true
		}
	}

	// 2. Postgres Check (Source of Truth)
	var count int
	query := `SELECT COUNT(*) FROM payments WHERE user_id = $1 AND mint_address = $2 AND status = 'success'`
	err := r.db.QueryRowContext(ctx, query, userID, mint).Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Database query for payment status failed")
		return false
	}

	isPaid := count > 0

	// 3. Backfill cache if paid
	if isPaid && r.redis != nil {
		r.redis.Set(ctx, cacheKey, "true", 1*time.Hour)
	}

	return isPaid
}
