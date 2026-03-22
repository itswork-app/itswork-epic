package processor

import (
	"context"
	"encoding/json"
	"os"

	"cloud.google.com/go/pubsub/v2" // Standardized to use v2 across project
	"github.com/rs/zerolog/log"
)

// HeliusPayload represents the simplified structure to extract needed fields
type HeliusPayload struct {
	MintAddress    string `json:"mint_address"`
	CreatorAddress string `json:"creator_address"`
	// Tambahkan field lain jika spek Helius berkembang
}

type Subscriber struct {
	client      *pubsub.Client
	subscriber  *pubsub.Subscriber
	brainClient *BrainClient
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewSubscriber(brainClient *BrainClient) (*Subscriber, error) {
	projectID := os.Getenv("PROJECT_ID")
	subID := os.Getenv("SUB_ID")
	if projectID == "" {
		projectID = "itswork-epic"
	}
	if subID == "" {
		subID = "helius-ingestion-sub"
	}

	ctx, cancel := context.WithCancel(context.Background())

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		cancel()
		return nil, err
	}

	subscriber := client.Subscriber(subID)

	return &Subscriber{
		client:      client,
		subscriber:  subscriber,
		brainClient: brainClient,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

func (s *Subscriber) Start() {
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

	// Invoke gRPC AnalyzeToken to Python Brain
	resp, err := s.brainClient.AnalyzeToken(ctx, payload.MintAddress, payload.CreatorAddress)
	if err != nil {
		log.Error().Err(err).Str("mint", payload.MintAddress).Msg("Intelligence Analysis failed")
		msg.Nack() // Requeue for retry if gRPC is down
		return
	}

	// Output Industrial Intelligence Result
	log.Info().
		Str("mint", payload.MintAddress).
		Int32("score", resp.Score).
		Str("verdict", resp.Verdict).
		Str("reason", resp.Reason).
		Msg("🚀 Token Intelligence Result Received")

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
