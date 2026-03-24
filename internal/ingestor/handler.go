package ingestor

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/pay"
	"itswork.app/internal/processor"
	"itswork.app/internal/repository"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// SetupRouter initializes the Gin engine and creates the routes.
func SetupRouter(
	pub *Publisher,
	repo *repository.TokenRepository,
	payRepo *repository.PaymentRepository,
	payService *pay.PayService,
	portalSub *processor.PortalSubscriber,
	authRepo *repository.AuthRepository,
) *gin.Engine {
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

	api := r.Group("/api/v1")
	api.Use(DualAuthMiddleware(authRepo, payRepo))
	{
		// MARKETING PORTAL: UI-only endpoint (Clerk JWT)
		api.GET("/token/:mint", func(c *gin.Context) {
			if authMethod, _ := c.Get("authMethod"); authMethod == "api_key" {
				c.JSON(http.StatusForbidden, gin.H{"error": "UI endpoint only. Use /api/v1/sniper/verdict/:mint for bot access."})
				return
			}
			TokenAnalysisHandler(c, repo, payRepo)
		})

		api.GET("/pay/verify/:reference", func(c *gin.Context) {
			VerifyPaymentHandler(c, payService, payRepo)
		})

		api.POST("/pay/subscribe", func(c *gin.Context) {
			CreateSubscriptionPaymentHandler(c, payService, payRepo)
		})

		// DEVELOPER PORTAL: API-only endpoint (X-API-KEY)
		api.GET("/sniper/verdict/:mint", func(c *gin.Context) {
			if authMethod, _ := c.Get("authMethod"); authMethod != "api_key" {
				c.JSON(http.StatusForbidden, gin.H{"error": "API endpoint only. Use X-API-KEY header for bot access."})
				return
			}
			SniperVerdictHandler(c, portalSub, payRepo)
		})
	}

	return r
}

func SniperVerdictHandler(c *gin.Context, portalSub *processor.PortalSubscriber, payRepo *repository.PaymentRepository) {
	mint := c.Param("mint")
	if mint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing mint"})
		return
	}

	// Audit PR-FIX-V1: API Quota Gating
	userID := GetUserID(c)
	granted, accessKind, err := payRepo.CheckAccess(c.Request.Context(), userID, mint, true)
	if err != nil || !granted {
		c.JSON(http.StatusForbidden, gin.H{
			"error":  "Access Denied",
			"reason": "Sniper API access requires an active subscription and available quota.",
		})
		return
	}

	state, ok := portalSub.GetSniperVerdict(mint)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not being tracked or not found in Pump Portal stream"})
		return
	}

	// Success: Deduct Quota
	payRepo.CommitUsage(c.Request.Context(), userID, accessKind, mint)

	// Minimalist High-Speed JSON Output for Bots (Merging fields for maximum utility)
	c.JSON(http.StatusOK, gin.H{
		"mint":             state.Mint,
		"score":            state.Score,
		"verdict":          state.Verdict,
		"bonding_progress": state.LastProgress,
		"velocity_rank":    state.VelocityRank,
		"trade_velocity":   state.TradesPerMin,
		"dev_sniped":       state.DevSniped,
		"is_momentum":      state.IsHighMomentum,
	})
}

// HeliusWebhookHandler processes incoming webhooks immediately passing to channels.
func HeliusWebhookHandler(c *gin.Context, pub *Publisher) {
	// Security: Verify WEBHOOK_SECRET
	secret := os.Getenv("WEBHOOK_SECRET")
	authHeader := c.GetHeader("Authorization")
	apiKeyParam := c.Query("api-key")

	if secret != "" && authHeader != secret && apiKeyParam != secret {
		log.Warn().Str("ip", c.ClientIP()).Msg("Unauthorized webhook attempt")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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

	// Access Control: Identify user and auth method
	userID := GetUserID(c)
	authMethod, _ := c.Get("authMethod")
	isAPI := (authMethod == "api_key")

	granted, accessKind, err := payRepo.CheckAccess(c.Request.Context(), userID, mint, isAPI)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Access check failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal auth error"})
		return
	}

	if !granted {
		if isAPI {
			c.JSON(http.StatusForbidden, gin.H{
				"error":  "Access Denied",
				"reason": "Developer API access requires an active Pro/Ultra/Enterprise subscription and available quota.",
			})
		} else {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":  "Insufficient Credits",
				"reason": "Usage Limit Exceeded. Please upgrade your plan or buy credits to unlock full AI reasoning.",
			})
		}
		return
	}

	// Perform the actual work (AI Analysis)
	resp, err := repo.GetAnalysis(c.Request.Context(), mint, true)
	if err != nil {
		// ANALYSIS FAILED: Atomic Quota Recovery — we do NOT call CommitUsage
		log.Warn().Err(err).Str("mint", mint).Msg("Analysis failed, quota NOT deducted")
		c.JSON(http.StatusNotFound, gin.H{"error": "Analysis failed or not found"})
		return
	}

	// Success: GOAL ACHIEVED — NOW we commit the usage
	payRepo.CommitUsage(c.Request.Context(), userID, accessKind, mint)

	// Gated Response Logic (Succcess)
	c.JSON(http.StatusOK, gin.H{
		"mint":    mint,
		"score":   resp.Score,
		"verdict": resp.Verdict,
		"is_paid": true,
		"reason":  resp.Reason,
	})
}
