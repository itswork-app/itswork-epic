package ingestor

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// SetupRouter initializes the Gin engine and creates the routes.
func SetupRouter() *gin.Engine {
	// Use ReleaseMode for production standards
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// Middleware for recovery from panics
	r.Use(gin.Recovery())

	// Webhook endpoint for Helius
	r.POST("/webhook/helius", HeliusWebhookHandler)

	// Healthcheck endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}

// HeliusWebhookHandler processes incoming POST webhooks from Helius.
func HeliusWebhookHandler(c *gin.Context) {
	var payload interface{}

	// Bind incoming JSON block
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Error().Err(err).Msg("Failed to parse Helius webhook payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON mapping"})
		return
	}

	// Stateless execution: Logs data or ships to message buses
	// No local disk persistence occurs here to fit stateless principles
	log.Info().Interface("payload", payload).Msg("Received Helius Webhook successfully")

	// Respond quickly to satisfy webhook timing constraints
	c.JSON(http.StatusOK, gin.H{"status": "received"})
}
