package processor

import (
	"context"
	"errors"
	"os"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"itswork.app/api/proto"
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
	return NewBrainClientWithTarget(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func NewBrainClientWithTarget(target string, opts ...grpc.DialOption) (*BrainClient, error) {
	log.Info().Str("target", target).Msg("Connecting to Python AI Brain via gRPC...")

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}

	return &BrainClient{
		conn:   conn,
		client: proto.NewIntelligenceServiceClient(conn),
	}, nil
}

func (bc *BrainClient) AnalyzeToken(
	ctx context.Context, mint, creator string, walletAge int32,
	isLpBurned bool, concentration float32, fundingPassed bool,
	isRenounced bool, hasSocials bool,
	bondingProgress, tradeVelocity float32,
	hasGoldens bool, goldens []string,
	reputation string, failedCount int32, insiderRisk string,
) (*proto.VerdictResponse, error) {
	if bc.client == nil {
		return nil, errors.New("gRPC client not initialized")
	}
	req := &proto.TokenRequest{
		MintAddress:                      mint,
		CreatorAddress:                   creator,
		CreatorWalletAgeHours:            walletAge,
		IsLpBurned:                       isLpBurned,
		Top_10HolderConcentrationPercent: concentration,
		FundingSourceCheckPassed:         fundingPassed,
		IsRenounced:                      isRenounced,
		HasSocials:                       hasSocials,
		BondingProgress:                  bondingProgress,
		TradeVelocity:                    tradeVelocity,
		HasGoldenWallets:                 hasGoldens,
		GoldenWallets:                    goldens,
		CreatorReputation:                reputation,
		FailedProjectsCount:              failedCount,
		InsiderRisk:                      insiderRisk,
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
