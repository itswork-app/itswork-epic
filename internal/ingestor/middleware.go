package ingestor

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func ClerkMiddleware() gin.HandlerFunc {
	secretKey := os.Getenv("CLERK_SECRET_KEY")
	if secretKey == "" {
		log.Warn().Msg("CLERK_SECRET_KEY not set, auth middleware will be permissive for development (DANGER)")
	}
	clerk.SetKey(secretKey)

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// Fallback for development if needed, but the mission is Zero Placeholder
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
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

		// Set UserID in context
		userID := claims.Subject
		c.Set("userID", userID)

		// Optionally fetch full user if needed (Industrial Grade)
		// but claims.Subject should be enough for IsPaid
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
