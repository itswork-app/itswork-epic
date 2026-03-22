package ingestor

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// SetupRouter initializes the Gin engine and creates the routes.
func SetupRouter(pub *Publisher) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/webhook/helius", func(c *gin.Context) {
		HeliusWebhookHandler(c, pub)
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}

// HeliusWebhookHandler processes incoming webhooks immediately passing to channels.
func HeliusWebhookHandler(c *gin.Context, pub *Publisher) {
	// Directly consume RawData (bytes) to avoid latency of JSON Bindings here
	payload, err := c.GetRawData()
	if err != nil {
		log.Error().Err(err).Msg("Failed to read raw body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body payload"})
		return
	}

	// Stateless logic: passing to asynchronous channel ensures latency < 50ms
	select {
	case pub.PublishChan <- payload:
		// Succesfully handed off
		c.JSON(http.StatusOK, gin.H{"status": "enqueued"})
	default:
		// Publisher Channel is backpressuring (full)
		log.Warn().Msg("Publisher channel is full, dropping/delaying webhook")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Ingestion queue at capacity"})
	}
}
