package ingestor

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHeliusWebhookHandler_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pub := NewPublisher()
	defer pub.Shutdown()

	router := SetupRouter(pub)

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
	router := SetupRouter(nil) // Publisher not needed for health

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

	router := SetupRouter(pub)

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

	router := SetupRouter(pub)

	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", errorReader{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 400 Bad Request due to GetRawData error
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
