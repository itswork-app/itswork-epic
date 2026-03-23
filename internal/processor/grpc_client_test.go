package processor

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"itswork.app/api/proto"
)

type mockIntelligenceServer struct {
	proto.UnimplementedIntelligenceServiceServer
}

func (m *mockIntelligenceServer) AnalyzeToken(ctx context.Context, req *proto.TokenRequest) (*proto.VerdictResponse, error) {
	return &proto.VerdictResponse{
		Score:   80,
		Verdict: "SUSPICIOUS",
		Reason:  "Mocked reasoning for " + req.MintAddress,
	}, nil
}

func TestBrainClient_AnalyzeToken_Success(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	proto.RegisterIntelligenceServiceServer(s, &mockIntelligenceServer{})
	go func() {
		if err := s.Serve(lis); err != nil {
			panic(err)
		}
	}()

	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	client, err := NewBrainClientWithTarget("passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	if client != nil {
		defer client.Close()
	}

	resp, err := client.AnalyzeToken(context.Background(), "mint1", "creator1", 48, true, 30.0, true)
	assert.NoError(t, err)
	assert.Equal(t, int32(80), resp.Score)
	assert.Equal(t, "SUSPICIOUS", resp.Verdict)
}

func TestBrainClient_AnalyzeToken_NilClient(t *testing.T) {
	bc := &BrainClient{client: nil}
	_, err := bc.AnalyzeToken(context.Background(), "mint", "creator", 48, true, 30.0, true)
	assert.Error(t, err)
}

func TestBrainClient_AnalyzeToken_Error(t *testing.T) {
	// Create a mock server that returns an error
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	mockServer := &mockIntelligenceServer{}
	// override the mock to return an error, but wait, the struct doesn't have an err field.
	// let's just close the server immediately so it fails
	proto.RegisterIntelligenceServiceServer(s, mockServer)
	go s.Serve(lis)
	s.Stop() // stop it so dialing or calling fails

	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	client, err := NewBrainClientWithTarget("passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	defer client.Close()

	_, err = client.AnalyzeToken(context.Background(), "mint1", "creator1", 48, true, 30.0, true)
	assert.Error(t, err)
}

func TestNewBrainClient_DefaultTarget(t *testing.T) {
	os.Unsetenv("BRAIN_TARGET")
	bc, _ := NewBrainClient()
	if bc != nil {
		bc.Close()
	}
}
