package ingestor

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/clerk/clerk-sdk-go/v2/user"
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
	r.Use(gin.Logger()) // Added for production visibility
	r.Use(gin.Recovery())

	// Sentry Middleware (Captures panics and sends to Sentry)
	r.Use(sentrygin.New(sentrygin.Options{
		Repanic: true,
	}))

	// OpenTelemetry Middleware (Distributed Tracing)
	r.Use(otelgin.Middleware("itswork-ingestor"))

	// CORS Middleware (PR-PRODUCTION-READY)
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowedOrigins := map[string]bool{
			"http://localhost:3000":   true,
			"https://itswork.app":      true,
			"https://www.itswork.app":  true,
		}

		if allowedOrigins[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if origin == "" {
			// Allow non-browser (e.g. curl/postman) or same-origin
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		corsHeaders := "Content-Type, Content-Length, Accept-Encoding, " +
			"X-CSRF-Token, Authorization, accept, origin, " +
			"Cache-Control, X-Requested-With, X-API-KEY"

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", corsHeaders)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

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

		api.POST("/user/role", func(c *gin.Context) {
			SaveUserRoleHandler(c, authRepo)
		})

		api.POST("/auth/sync", func(c *gin.Context) {
			AuthSyncHandler(c, authRepo)
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
			"error":            "Access Denied",
			"reason":           "Sniper API access requires an active subscription and available quota.",
			"suggested_action": "upgrade",
			"upgrade_url":      "https://itswork.app/developer/billing",
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
		"mint":               state.Mint,
		"score":              state.Score,
		"verdict":            state.Verdict,
		"bonding_progress":   state.LastProgress,
		"velocity_rank":      state.VelocityRank,
		"trade_velocity":     state.TradesPerMin,
		"dev_sniped":         state.DevSniped,
		"is_momentum":        state.IsHighMomentum,
		"creator_reputation": state.CreatorReputation,
		"insider_risk":       state.InsiderRisk,
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

	// Teaser Logic (PR-NEXUS-INTELLIGENCE)
	isTeaser := (c.Query("teaser") == "true" || authMethod == "public")

	if isTeaser {
		// Perform the actual work (AI Analysis)
		resp, err := repo.GetAnalysis(c.Request.Context(), mint, true)
		if err != nil {
			log.Warn().Err(err).Str("mint", mint).Msg("Teaser analysis failed")
			if err.Error() == "analysis not found for mint: "+mint {
				c.JSON(http.StatusNotFound, gin.H{"error": "Analysis not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Analysis failed internally"})
			}
			return
		}

		// Teaser Mode: Scrub sensitive intelligence
		c.JSON(http.StatusOK, gin.H{
			"mint":    mint,
			"score":   resp.Score,
			"verdict": resp.Verdict,
			"teaser":  true,
			"message": "Upgrade to unlock creator reputation and holder insights.",
		})
		return
	}

	granted, accessKind, err := payRepo.CheckAccess(c.Request.Context(), userID, mint, isAPI)
	if err != nil {
		log.Error().Err(err).Str("user", userID).Msg("Access check failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal auth error"})
		return
	}

	if !granted {
		// PR-SUBSCRIPTION-PRESTIGE: EMERGENCY BRIDGE
		// If subscription is exhausted, check if user has a successful SINGLE_PAY for this mint.
		if isAPI {
			c.JSON(http.StatusForbidden, gin.H{
				"error":            "Access Denied",
				"reason":           "Developer API access requires an active Pro/Ultra/Enterprise subscription and available quota.",
				"remaining":        0,
				"suggested_action": "upgrade",
				"upgrade_url":      "https://itswork.app/developer/billing",
			})
		} else {
			// Check for Single-Pay Bridge (UI Only)
			var count int
			query := `SELECT COUNT(*) FROM payments WHERE user_id = $1 AND mint_address = $2 AND status = 'success'`
			_ = payRepo.GetDB().QueryRowContext(c.Request.Context(), query, userID, mint).Scan(&count)

			if count > 0 {
				log.Info().Str("user", userID).Str("mint", mint).Msg("Emergency Bridge: Granting access via Single-Pay")
				// Proceed with analysis branch...
			} else {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":            "Insufficient Credits",
					"reason":           "Usage Limit Exceeded. Please upgrade your plan or buy credits to unlock full AI reasoning.",
					"remaining":        0,
					"suggested_action": "buy_credits",
					"topup_url":        "https://itswork.app/billing",
				})
				return
			}
		}
	}

	// Perform the actual work (AI Analysis) - Full Access
	resp, err := repo.GetAnalysis(c.Request.Context(), mint, true)
	if err != nil {
		// ANALYSIS FAILED: Atomic Quota Recovery — we do NOT call CommitUsage
		log.Warn().Err(err).Str("mint", mint).Msg("Analysis failed, quota NOT deducted")

		if err.Error() == "analysis not found for mint: "+mint {
			c.JSON(http.StatusNotFound, gin.H{"error": "Analysis not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Analysis failed internally"})
		}
		return
	}

	// Success: GOAL ACHIEVED — NOW we commit the usage
	payRepo.CommitUsage(c.Request.Context(), userID, accessKind, mint)

	// Gated Response Logic (Full Success)
	c.JSON(http.StatusOK, gin.H{
		"mint":               mint,
		"score":              resp.Score,
		"verdict":            resp.Verdict,
		"reason":             resp.Reason,
		"creator_reputation": resp.CreatorReputation,
		"insider_risk":       resp.InsiderRisk,
		"is_paid":            true,
	})
}

// SaveUserRoleHandler updates the user role in the local database.
func SaveUserRoleHandler(c *gin.Context, authRepo *repository.AuthRepository) {
	var input struct {
		Role string `json:"role" binding:"required,oneof=trader developer"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role selection. Must be 'trader' or 'developer'."})
		return
	}

	userID := GetUserID(c)
	if userID == "" || userID == "guest_teaser" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	if err := authRepo.SaveUserRole(c.Request.Context(), userID, input.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user role"})
		return
	}

	// Sync to Clerk Public Metadata (PR-NEXUS-AUTH-JOURNEY)
	go func() {
		// Use a background context as this is an async sync
		ctx := context.Background()
		metadata := map[string]interface{}{
			"role":                 input.Role,
			"onboarding_completed": true,
		}
		rawMetadata, _ := json.Marshal(metadata)
		clerkRaw := json.RawMessage(rawMetadata)

		_, err := user.Update(ctx, userID, &user.UpdateParams{
			PublicMetadata: &clerkRaw,
		})
		if err != nil {
			log.Error().Err(err).Str("user", userID).Msg("Failed to sync role to Clerk Metadata")
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "success", "role": input.Role})
}

// AuthSyncHandler ensures the user exists in our local DB (PR-WIRE-REPAIR).
func AuthSyncHandler(c *gin.Context, authRepo *repository.AuthRepository) {
	userID := GetUserID(c)
	if userID == "" || userID == "guest_teaser" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	if err := authRepo.SyncUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "User Synced Successfully"})
}
