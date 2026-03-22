package ingestor

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
