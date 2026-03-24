package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"itswork.app/api/proto"
)

// TokenRepository handles database operations for token analysis
type TokenRepository struct {
	db    *sql.DB
	redis *redis.Client
}

// NewTokenRepository creates a new instance of TokenRepository
func NewTokenRepository(db *sql.DB, rdb *redis.Client) *TokenRepository {
	return &TokenRepository{db: db, redis: rdb}
}

func (r *TokenRepository) GetDB() *sql.DB {
	return r.db
}

func (r *TokenRepository) GetRedis() *redis.Client {
	return r.redis
}

// SaveAnalysis persists the AI verdict for a token using a nested transaction (Wallets + TokenAnalysis)
func (r *TokenRepository) SaveAnalysis(ctx context.Context, mint, creator, verdict, reason string, score int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// ALWAYS Rollback if any error occurs; No-op if already committed
	defer func() {
		_ = tx.Rollback()
	}()

	// Stage 1: UPSERT Wallet and retrieve UUID
	var creatorID string
	walletQuery := `
		INSERT INTO wallets (address) 
		VALUES ($1) 
		ON CONFLICT (address) DO UPDATE SET last_seen = now() 
		RETURNING id;
	`
	err = tx.QueryRowContext(ctx, walletQuery, creator).Scan(&creatorID)
	if err != nil {
		log.Error().Err(err).Str("creator", creator).Msg("Transaction failed: Wallet UPSERT error")
		return err
	}

	// Stage 2: UPSERT Token Analysis linked by creatorID
	analysisQuery := `
		INSERT INTO token_analysis (mint_address, creator_id, verdict, rug_score, reason, processed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (mint_address) DO UPDATE SET
			verdict = EXCLUDED.verdict,
			rug_score = EXCLUDED.rug_score,
			reason = EXCLUDED.reason,
			processed_at = EXCLUDED.processed_at;
	`
	_, err = tx.ExecContext(ctx, analysisQuery, mint, creatorID, verdict, score, reason, time.Now())
	if err != nil {
		log.Error().Err(err).Str("mint", mint).Msg("Transaction failed: Token Analysis UPSERT error")
		return err
	}

	// Commit Transaction
	if err := tx.Commit(); err != nil {
		log.Error().Err(err).Msg("Transaction commit failed")
		return err
	}

	log.Debug().Str("mint", mint).Msg("Atomic transaction successful: Wallet and Analysis persisted")
	return nil
}

// GetAnalysis retrieves the token verdict utilizing a Look-Aside Caching Strategy with Upstash Redis
func (r *TokenRepository) GetAnalysis(ctx context.Context, mint string, isPaid bool) (*proto.VerdictResponse, error) {
	cacheKey := fmt.Sprintf("token_verdict:%s", mint)

	// 1. Cache Check (Look-Aside)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var resp proto.VerdictResponse
			if unmarshalErr := json.Unmarshal([]byte(val), &resp); unmarshalErr == nil {
				log.Debug().Str("mint", mint).Msg("Cache Hit: Served Verdict from Redis")
				return &resp, nil
			}
			log.Warn().Err(err).Msg("Failed to unmarshal cached verdict, falling back to database")
		} else if err != redis.Nil {
			log.Warn().Err(err).Msg("Redis GET error, falling back to database")
		}
	}

	// 2. Cache Miss: Query Database
	query := `
		SELECT verdict, rug_score, reason
		FROM token_analysis
		WHERE mint_address = $1
	`
	var verdict, reason string
	var score int32

	err := r.db.QueryRowContext(ctx, query, mint).Scan(&verdict, &score, &reason)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("analysis not found for mint: %s", mint)
		}
		log.Error().Err(err).Str("mint", mint).Msg("Database query failed")
		return nil, err
	}

	resp := &proto.VerdictResponse{
		Score:   score,
		Verdict: verdict,
		Reason:  reason,
	}

	// 3. Cache Miss Recovery: Store result in Redis configured with 5-minute TTL
	if r.redis != nil {
		data, marshalErr := json.Marshal(resp)
		if marshalErr == nil {
			err = r.redis.Set(ctx, cacheKey, data, 5*time.Minute).Err()
			if err != nil {
				log.Warn().Err(err).Msg("Failed to cache verdict to Redis")
			} else {
				log.Debug().Str("mint", mint).Msg("Cache Miss: Fetched from DB and cached outcome to Redis")
			}
		}
	}

	return resp, nil
}
