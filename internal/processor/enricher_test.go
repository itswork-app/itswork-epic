package processor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEnricher(t *testing.T) {
	e := NewEnricher("test-key")
	assert.NotNil(t, e)
	assert.Equal(t, "test-key", e.HeliusAPIKey)
}

func TestEnricher_Enrich_NoKey(t *testing.T) {
	e := NewEnricher("")
	payload := &HeliusPayload{}
	
	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
	// Should fallback cleanly
	assert.Equal(t, float32(50), payload.Top10HolderConcentrationPercent)
}

func TestEnricher_Enrich_InvalidKey_NetworkError(t *testing.T) {
	e := NewEnricher("invalid")
	payload := &HeliusPayload{
		MintAddress: "mint123",
		CreatorAddress: "creator123",
	}

	// This will try to make actual HTTP calls and likely fail or return 401. 
	// We just ensure it doesn't panic and sets fallbacks.
	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err) // Enrich doesn't return network errors to avoid breaking the pipe
	assert.Equal(t, float32(50), payload.Top10HolderConcentrationPercent)
}

func TestEnricher_Enrich_SuccessMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		
		method, _ := req["method"].(string)
		
		if method == "getSignaturesForAddress" {
			w.Write([]byte(`{
				"result": [
					{"blockTime": 1700000000}
				],
				"error": null
			}`))
		} else if method == "getAsset" {
			w.Write([]byte(`{
				"result": {
					"authorities": []
				},
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	e := NewEnricher("test-key")
	e.BaseURL = ts.URL
	
	payload := &HeliusPayload{
		MintAddress: "mint123",
		CreatorAddress: "creator123",
	}

	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
	assert.True(t, payload.CreatorWalletAgeHours > 0)
	assert.True(t, payload.IsLpBurned)
	assert.Equal(t, float32(50), payload.Top10HolderConcentrationPercent)
}

func TestEnricher_Enrich_NoSignatures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)
		if method == "getSignaturesForAddress" {
			w.Write([]byte(`{
				"result": [],
				"error": null
			}`))
		} else if method == "getAsset" {
			w.Write([]byte(`{
				"result": {
					"authorities": [{"address": "123"}]
				},
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	e := NewEnricher("test-key")
	e.BaseURL = ts.URL
	
	payload := &HeliusPayload{
		MintAddress: "mint123",
		CreatorAddress: "creator123",
	}

	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
	assert.Equal(t, int32(0), payload.CreatorWalletAgeHours)
	assert.False(t, payload.IsLpBurned)
}

func TestEnricher_Enrich_BadJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{bad json`))
	}))
	defer ts.Close()

	e := NewEnricher("test-key")
	e.BaseURL = ts.URL
	
	payload := &HeliusPayload{
		MintAddress: "mint123",
		CreatorAddress: "creator123",
	}

	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
}
