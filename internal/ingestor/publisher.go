package ingestor

import (
	"context"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/rs/zerolog/log"
)

type Publisher struct {
	client      *pubsub.Client
	topicPub    *pubsub.Publisher
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

	// Init PubSub Client (Credential ditarik native dari Auth Provider GCP Run)
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create PubSub client")
	}

	var topicPub *pubsub.Publisher
	if client != nil {
		topicPub = client.Publisher(topicID)

		// Concurrency Optimization for Low Latency High Throughput
		topicPub.PublishSettings.DelayThreshold = 50 * time.Millisecond
		topicPub.PublishSettings.CountThreshold = 100
		topicPub.PublishSettings.ByteThreshold = 1e6
	}

	pub := &Publisher{
		client:      client,
		topicPub:    topicPub,
		PublishChan: make(chan []byte, 10000), // Buffered channel prevents HTTP handler blocking
		ctx:         ctx,
		cancel:      cancel,
	}

	pub.StartWorkers(5) // High concurrency setup: 5 dedicated background workers siap tempur

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
			if p.topicPub == nil {
				log.Debug().Msg("PubSub topic nil, message dropped")
				continue
			}

			// Asynchronous Non-blocking Publish ke Google Cloud
			res := p.topicPub.Publish(p.ctx, &pubsub.Message{
				Data: data,
			})

			// Process Result asynchronously so worker isn't blocked by roundtrips
			go func(r *pubsub.PublishResult) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				_, err := r.Get(ctx)
				if err != nil {
					log.Error().Err(err).Msg("Failed to publish message to PubSub - triggering fallback logging")
				}
			}(res)
		}
	}
}

func (p *Publisher) Shutdown() {
	log.Info().Msg("Initiating Publisher graceful shutdown...")
	p.cancel()
	p.wg.Wait()
	if p.topicPub != nil {
		p.topicPub.Stop() // Flushes remaining messages gracefully
	}
	if p.client != nil {
		p.client.Close()
	}
	log.Info().Msg("Publisher shutdown complete")
}
