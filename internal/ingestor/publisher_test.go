package ingestor

import (
	"context"
	"os"
	"testing"
	"time"

	pubsub "cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

type mockTopic struct {
	stopped bool
}

func (m *mockTopic) Publish(ctx context.Context, msg *pubsub.Message) *pubsub.PublishResult {
	return &pubsub.PublishResult{} 
}

func (m *mockTopic) Stop() {
	m.stopped = true
}

func TestNewPublisher_MockSuccess(t *testing.T) {
	old := newPubsubClient
	defer func() { newPubsubClient = old }()
	newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
		return nil, nil
	}

	pub := NewPublisher()
	assert.NotNil(t, pub)
	pub.topicPub = &mockTopic{}
	
	pub.PublishChan <- []byte("test")
	time.Sleep(50 * time.Millisecond)
	
	pub.Shutdown()
	assert.True(t, pub.topicPub.(*mockTopic).stopped)
}

func TestNewPublisher_ClientError(t *testing.T) {
	old := newPubsubClient
	defer func() { newPubsubClient = old }()
	newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
		return nil, assert.AnError
	}
	pub := NewPublisher()
	assert.NotNil(t, pub)
}

func TestNewPublisher_WithClient(t *testing.T) {
	old := newPubsubClient
	defer func() { newPubsubClient = old }()
	newPubsubClient = func(ctx context.Context, projectID string, opts ...option.ClientOption) (*pubsub.Client, error) {
		return pubsub.NewClient(ctx, "test-p", option.WithoutAuthentication(), option.WithEndpoint("localhost:8085"))
	}
	pub := NewPublisher()
	assert.NotNil(t, pub)
	pub.Shutdown()
}

func TestNewPublisher_CustomKeys(t *testing.T) {
	os.Setenv("PROJECT_ID", "test-project")
	os.Setenv("TOPIC_ID", "test-topic")
	defer os.Unsetenv("PROJECT_ID")
	defer os.Unsetenv("TOPIC_ID")

	pub := NewPublisher()
	assert.NotNil(t, pub)
	pub.Shutdown()
}

func TestNewPublisher_Defaults(t *testing.T) {
	os.Unsetenv("PROJECT_ID")
	os.Unsetenv("TOPIC_ID")
	pub := NewPublisher()
	assert.NotNil(t, pub)
	pub.Shutdown()
}
