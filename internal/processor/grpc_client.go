package processor

import (
	"context"
	"os"

	"itswork.app/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"github.com/rs/zerolog/log"
)

type BrainClient struct {
	conn   *grpc.ClientConn
	client proto.IntelligenceServiceClient
}

func NewBrainClient() (*BrainClient, error) {
	target := os.Getenv("BRAIN_TARGET")
	if target == "" {
		target = "localhost:50051"
	}

	log.Info().Str("target", target).Msg("Connecting to Python AI Brain via gRPC...")

	// Setup connection with insecure credentials for local/internal cluster communication
	// Industrial Grade: Using NewClient (Non-blocking) as DialContext is deprecated.
	conn, err := grpc.NewClient(target, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	return &BrainClient{
		conn:   conn,
		client: proto.NewIntelligenceServiceClient(conn),
	}, nil
}

func (bc *BrainClient) AnalyzeToken(ctx context.Context, mint, creator string) (*proto.VerdictResponse, error) {
	req := &proto.TokenRequest{
		MintAddress:    mint,
		CreatorAddress: creator,
	}

	// High Performance: Calling async Brain via standard gRPC call
	resp, err := bc.client.AnalyzeToken(ctx, req)
	if err != nil {
		log.Error().Err(err).Str("mint", mint).Msg("gRPC AnalyzeToken failed")
		return nil, err
	}

	return resp, nil
}

func (bc *BrainClient) Close() {
	if bc.conn != nil {
		log.Info().Msg("Closing Brain gRPC connection...")
		bc.conn.Close()
	}
}
