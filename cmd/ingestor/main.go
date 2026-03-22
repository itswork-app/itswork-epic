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
	"itswork.app/internal/processor"
	"itswork.app/internal/repository"
	"itswork.app/pkg/database"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(os.Stdout)

	log.Info().Msg("Starting ItsWork Ingestor, Processor & Vault Service")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize Neon DB Connection Pool (The Vault)
	db, err := database.InitDB()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Neon DB")
	}

	// Initialize Repository
	repo := repository.NewTokenRepository(db)

	// Initialize PubSub Publisher
	pub := ingestor.NewPublisher()

	// Initialize BrainClient and Subscriber (The Nervous System)
	brainClient, err := processor.NewBrainClient()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize gRPC Brain Client")
	}

	sub, err := processor.NewSubscriber(brainClient, repo)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize PubSub Subscriber")
	}

	// Start Subscriber in a background goroutine
	go sub.Start()

	// Pass Publisher Dependency directly
	router := ingestor.SetupRouter(pub)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		log.Info().Str("port", port).Msg("Server listening on port")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("Failed to listen and serve")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Interrupt signal received. Shutting down service gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Graceful Shutdown Chain:
	// 1. Stop accepting new HTTP requests
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	// 2. Stop Pulling new messages from PubSub
	sub.Shutdown()

	// 3. Stop Connection to AI Brain
	brainClient.Close()

	// 4. Safely close Database Connection
	if db != nil {
		log.Info().Msg("Closing Neon DB connection pool...")
		db.Close()
	}

	// 5. Wait and safely flush messages to PubSub before container exits
	pub.Shutdown()

	log.Info().Msg("Service exited properly")
}
