package ingestor

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/pay"
	"itswork.app/internal/repository"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// SetupRouter initializes the Gin engine and creates the routes.
func SetupRouter(pub *Publisher, repo *repository.TokenRepository, payRepo *repository.PaymentRepository) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	// Sentry Middleware (Captures panics and sends to Sentry)
	r.Use(sentrygin.New(sentrygin.Options{
		Repanic: true,
	}))

	// OpenTelemetry Middleware (Distributed Tracing)
	r.Use(otelgin.Middleware("itswork-ingestor"))

	r.POST("/webhook/helius", func(c *gin.Context) {
		HeliusWebhookHandler(c, pub)
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/v1/token/:mint", func(c *gin.Context) {
		TokenAnalysisHandler(c, repo, payRepo)
	})

	payService := pay.NewPayService()
	r.POST("/api/v1/pay/create", func(c *gin.Context) {
		CreatePaymentHandler(c, payService, payRepo)
	})

	r.GET("/api/v1/pay/verify/:reference", func(c *gin.Context) {
		VerifyPaymentHandler(c, payService, payRepo)
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
		// Successfully handed off
		c.JSON(http.StatusOK, gin.H{"status": "enqueued"})
	default:
		// Publisher Channel is backpressuring (full)
		log.Warn().Msg("Publisher channel is full, dropping/delaying webhook")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Ingestion queue at capacity"})
	}
}

// TokenAnalysisHandler processes requests to retrieve the AI verdict of a token.
func TokenAnalysisHandler(c *gin.Context, repo *repository.TokenRepository, payRepo *repository.PaymentRepository) {
	mint := c.Param("mint")
	if mint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing mint parameter"})
		return
	}

	// Access Control: Identify user via Clerk Header (Mocked for now in header X-User-Id)
	userID := c.GetHeader("X-User-Id")
	isPaid := false
	if userID != "" {
		isPaid = payRepo.IsPaid(c.Request.Context(), userID, mint)
	}

	resp, err := repo.GetAnalysis(c.Request.Context(), mint, isPaid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Gated Response Logic
	result := gin.H{
		"mint":    mint,
		"score":   resp.Score,
		"verdict": resp.Verdict,
		"is_paid": isPaid,
	}

	if isPaid {
		result["reason"] = resp.Reason
		// In the future, we can add more heuristic details here
	} else {
		result["reason"] = "Details locked. Please pay to unlock full AI reasoning."
	}

	c.JSON(http.StatusOK, result)
}
