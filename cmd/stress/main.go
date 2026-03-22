package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/app"
)

func main() {
	// 1. Start MiniRedis
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start miniredis")
	}
	defer mr.Close()

	// Set REDIS_URL for the app to pick up
	os.Setenv("REDIS_URL", "redis://"+mr.Addr())
	log.Info().Str("addr", mr.Addr()).Msg("Mock Redis started")

	// 2. Start SQL Mock (Postgres)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start sqlmock")
	}
	defer db.Close()

	// Expect pings
	mock.ExpectPing()

	log.Info().Msg("Mock Postgres (sqlmock) started")

	// 3. Setup and Run App
	application, err := app.SetupApp(app.AppOptions{
		DB: db,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to setup app with mocks")
	}

	application.Run()

	log.Info().Msg("🚀 Mock Stress Server is LIVE on :8080")
	log.Info().Msg("Ready for 'The Proving Grounds' stress test.")

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	application.Shutdown(ctx)
	log.Info().Msg("Mock Stress Server shut down")
}
