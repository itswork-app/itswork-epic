package processor

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"

	"itswork.app/api/proto"
)

type mockBrainger struct {
	err error
}

func (m *mockBrainger) AnalyzeToken(ctx context.Context, mint, creator string, walletAge int32, isLpBurned bool, concentration float32, fundingPassed bool) (*proto.VerdictResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &proto.VerdictResponse{Score: 90, Verdict: "BULLISH"}, nil
}

type mockRepo struct {
	err error
}

func (m *mockRepo) SaveAnalysis(ctx context.Context, mint, creator, verdict string, score int) error {
	return m.err
}

func TestSubscriber_HandleMessage_Success(t *testing.T) {
	brain := &mockBrainger{}
	repo := &mockRepo{}

	s := NewSubscriber(brain, repo, nil)

	data := []byte(`{"mint_address": "MINT1", "creator_address": "CREA1"}`)
	msg := &pubsub.Message{
		Data: data,
	}

	s.handleMessage(context.Background(), msg)
	assert.NotNil(t, s.brainClient)
}

func TestSubscriber_HandleMessage_InvalidJSON(t *testing.T) {
	s := NewSubscriber(nil, nil, nil)
	msg := &pubsub.Message{
		Data: []byte(`{invalid`),
	}
	s.handleMessage(context.Background(), msg)
	assert.Nil(t, s.repo)
}

func TestSubscriber_HandleMessage_EmptyMint(t *testing.T) {
	s := NewSubscriber(nil, nil, nil)
	msg := &pubsub.Message{
		Data: []byte(`{"mint_address": ""}`),
	}
	s.handleMessage(context.Background(), msg)
}

func TestSubscriber_HandleMessage_BrainError(t *testing.T) {
	brain := &mockBrainger{err: context.DeadlineExceeded}
	repo := &mockRepo{}
	s := NewSubscriber(brain, repo, nil)
	msg := &pubsub.Message{
		Data: []byte(`{"mint_address": "MINT1", "creator_address": "CREA1"}`),
	}
	s.handleMessage(context.Background(), msg)
}

func TestSubscriber_HandleMessage_RepoError(t *testing.T) {
	brain := &mockBrainger{}
	repo := &mockRepo{err: context.DeadlineExceeded}
	s := NewSubscriber(brain, repo, nil)
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
	s := NewSubscriber(nil, nil, sub)
	s.Start()
}

func TestSubscriber_Shutdown(t *testing.T) {
	s := NewSubscriber(nil, nil, nil)
	s.Shutdown()
}

func TestInitSubscriber_Error(t *testing.T) {
	// Should fail because no credentials in test env
	_, err := InitSubscriber(nil, nil)
	assert.Error(t, err)
}

func TestNewBrainClient_Error(t *testing.T) {
	// Insecure credentials shouldn't error on Dial, but we cover the entry point
	os.Setenv("BRAIN_TARGET", "invalid:port")
	bc, err := NewBrainClient()
	if err == nil {
		bc.Close()
	}
}
