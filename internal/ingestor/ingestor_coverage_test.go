package ingestor

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/repository"
)

func TestHeliusWebhookHandler_JSONUnmarshalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub := NewPublisher()
	defer pub.Shutdown()

	router := SetupRouter(pub, nil, nil, nil, nil, nil)

	body := []byte(`{invalid json}`)
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTokenAnalysisHandler_InvalidAuthMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/api/v1/token/mint1", nil)
	c.Params = gin.Params{{Key: "mint", Value: "mint1"}}
	c.Set("userID", "user1")
	c.Set("authMethod", 12345) // wrong type for authMethod

	db, _, _ := sqlmock.New()
	defer db.Close()
	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	TokenAnalysisHandler(c, repo, payRepo)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
