package e2e

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"itswork.app/internal/ingestor"
)

// TestSmoke_HeliusWebhook simulates a Helius webhook delivery to verify the pipeline is alive.
func TestSmoke_HeliusWebhook(t *testing.T) {
	// 1. Setup Mock Publisher (No credentials needed for mock)
	pub := ingestor.NewPublisher()
	defer pub.Shutdown()

	// 2. Setup Router with mock components
	router := ingestor.SetupRouter(pub, nil, nil, nil, nil, nil)

	// 3. Create Ephemeral Test Server
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 4. Create Mock Helius Payload
	payload := []byte(`{"mint_address": "E2E_SMOKE_TEST_1", "creator_address": "CREATOR_E2E_1"}`)

	url := ts.URL + "/webhook/helius"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 5. Execute Request
	client := ts.Client()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to call test server: %v", err)
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Helius Ingestion should return 200 OK")
}
