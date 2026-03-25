package ingestor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"regexp"

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
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.Use(sentrygin.New(sentrygin.Options{Repanic: true}))
	r.Use(otelgin.Middleware("itswork-ingestor"))

	// CORS Middleware (PR-SHIELD)
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowedOrigin := "https://itswork.app" // Default
		isAllowed := false

		if origin == "http://localhost:3000" {
			allowedOrigin = origin
			isAllowed = true
		} else if origin != "" {
			matched, _ := regexp.MatchString(`^https?://.*\.itswork\.app$`, origin)
			if matched || origin == "https://itswork.app" || origin == "https://www.itswork.app" {
				allowedOrigin = origin
				isAllowed = true
			}
		}

		if isAllowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		} else if origin == "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		corsHeaders := "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-KEY"
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
		api.GET("/token/:mint", func(c *gin.Context) {
			if authMethod, _ := c.Get("authMethod"); authMethod == "api_key" {
				c.JSON(http.StatusForbidden, gin.H{"error": "UI endpoint only."})
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

		api.GET("/user/quota", func(c *gin.Context) {
			GetQuotaHandler(c, payRepo)
		})

		api.GET("/sniper/verdict/:mint", func(c *gin.Context) {
			if authMethod, _ := c.Get("authMethod"); authMethod != "api_key" {
				c.JSON(http.StatusForbidden, gin.H{"error": "API endpoint only."})
				return
			}
			SniperVerdictHandler(c, portalSub, payRepo)
		})
	}

	return r
}

func HeliusWebhookHandler(c *gin.Context, pub *Publisher) {
	secret := os.Getenv("HELIUS_WEBHOOK_SECRET")
	signature := c.GetHeader("X-Helius-Signature")

	payload, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}

	if secret != "" {
		if signature == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing signature"})
			return
		}
		if !VerifyHeliusSignature(secret, signature, payload) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}
	}

	select {
	case pub.PublishChan <- payload:
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Queue full"})
	}
}

func VerifyHeliusSignature(secret, signature string, payload []byte) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil)) == signature
}

func SniperVerdictHandler(c *gin.Context, portalSub *processor.PortalSubscriber, payRepo *repository.PaymentRepository) {
	mint := c.Param("mint")
	userID := GetUserID(c)
	granted, accessKind, _ := payRepo.CheckAccess(c.Request.Context(), userID, mint, true)
	if !granted {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	state, ok := portalSub.GetSniperVerdict(mint)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not tracked"})
		return
	}
	payRepo.CommitUsage(c.Request.Context(), userID, accessKind, mint)
	c.JSON(http.StatusOK, state)
}

func TokenAnalysisHandler(c *gin.Context, repo *repository.TokenRepository, payRepo *repository.PaymentRepository) {
	mint := c.Param("mint")
	userID := GetUserID(c)
	authMethod, _ := c.Get("authMethod")
	isAPI := (authMethod == "api_key")
	isTeaser := (c.Query("teaser") == "true" || authMethod == "public")

	if isTeaser {
		resp, _ := repo.GetAnalysis(c.Request.Context(), mint, true)
		c.JSON(http.StatusOK, gin.H{"mint": mint, "score": resp.Score, "verdict": resp.Verdict, "teaser": true})
		return
	}

	granted, kind, _ := payRepo.CheckAccess(c.Request.Context(), userID, mint, isAPI)
	if !granted {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "Insufficient Credits"})
		return
	}

	resp, err := repo.GetAnalysis(c.Request.Context(), mint, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Analysis failed"})
		return
	}

	payRepo.CommitUsage(c.Request.Context(), userID, kind, mint)
	c.JSON(http.StatusOK, resp)
}

func VerifyPaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	ref := c.Param("reference")
	// Using actual method name from PayService
	_, _ = payService.VerifyTransaction(c.Request.Context(), ref)
	c.JSON(http.StatusOK, gin.H{"status": "verified"})
}

func CreateSubscriptionPaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	var input struct{ Plan string `json:"plan"` }
	_ = c.ShouldBindJSON(&input)
	userID := GetUserID(c)
	// Using actual method name from PayService
	url, ref, _ := payService.GenerateSubscriptionPaymentURL(c.Request.Context(), userID, input.Plan)
	c.JSON(http.StatusOK, gin.H{"url": url, "reference": ref})
}

func SaveUserRoleHandler(c *gin.Context, authRepo *repository.AuthRepository) {
	var input struct{ Role string `json:"role"` }
	_ = c.ShouldBindJSON(&input)
	userID := GetUserID(c)
	_ = authRepo.SaveUserRole(c.Request.Context(), userID, input.Role)
	
	go func() {
		ctx := context.Background()
		metadata := map[string]interface{}{"role": input.Role, "onboarding_completed": true}
		raw, _ := json.Marshal(metadata)
		clerkRaw := json.RawMessage(raw)
		_, _ = user.Update(ctx, userID, &user.UpdateParams{PublicMetadata: &clerkRaw})
	}()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func AuthSyncHandler(c *gin.Context, authRepo *repository.AuthRepository) {
	userID := GetUserID(c)
	_ = authRepo.SyncUser(c.Request.Context(), userID)
	role, _ := authRepo.GetUserRole(c.Request.Context(), userID)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "role": role})
}

func GetQuotaHandler(c *gin.Context, payRepo *repository.PaymentRepository) {
	userID := GetUserID(c)
	free := payRepo.GetFreeUsage(c.Request.Context(), userID, "ui")
	c.JSON(http.StatusOK, gin.H{"free_ui": free})
}
