package ingestor

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/repository"
)

func TestHeliusWebhookHandler_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pub := NewPublisher()
	defer pub.Shutdown()

	router := SetupRouter(pub, nil, nil)

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
	router := SetupRouter(nil, nil, nil) // Publisher and Repo not needed for health

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

	router := SetupRouter(pub, nil, nil)

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

	router := SetupRouter(pub, nil, nil)

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

	// Mock PaymentRepository
	payRepo := repository.NewPaymentRepository(db, nil)

	// Step 1: Subscription check (fails)
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Step 2: Credit check (fails)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	// Step 3: Eceran check (success)
	mock.ExpectQuery("SELECT COUNT(.*) FROM payments").
		WithArgs("user123", "mint123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetAnalysis mock
	mock.ExpectQuery(`SELECT verdict, rug_score, reason FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint123").
		WillReturnRows(sqlmock.NewRows([]string{"verdict", "rug_score", "reason"}).AddRow("SAFE", 90, "LP Burned"))

	router := SetupRouter(nil, repo, payRepo)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("X-User-Id", "user123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "SAFE")
	assert.Contains(t, w.Body.String(), "reason")
}

func TestTokenAnalysisHandler_Unpaid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	// All payment checks fail
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	mock.ExpectQuery("SELECT COUNT(.*) FROM payments").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	router := SetupRouter(nil, repo, payRepo)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint123", nil)
	req.Header.Set("X-User-Id", "user123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusPaymentRequired, w.Code)
	assert.Contains(t, w.Body.String(), "Insufficient Credits")
}

func TestTokenAnalysisHandler_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	// Mock Payment Check (Success)
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock GetAnalysis (Fail)
	mock.ExpectQuery(`SELECT verdict, rug_score, reason FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint404").
		WillReturnError(sql.ErrNoRows)

	router := SetupRouter(nil, repo, payRepo)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/token/mint404", nil)
	req.Header.Set("X-User-Id", "user123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTokenAnalysisHandler_MissingMint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Don't set params (mint is empty)
	TokenAnalysisHandler(c, nil, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
