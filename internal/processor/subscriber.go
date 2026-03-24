package processor

import (
	"context"
	"encoding/json"
	"os"

	"cloud.google.com/go/pubsub/v2" // Standardized to use v2 across project
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"

	"itswork.app/api/proto"
)

var newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
	return pubsub.NewClient(ctx, projectID, opts...)
}

// Brainger defines the interface for AI analysis calls
type Brainger interface {
	AnalyzeToken(
		ctx context.Context, mint, creator string, walletAge int32,
		isLpBurned bool, concentration float32, fundingPassed bool,
		isRenounced bool, hasSocials bool,
		bondingProgress, tradeVelocity float32,
		hasGoldens bool, goldens []string,
		reputation string, failedCount int32, insiderRisk string,
	) (*proto.VerdictResponse, error)
}

// VaultRepository defines the interface for data persistence
type VaultRepository interface {
	SaveAnalysis(ctx context.Context, mint, creator, verdict, reason string, score int) error
	GetRedis() *redis.Client
}

// HeliusPayload represents the simplified structure to extract needed fields
type HeliusPayload struct {
	MintAddress                     string  `json:"mint_address"`
	CreatorAddress                  string  `json:"creator_address"`
	CreatorWalletAgeHours           int32   `json:"creator_wallet_age_hours"`
	IsLpBurned                      bool    `json:"is_lp_burned"`
	Top10HolderConcentrationPercent float32 `json:"top_10_holder_concentration_percent"`
	FundingSourceCheckPassed        bool    `json:"funding_source_check_passed"`
	IsRenounced                     bool     `json:"is_renounced"`
	HasSocials                      bool     `json:"has_socials"`
	HasGoldenWallets                bool     `json:"has_goldens"`
	GoldenWallets                   []string `json:"golden_wallets"`
	CreatorReputation               string   `json:"creator_reputation"`
	FailedProjectsCount             int      `json:"failed_projects_count"`
	InsiderRisk                     string   `json:"insider_risk"`
}

// PubSubSubscriber defines the interface for pulling messages
type PubSubSubscriber interface {
	Receive(ctx context.Context, f func(context.Context, *pubsub.Message)) error
}

type Subscriber struct {
	client      *pubsub.Client
	subscriber  PubSubSubscriber
	brainClient Brainger
	repo        VaultRepository
	enricher    *Enricher
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewSubscriber(brainClient Brainger, repo VaultRepository, subscriber PubSubSubscriber, enricher *Enricher) *Subscriber {
	ctx, cancel := context.WithCancel(context.Background())

	return &Subscriber{
		subscriber:  subscriber,
		brainClient: brainClient,
		repo:        repo,
		enricher:    enricher,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func InitSubscriber(brainClient Brainger, repo VaultRepository) (*Subscriber, error) {
	projectID := os.Getenv("PROJECT_ID")
	subID := os.Getenv("SUB_ID")
	if projectID == "" {
		projectID = "itswork-epic"
	}
	if subID == "" {
		subID = "helius-ingestion-sub"
	}

	ctx := context.Background()
	client, err := newPubsubClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	apiKey := os.Getenv("HELIUS_API_KEY")
	enricher := NewEnricher(apiKey, repo.GetRedis())

	var sub PubSubSubscriber
	if client != nil {
		sub = client.Subscriber(subID)
	}

	s := NewSubscriber(brainClient, repo, sub, enricher)
	s.client = client
	return s, nil
}

func (s *Subscriber) Start() {
	if s.subscriber == nil {
		log.Warn().Msg("Subscriber is nil, skipping Receive loop (likely in test mode)")
		return
	}
	log.Info().Msg("Starting Pub/Sub Subscriber worker...")

	// Concurrent Processing: Receive utilizes a pool of goroutines internally
	err := s.subscriber.Receive(s.ctx, func(ctx context.Context, msg *pubsub.Message) {
		s.handleMessage(ctx, msg)
	})

	if err != nil {
		log.Error().Err(err).Msg("Subscriber Receive loop failed")
	}
}

func (s *Subscriber) handleMessage(ctx context.Context, msg *pubsub.Message) {
	var payload HeliusPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal Helius payload - Message ignored/Acked to clear queue")
		msg.Ack()
		return
	}

	if payload.MintAddress == "" {
		log.Warn().Msg("Empty MintAddress in payload - Acked")
		msg.Ack()
		return
	}

	log.Debug().Str("mint", payload.MintAddress).Msg("Processing token from Pub/Sub...")

	// Enrich the payload with real on-chain data before sending to Brain
	if s.enricher != nil {
		_ = s.enricher.Enrich(ctx, &payload)
	}
	// Invoke gRPC AnalyzeToken to Python Brain with REAL data from Ingestor
	resp, err := s.brainClient.AnalyzeToken(
		ctx,
		payload.MintAddress,
		payload.CreatorAddress,
		payload.CreatorWalletAgeHours,
		payload.IsLpBurned,
		payload.Top10HolderConcentrationPercent,
		payload.FundingSourceCheckPassed,
		payload.IsRenounced,
		payload.HasSocials,
		0, 0, // BondingProgress, TradeVelocity
		payload.HasGoldenWallets,
		payload.GoldenWallets,
		payload.CreatorReputation,
		int32(payload.FailedProjectsCount),
		payload.InsiderRisk,
	)
	if err != nil {
		log.Error().Err(err).Str("mint", payload.MintAddress).Msg("Intelligence Analysis failed")
		msg.Nack() // Requeue for retry if gRPC is down
		return
	}

	// Persist Analysis result to Neon DB via Repository Layer
	err = s.repo.SaveAnalysis(ctx, payload.MintAddress, payload.CreatorAddress, resp.Verdict, resp.Reason, int(resp.Score))
	if err != nil {
		// Log error is handled in Repository, but we Decide Nack or Ack here
		// Standard: Nack to allow retry if DB is temporarily unstable
		msg.Nack()
		return
	}

	// Output Industrial Intelligence Result
	log.Info().
		Str("mint", payload.MintAddress).
		Int32("score", resp.Score).
		Str("verdict", resp.Verdict).
		Str("reason", resp.Reason).
		Msg("🚀 Token Intelligence Result Persisted Successfully")

	// Master Blueprint: Successful processing REQUIRES an Ack
	msg.Ack()
}

func (s *Subscriber) Shutdown() {
	log.Info().Msg("Initiating Subscriber graceful shutdown...")
	s.cancel()
	if s.client != nil {
		s.client.Close()
	}
}
