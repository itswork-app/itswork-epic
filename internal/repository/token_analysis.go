package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"
)

// TokenRepository handles database operations for token analysis
type TokenRepository struct {
	db *sql.DB
}

// NewTokenRepository creates a new instance of TokenRepository
func NewTokenRepository(db *sql.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

// SaveAnalysis persists the AI verdict for a token using a nested transaction (Wallets + TokenAnalysis)
func (r *TokenRepository) SaveAnalysis(ctx context.Context, mint, creator, verdict string, score int) error {
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
		INSERT INTO token_analysis (mint_address, creator_id, verdict, rug_score, processed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (mint_address) DO UPDATE SET
			verdict = EXCLUDED.verdict,
			rug_score = EXCLUDED.rug_score,
			processed_at = EXCLUDED.processed_at;
	`
	_, err = tx.ExecContext(ctx, analysisQuery, mint, creatorID, verdict, score, time.Now())
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
