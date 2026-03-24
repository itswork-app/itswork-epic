package ingestor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/repository"
)

func DualAuthMiddleware(authRepo *repository.AuthRepository, payRepo *repository.PaymentRepository) gin.HandlerFunc {
	secretKey := os.Getenv("CLERK_SECRET_KEY")
	if secretKey == "" {
		log.Warn().Msg("CLERK_SECRET_KEY not set")
	}
	clerk.SetKey(secretKey)

	return func(c *gin.Context) {
		// 1. Check for X-API-KEY (Bot Sniper Gateway)
		apiKey := c.GetHeader("X-API-KEY")
		if apiKey != "" {
			// Hash the key (SHA256 for industrial security)
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(apiKey)))

			userID, err := authRepo.GetUserIDByAPIKey(c.Request.Context(), hash)
			if err != nil {
				log.Error().Err(err).Str("key", maskKey(apiKey)).Msg("API Key validation error")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal auth error"})
				c.Abort()
				return
			}

			if userID != "" {
				c.Set("userID", userID)
				c.Set("authMethod", "api_key")

				// Inject Quota Header for Bot developers
				remaining, _ := payRepo.GetQuotaRemaining(c.Request.Context(), userID)
				c.Header("X-Quota-Remaining", fmt.Sprintf("%d", remaining))

				log.Debug().Str("userID", userID).Msg("Bot authenticated via API Key")
				c.Next()
				return
			}

			// Invalid API Key
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API Key"})
			c.Abort()
			return
		}

		// 2. Fallback to Authorization: Bearer <JWT> (Dashboard)
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required (X-API-KEY or Bearer token)"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization format"})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := jwt.Verify(c.Request.Context(), &jwt.VerifyParams{
			Token: token,
		})

		if err != nil {
			log.Error().Err(err).Msg("Clerk JWT verification failed")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		userID := claims.Subject
		c.Set("userID", userID)
		c.Set("authMethod", "clerk_jwt")
		log.Debug().Str("userID", userID).Msg("User authenticated via Clerk")
		c.Next()
	}
}

// GetUserID is a helper to extract the authenticated user ID from the context
func GetUserID(c *gin.Context) string {
	val, exists := c.Get("userID")
	if !exists {
		return ""
	}
	userID, ok := val.(string)
	if !ok {
		return ""
	}
	return userID
}

// FetchClerkUser is a helper if we ever need more than the ID
func FetchClerkUser(ctx context.Context, userID string) (*clerk.User, error) {
	return user.Get(ctx, userID)
}

// maskKey (Audit PR-FIX-V1) protects sensitive keys in logs
func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "...." + key[len(key)-4:]
}
