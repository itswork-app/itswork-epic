package ingestor

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/processor"
	"itswork.app/internal/repository"
)

func TestHeliusWebhookHandler_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pub := NewPublisher()
	defer pub.Shutdown()

	router := SetupRouter(pub, nil, nil, nil, nil, nil)

	body := []byte(`{"transaction": "sol123", "type": "transfer", "amount": 100}`)
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code 200, got %v", w.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter(nil, nil, nil, nil, nil, nil) // Publisher and Repo not needed for health

	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHeliusWebhookHandler_Backpressure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a publisher with a small/filled channel
	pub := &Publisher{
		PublishChan: make(chan []byte, 1),
	}
	// Fill the channel
	pub.PublishChan <- []byte("initial")

	router := SetupRouter(pub, nil, nil, nil, nil, nil)

	body := []byte(`{"data": "second"}`)
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer(body))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 429 Too Many Requests
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

type errorReader struct{}

func (errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("forced read error")
}

func TestHeliusWebhookHandler_BodyError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub := NewPublisher()
	defer pub.Shutdown()

	router := SetupRouter(pub, nil, nil, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", errorReader{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 400 Bad Request due to GetRawData error
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTokenAnalysisHandler_PaidSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// Free tier: exhausted (used=3 >= limit=3)
	mock.ExpectQuery("SELECT free_scans_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_scans_used"}).AddRow(3))

	// Subscription check (success)
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Quota check
	mock.ExpectQuery("SELECT quota_limit FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"quota_limit"}).AddRow(5000))

	// GetAnalysis mock
	mock.ExpectQuery(`SELECT verdict, rug_score, reason FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint123").
		WillReturnRows(sqlmock.NewRows([]string{"verdict", "rug_score", "reason"}).AddRow("SAFE", 90, "LP Burned"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	c.Params = gin.Params{{Key: "mint", Value: "mint123"}}
	c.Set("userID", "user123")
	c.Set("authMethod", "clerk_jwt")

	TokenAnalysisHandler(c, repo, payRepo)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "SAFE")
}

func TestTokenAnalysisHandler_Unpaid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// Free tier: exhausted
	mock.ExpectQuery("SELECT free_scans_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_scans_used"}).AddRow(3))

	// Subscription check fails
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Credit check fails
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	// Eceran check fails
	mock.ExpectQuery("SELECT COUNT(.*) FROM payments").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	c.Params = gin.Params{{Key: "mint", Value: "mint123"}}
	c.Set("userID", "user123")
	c.Set("authMethod", "clerk_jwt")

	TokenAnalysisHandler(c, repo, payRepo)

	assert.Equal(t, http.StatusPaymentRequired, w.Code)
}

func TestTokenAnalysisHandler_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// Free tier: exhausted
	mock.ExpectQuery("SELECT free_scans_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_scans_used"}).AddRow(3))

	// Subscription check (success)
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Quota check
	mock.ExpectQuery("SELECT quota_limit FROM user_subscriptions").
		WillReturnRows(sqlmock.NewRows([]string{"quota_limit"}).AddRow(5000))

	// GetAnalysis (fail - not found)
	mock.ExpectQuery(`SELECT verdict, rug_score, reason FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint404").
		WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/token/mint404", nil)
	c.Params = gin.Params{{Key: "mint", Value: "mint404"}}
	c.Set("userID", "user123")
	c.Set("authMethod", "clerk_jwt")

	TokenAnalysisHandler(c, repo, payRepo)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTokenAnalysisHandler_MissingMint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	TokenAnalysisHandler(c, nil, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSniperVerdictHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("NotFound", func(t *testing.T) {
		portalSub := processor.NewPortalSubscriber(nil, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "mint", Value: "missing"}}

		SniperVerdictHandler(c, portalSub)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Success", func(t *testing.T) {
		mr, _ := miniredis.Run()
		defer mr.Close()
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

		portalSub := processor.NewPortalSubscriber(rdb, nil)

		// Manually inject state for testing (simulating a create event)
		pm := processor.PortalMessage{
			TxType: "create",
			Mint:   "snipe123",
			Trader: "creator123",
		}

		// We can't call private handleMessage, but we can call Start in a limited way or
		// if we make handleMessage public. Wait, I made it public in my thought but wrote it as private?
		// Let me check portal_subscriber.go.

		// It's public: func (s *PortalSubscriber) HandleMessage(pm PortalMessage)
		portalSub.HandleMessage(pm)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "mint", Value: "snipe123"}}

		SniperVerdictHandler(c, portalSub)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "LOW", resp["velocity_rank"])
	})
}

func TestTokenAnalysisHandler_Bot_SubscriptionRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)

	// 1. API Key Auth Success
	mock.ExpectQuery("SELECT user_id, status FROM api_keys").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("user_bot", "active"))

	// NOTE: IsPaid flow starts: GetFreeUsage (Redis nil → DB)
	// But since route blocks bots at route level, mock is not needed.

	router := SetupRouter(nil, nil, payRepo, nil, nil, authRepo)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("X-API-KEY", "bot-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Bots are blocked at the route level (UI endpoint only)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "UI endpoint only")
}

func TestTokenAnalysisHandler_Bot_QuotaExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)

	// 1. API Key Auth Success
	mock.ExpectQuery("SELECT user_id, status FROM api_keys").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("user_bot", "active"))

	router := SetupRouter(nil, nil, payRepo, nil, nil, authRepo)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("X-API-KEY", "bot-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Bots are blocked at the route level (UI endpoint only)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "UI endpoint only")
}

func TestDualAuthMiddleware_NoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	defer db.Close()
	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)
	router := SetupRouter(nil, nil, payRepo, nil, nil, authRepo)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDualAuthMiddleware_InvalidBearerFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	defer db.Close()
	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)
	router := SetupRouter(nil, nil, payRepo, nil, nil, authRepo)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDualAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	defer db.Close()
	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)
	router := SetupRouter(nil, nil, payRepo, nil, nil, authRepo)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDualAuthMiddleware_InvalidAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	payRepo := repository.NewPaymentRepository(db, nil)
	authRepo := repository.NewAuthRepository(db, nil)
	mock.ExpectQuery("SELECT user_id, status FROM api_keys").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}))
	router := SetupRouter(nil, nil, payRepo, nil, nil, authRepo)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("X-API-KEY", "bad-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
