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
	return InitDBWithDriver("postgres", dsn)
}

func InitDBWithDriver(driver, dsn string) (*sql.DB, error) {
	if dsn == "" {
		log.Warn().Msg("DATABASE_URL not set, DB features will be unavailable or fail")
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	// Industrial Grade Connection Pooling Configuration
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection on startup
	if err := db.Ping(); err != nil {
		log.Error().Err(err).Msg("Failed to ping Neon DB")
		return db, err
	}

	log.Info().Msg("Successfully connected to Neon DB")
	return db, nil
}
