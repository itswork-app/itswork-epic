package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/ingestor"
)

func main() {
	// Initialize Structured Logging (JSON format for Google Cloud Logging)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(os.Stdout)

	log.Info().Msg("Starting ItsWork Ingestor Service")

	// Ensure Stateless execution by pulling configs from standard Env Vars
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := ingestor.SetupRouter()

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Run the server in a goroutine so that it doesn't block graceful shutdown
	go func() {
		log.Info().Str("port", port).Msg("Server listening on port")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("Failed to listen and serve")
		}
	}()

	// Graceful Shutdown logic
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Interrupt signal received. Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited properly")
}
