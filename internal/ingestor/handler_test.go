package ingestor

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHeliusWebhookHandler_ValidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter()

	body := []byte(`{"transaction": "sol123", "type": "transfer", "amount": 100}`)
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code 200, got %v", w.Code)
	}
}

func TestHeliusWebhookHandler_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter()

	// Missing closing brace causes bad JSON parse
	body := []byte(`{"transaction": "sol123", "type": "transfer"`)
	req, _ := http.NewRequest(http.MethodPost, "/webhook/helius", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status code 400, got %v", w.Code)
	}
}
