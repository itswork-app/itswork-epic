package ingestor

import (
	"context"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/pubsub" //nolint:staticcheck
	"github.com/rs/zerolog/log"
)

type Publisher struct {
	client      *pubsub.Client
	topic       *pubsub.Topic
	PublishChan chan []byte
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewPublisher() *Publisher {
	projectID := os.Getenv("PROJECT_ID")
	topicID := os.Getenv("TOPIC_ID")

	if projectID == "" {
		projectID = "itswork-epic" // default dev fallback
	}
	if topicID == "" {
		topicID = "helius-ingestion-stream"
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create PubSub Client
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create PubSub client (Skipping publish externally)")
	}

	var topic *pubsub.Topic
	if client != nil {
		topic = client.Topic(topicID)

		// Concurrency Optimization for Low Latency
		topic.PublishSettings.DelayThreshold = 50 * time.Millisecond
		topic.PublishSettings.CountThreshold = 100
		topic.PublishSettings.ByteThreshold = 1e6
	}

	pub := &Publisher{
		client:      client,
		topic:       topic,
		PublishChan: make(chan []byte, 10000), // Buffered channel prevents HTTP handler blocking
		ctx:         ctx,
		cancel:      cancel,
	}

	pub.StartWorkers(5) // High concurrency setup: 5 dedicated background workers

	return pub
}

func (p *Publisher) StartWorkers(numWorkers int) {
	for i := 0; i < numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

func (p *Publisher) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case data := <-p.PublishChan:
			if p.topic == nil {
				// Fallback state if PubSub is disabled (e.g., local dev without credentials)
				log.Debug().Msg("PubSub topic nil, message dropped")
				continue
			}

			// Asynchronous Non-blocking Publish
			res := p.topic.Publish(p.ctx, &pubsub.Message{
				Data: data,
			})

			// Process Result asynchronously so worker isn't blocked by roundtrips
			go func(r *pubsub.PublishResult) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				_, err := r.Get(ctx)
				if err != nil {
					log.Error().Err(err).Msg("Failed to publish message to PubSub - retry mechanism could trigger here")
				}
			}(res)
		}
	}
}

func (p *Publisher) Shutdown() {
	log.Info().Msg("Initiating Publisher graceful shutdown...")
	p.cancel()
	p.wg.Wait()
	if p.topic != nil {
		p.topic.Stop() // Flushes remaining messages gracefully
	}
	if p.client != nil {
		p.client.Close()
	}
	log.Info().Msg("Publisher shutdown complete")
}
