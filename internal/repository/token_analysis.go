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

// SaveAnalysis persists the AI verdict for a token using UPSERT (ON CONFLICT)
func (r *TokenRepository) SaveAnalysis(ctx context.Context, mint, creator, verdict string, score int) error {
	query := `
		INSERT INTO token_analysis (mint_address, creator_id, verdict, rug_score, processed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (mint_address) DO UPDATE SET
			verdict = EXCLUDED.verdict,
			rug_score = EXCLUDED.rug_score,
			processed_at = EXCLUDED.processed_at;
	`

	_, err := r.db.ExecContext(ctx, query, mint, creator, verdict, score, time.Now())
	if err != nil {
		log.Error().Err(err).Str("mint", mint).Msg("Failed to UPSERT token analysis in Neon DB")
		return err
	}

	log.Debug().Str("mint", mint).Msg("Successfully persisted token analysis result")
	return nil
}
