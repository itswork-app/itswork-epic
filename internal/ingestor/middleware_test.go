package ingestor

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/repository"
)

func TestDualAuthMiddleware_APIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)

	apiKey := "test-api-key"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(apiKey)))

	t.Run("ValidAPIKey", func(t *testing.T) {
		mock.ExpectQuery("SELECT user_id, status FROM api_keys").
			WithArgs(hash).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("user123", "active"))

		mock.ExpectQuery("SELECT quota_limit FROM user_subscriptions").
			WithArgs("user123").
			WillReturnRows(sqlmock.NewRows([]string{"quota_limit"}).AddRow(1000))

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("X-API-KEY", apiKey)

		handler := DualAuthMiddleware(authRepo, payRepo)
		handler(c)

		userID, _ := c.Get("userID")
		assert.Equal(t, "user123", userID)
		authMethod, _ := c.Get("authMethod")
		assert.Equal(t, "api_key", authMethod)
		assert.Equal(t, "1000", w.Header().Get("X-Quota-Remaining"))
	})

	t.Run("InvalidAPIKey", func(t *testing.T) {
		mock.ExpectQuery("SELECT user_id, status FROM api_keys").
			WithArgs(hash).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}))

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("X-API-KEY", apiKey)

		handler := DualAuthMiddleware(authRepo, payRepo)
		handler(c)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.True(t, c.IsAborted())
	})
}

func TestDualAuthMiddleware_NoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)

	handler := DualAuthMiddleware(nil, nil)
	handler(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, c.IsAborted())
}

func TestDualAuthMiddleware_InvalidBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "InvalidFormat")

	handler := DualAuthMiddleware(nil, nil)
	handler(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid Authorization format")
}

func TestDualAuthMiddleware_ClerkJWT_Failure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("CLERK_SECRET_KEY", "test-key")
	
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer invalid-token")

	handler := DualAuthMiddleware(nil, nil)
	handler(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired token")
}
