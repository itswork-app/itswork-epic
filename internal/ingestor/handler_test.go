package ingestor

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/processor"
	"itswork.app/internal/repository"
)

func TestHeliusWebhookHandler_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub := NewPublisher()
	defer pub.Shutdown()
	router := SetupRouter(pub, nil, nil, nil, nil, nil)
	body := []byte(`[]`) // Minimal valid body
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter(nil, nil, nil, nil, nil, nil)
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHeliusWebhookHandler_Backpressure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pub := &Publisher{PublishChan: make(chan []byte, 1)}
	pub.PublishChan <- []byte("initial")
	router := SetupRouter(pub, nil, nil, nil, nil, nil)
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer([]byte(`[]`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestTokenAnalysisHandler_PaidSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	mock.ExpectExec("INSERT INTO user_credits").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT COUNT.*FROM user_subscriptions").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT verdict").WillReturnRows(sqlmock.NewRows([]string{
		"verdict", "rug_score", "reason", "creator_reputation", "insider_risk",
	}).AddRow("SAFE", 90, "good", "TRUSTED", "LOW"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/分析/mint123", nil)
	c.Params = gin.Params{{Key: "mint", Value: "mint123"}}
	c.Set("userID", "user123")
	c.Set("authMethod", "clerk_jwt")

	TokenAnalysisHandler(c, repo, payRepo)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTokenAnalysisHandler_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.NewTokenRepository(db, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	mock.ExpectExec("INSERT INTO user_credits").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT COUNT.*FROM user_subscriptions").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT verdict").WillReturnError(sql.ErrNoRows)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/分析/mint404", nil)
	c.Params = gin.Params{{Key: "mint", Value: "mint404"}}
	c.Set("userID", "user123")
	c.Set("authMethod", "clerk_jwt")

	TokenAnalysisHandler(c, repo, payRepo)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSniperVerdictHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	portalSub := processor.NewPortalSubscriber(nil, nil, nil)
	payRepo := repository.NewPaymentRepository(db, nil)

	mock.ExpectExec("INSERT INTO user_credits").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT COUNT.*FROM user_subscriptions").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "mint", Value: "mint123"}}
	c.Set("userID", "user123")

	SniperVerdictHandler(c, portalSub, payRepo)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMaskKey(t *testing.T) {
	assert.Equal(t, "****", maskKey("abc"))
	assert.Equal(t, "sk_1....cdef", maskKey("sk_1234567890abcdef"))
}

func TestGetUserID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.Equal(t, "", GetUserID(c))
	c.Set("userID", "u1")
	assert.Equal(t, "u1", GetUserID(c))
}

func TestTokenAnalysisHandler_TeaserMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := repository.NewTokenRepository(db, nil)
	mock.ExpectQuery("SELECT verdict").WillReturnRows(sqlmock.NewRows([]string{
		"verdict", "rug_score", "reason", "creator_reputation", "insider_risk",
	}).AddRow("SAFE", 90, "good", "TRUSTED", "LOW"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/?teaser=true", nil)
	c.Params = gin.Params{{Key: "mint", Value: "m1"}}
	c.Set("authMethod", "public")

	TokenAnalysisHandler(c, repo, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTokenAnalysisHandler_MissingMint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	TokenAnalysisHandler(c, nil, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetQuotaHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	payRepo := repository.NewPaymentRepository(db, nil)
	mock.ExpectExec("INSERT INTO user_credits").WillReturnResult(sqlmock.NewResult(1, 1))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", "u1")
	GetQuotaHandler(c, payRepo)
	assert.Equal(t, http.StatusOK, w.Code)
}
