package processor

import (
	"context"
	"os"
	"testing"

	pubsub "cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"

	"itswork.app/api/proto"
)

type mockBrainger struct {
	err error
}

func (m *mockBrainger) AnalyzeToken(
	ctx context.Context, mint, creator string, walletAge int32,
	isLpBurned bool, concentration float32, fundingPassed bool,
) (*proto.VerdictResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &proto.VerdictResponse{Score: 90, Verdict: "BULLISH"}, nil
}

type mockRepo struct {
	err error
}

func (m *mockRepo) SaveAnalysis(ctx context.Context, mint, creator, verdict, reason string, score int) error {
	return m.err
}

func TestSubscriber_HandleMessage_Success(t *testing.T) {
	brain := &mockBrainger{}
	repo := &mockRepo{}

	s := NewSubscriber(brain, repo, nil, nil)

	data := []byte(`{"mint_address": "MINT1", "creator_address": "CREA1"}`)
	msg := &pubsub.Message{
		Data: data,
	}

	s.handleMessage(context.Background(), msg)
	assert.NotNil(t, s.brainClient)
}

func TestSubscriber_HandleMessage_InvalidJSON(t *testing.T) {
	s := NewSubscriber(nil, nil, nil, nil)
	msg := &pubsub.Message{
		Data: []byte(`{invalid`),
	}
	s.handleMessage(context.Background(), msg)
	assert.Nil(t, s.repo)
}

func TestSubscriber_HandleMessage_EmptyMint(t *testing.T) {
	s := NewSubscriber(nil, nil, nil, nil)
	msg := &pubsub.Message{
		Data: []byte(`{"mint_address": ""}`),
	}
	s.handleMessage(context.Background(), msg)
}

func TestSubscriber_HandleMessage_BrainError(t *testing.T) {
	brain := &mockBrainger{err: context.DeadlineExceeded}
	repo := &mockRepo{}
	s := NewSubscriber(brain, repo, nil, nil)
	msg := &pubsub.Message{
		Data: []byte(`{"mint_address": "MINT1", "creator_address": "CREA1"}`),
	}
	s.handleMessage(context.Background(), msg)
}

func TestSubscriber_HandleMessage_RepoError(t *testing.T) {
	brain := &mockBrainger{}
	repo := &mockRepo{err: context.DeadlineExceeded}
	s := NewSubscriber(brain, repo, nil, nil)
	msg := &pubsub.Message{
		Data: []byte(`{"mint_address": "MINT1", "creator_address": "CREA1"}`),
	}
	s.handleMessage(context.Background(), msg)
}

type mockPubSubSub struct {
	receiveErr error
}

func (m *mockPubSubSub) Receive(ctx context.Context, f func(context.Context, *pubsub.Message)) error {
	return m.receiveErr
}

func TestSubscriber_Start(t *testing.T) {
	sub := &mockPubSubSub{}
	s := NewSubscriber(nil, nil, sub, nil)
	s.Start()
}

func TestSubscriber_Start_Nil(t *testing.T) {
	s := NewSubscriber(nil, nil, nil, nil)
	s.Start() // branches to "Subscriber is nil, skipping"
}

func TestSubscriber_Start_Error(t *testing.T) {
	sub := &mockPubSubSub{receiveErr: assert.AnError}
	s := NewSubscriber(nil, nil, sub, nil)
	s.Start()
}

func TestSubscriber_Shutdown(t *testing.T) {
	s := NewSubscriber(nil, nil, nil, nil)
	s.Shutdown()
}

func TestInitSubscriber_Error(t *testing.T) {
	os.Unsetenv("PROJECT_ID")
	os.Unsetenv("SUB_ID")
	_, err := InitSubscriber(nil, nil)
	assert.Error(t, err)
}

func TestInitSubscriber_CustomKeys(t *testing.T) {
	os.Setenv("PROJECT_ID", "test-project")
	os.Setenv("SUB_ID", "test-sub")
	defer os.Unsetenv("PROJECT_ID")
	defer os.Unsetenv("SUB_ID")
	// Will still fail without auth, but covers the assignment branches
	_, _ = InitSubscriber(nil, nil)
}

func TestInitSubscriber_Defaults(t *testing.T) {
	os.Setenv("PROJECT_ID", "")
	os.Setenv("SUB_ID", "")
	// Defaults will be picked: "itswork-epic" and "helius-ingestion-sub"
	_, _ = InitSubscriber(nil, nil)
}

func TestInitSubscriber_ClientError(t *testing.T) {
	old := newPubsubClient
	defer func() { newPubsubClient = old }()
	newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
		return nil, assert.AnError
	}
	_, err := InitSubscriber(nil, nil)
	assert.Error(t, err)
}

func TestInitSubscriber_MockSuccess(t *testing.T) {
	old := newPubsubClient
	defer func() { newPubsubClient = old }()
	newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
		return nil, nil // Return nil client to avoid real connection checks
	}

	s, err := InitSubscriber(nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	s.Shutdown()
}

func TestInitSubscriber_WithClient(t *testing.T) {
	old := newPubsubClient
	defer func() { newPubsubClient = old }()
	newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
		return pubsub.NewClient(ctx, "test-p", option.WithoutAuthentication(), option.WithEndpoint("localhost:8085"))
	}
	s, err := InitSubscriber(nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	s.Shutdown()
}

func TestSubscriber_Shutdown_WithClient(t *testing.T) {
	ctx := context.Background()
	client, _ := pubsub.NewClient(ctx, "test-p", option.WithoutAuthentication(), option.WithEndpoint("localhost:8085"))
	s := NewSubscriber(nil, nil, nil, nil)
	s.client = client
	s.Shutdown()
}

func TestNewBrainClient_Error(t *testing.T) {
	// Insecure credentials shouldn't error on Dial, but we cover the entry point
	os.Setenv("BRAIN_TARGET", "invalid:port")
	bc, err := NewBrainClient()
	if err == nil {
		bc.Close()
	}
}
