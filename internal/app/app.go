package app

import (
	"context"
	"database/sql"
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
	"itswork.app/pkg/cache"
	"itswork.app/pkg/database"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

type App struct {
	DB          *sql.DB
	Repo        *repository.TokenRepository
	Pub         *ingestor.Publisher
	BrainClient *processor.BrainClient
	Sub         *processor.Subscriber
	PayRepo     *repository.PaymentRepository
	Server      *http.Server
	Port        string
}

type AppOptions struct {
	DB *sql.DB
}

func SetupApp(opts ...AppOptions) (*App, error) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(os.Stdout)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var db *sql.DB
	var err error
	if len(opts) > 0 && opts[0].DB != nil {
		db = opts[0].DB
	} else {
		db, err = database.InitDB()
		if err != nil {
			return nil, err
		}
	}

	redisClient, err := cache.InitRedis()
	if err != nil {
		log.Warn().Err(err).Msg("Redis init failed - functioning without cache if expected")
	}

	repo := repository.NewTokenRepository(db, redisClient)
	payRepo := repository.NewPaymentRepository(db, redisClient)
	pub := ingestor.NewPublisher()

	// Initialize Observability (Sentry & OTel)
	initTelemetry()

	brainClient, err := processor.NewBrainClient()
	if err != nil {
		log.Warn().Err(err).Msg("gRPC Brain Client init failed - normal in restricted test envs")
		brainClient = &processor.BrainClient{}
	}

	sub, err := processor.InitSubscriber(brainClient, repo)
	if err != nil {
		log.Warn().Err(err).Msg("Subscriber init failed - normal in restricted test envs")
		sub = processor.NewSubscriber(brainClient, repo, nil, nil)
	}

	router := ingestor.SetupRouter(pub, repo, payRepo)
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	return &App{
		DB:          db,
		Repo:        repo,
		Pub:         pub,
		BrainClient: brainClient,
		Sub:         sub,
		PayRepo:     payRepo,
		Server:      srv,
		Port:        port,
	}, nil
}

func (a *App) Run() {
	go a.Sub.Start()

	go func() {
		log.Info().Str("port", a.Port).Msg("Server listening on port")
		if err := a.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("Failed to listen and serve")
		}
	}()
}

func (a *App) Shutdown(ctx context.Context) {
	log.Info().Msg("Initiating graceful shutdown...")
	if err := a.Server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}
	a.Sub.Shutdown()
	a.BrainClient.Close()
	if a.DB != nil {
		a.DB.Close()
	}
	a.Pub.Shutdown()

	// Flush Telemetry
	sentry.Flush(2 * time.Second)
	log.Info().Msg("Telemetry flushed")
}

func initTelemetry() {
	// 1. Sentry Initialization
	dsn := os.Getenv("SENTRY_DSN")
	if dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			EnableTracing:    true,
			TracesSampleRate: 1.0,
			Environment:      os.Getenv("ENV"),
		})
		if err != nil {
			log.Error().Err(err).Msg("Sentry init failed")
		} else {
			log.Info().Msg("Sentry initialized successfully")
		}
	} else {
		log.Warn().Msg("SENTRY_DSN not set, skipping Sentry init")
	}

	// 2. OpenTelemetry (Stdout for now, easily swappable to OTLP)
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize OTel stdout exporter")
		return
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("itswork-ingestor"),
		)),
	)
	otel.SetTracerProvider(tp)
	log.Info().Msg("OpenTelemetry Tracer Provider initialized (Stdout)")
}

func RunMain(opts ...AppOptions) error {
	app, err := SetupApp(opts...)
	if err != nil {
		return err
	}

	app.Run()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app.Shutdown(ctx)
	log.Info().Msg("Service exited properly")
	return nil
}
