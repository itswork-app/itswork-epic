package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

type AuthRepository struct {
	db    *sql.DB
	redis *redis.Client
}

func NewAuthRepository(db *sql.DB, rdb *redis.Client) *AuthRepository {
	return &AuthRepository{db: db, redis: rdb}
}

// GetUserIDByAPIKey retrieves the user ID associated with a given API key hash.
// Uses Redis as a primary stateless cache to ensure sub-50ms latency.
func (r *AuthRepository) GetUserIDByAPIKey(ctx context.Context, apiKeyHash string) (string, error) {
	cacheKey := fmt.Sprintf("api_key_user:%s", apiKeyHash)

	// 1. Redis Lookup (Stateless First)
	if r.redis != nil {
		userID, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil && userID != "" {
			return userID, nil
		}
	}

	// 2. Postgres Fallback
	var userID string
	var status string
	query := `SELECT user_id, status FROM api_keys WHERE api_key_hash = $1`
	err := r.db.QueryRowContext(ctx, query, apiKeyHash).Scan(&userID, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Not found
		}
		log.Error().Err(err).Msg("Failed to query api_keys table")
		return "", err
	}

	if status != "active" {
		return "", nil // Key revoked or inactive
	}

	// 3. Backfill Cache (1 hour TTL for active keys)
	if r.redis != nil {
		r.redis.Set(ctx, cacheKey, userID, 1*time.Hour)
	}

	return userID, nil
}

// SaveAPIKey persists a new API key hash for a user.
func (r *AuthRepository) SaveAPIKey(ctx context.Context, userID, apiKeyHash, label string) error {
	query := `
		INSERT INTO api_keys (user_id, api_key_hash, label, status)
		VALUES ($1, $2, $3, 'active')
		ON CONFLICT (api_key_hash) DO NOTHING;
	`
	_, err := r.db.ExecContext(ctx, query, userID, apiKeyHash, label)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Failed to save API key hash")
		return err
	}
	return nil
}

// Master Blueprint Read & Verified.
