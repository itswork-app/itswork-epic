package database

import (
	"database/sql"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

// InitDB initializes a connection pool to Neon DB.
func InitDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Warn().Msg("DATABASE_URL not set, DB features will be unavailable or fail")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// Industrial Grade Connection Pooling Configuration
	// Optimized for High Concurrency & Latency Requirements
	db.SetMaxOpenConns(25)                 // Menghindari kelebihan koneksi ke Neon (Free/Pro tier limits)
	db.SetMaxIdleConns(25)                 // Menjaga koneksi tetap hangat untuk akses cepat
	db.SetConnMaxLifetime(5 * time.Minute) // Menghindari stale connections

	// Verify connection on startup
	if err := db.Ping(); err != nil {
		log.Error().Err(err).Msg("Failed to ping Neon DB")
		return nil, err
	}

	log.Info().Msg("Successfully connected to Neon DB with connection pooling")
	return db, nil
}
