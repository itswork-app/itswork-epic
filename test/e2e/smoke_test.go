package e2e

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSmoke_HeliusWebhook simulates a Helius webhook delivery to verify the pipeline is alive.
// Note: This requires the Ingestor to be running on localhost:8080.
func TestSmoke_HeliusWebhook(t *testing.T) {
	// Skip if not in E2E environment or if local server isn't up
	client := &http.Client{Timeout: 2 * time.Second}
	
	// Create Mock Helius Payload
	payload := []byte(`{"mint_address": "E2E_SMOKE_TEST_1", "creator_address": "CREATOR_E2E_1"}`)
	
	url := "http://localhost:8080/webhook/helius"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Skip("Skipping E2E smoke test: Ingestor service not reachable on localhost:8080")
		return
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Helius Ingestion should return 200 OK")
}
