package processor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestNewEnricher(t *testing.T) {
	e := NewEnricher("test-key", nil)
	assert.NotNil(t, e)
	assert.Equal(t, "test-key", e.HeliusAPIKey)
}

func TestEnricher_Enrich_NoKey(t *testing.T) {
	e := NewEnricher("", nil)
	payload := &HeliusPayload{}

	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
	// Should fallback cleanly
	assert.Equal(t, float32(50), payload.Top10HolderConcentrationPercent)
}

func TestEnricher_Enrich_InvalidKey_NetworkError(t *testing.T) {
	e := NewEnricher("invalid", nil)
	payload := &HeliusPayload{
		MintAddress:    "mint123",
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
		_ = json.NewDecoder(r.Body).Decode(&req)

		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{
				"result": [
					{"signature": "sig123", "blockTime": 1700000000}
				],
				"error": null
			}`))
		} else if method == "getTransaction" {
			_, _ = w.Write([]byte(`{
				"result": {
					"transaction": {
						"message": {
							"accountKeys": [
								{"pubkey": "5tzFkiKscXHK5ZXCGbXZxdw7gTjjD1mBwuoFbhUvuAi9", "signer": true}
							]
						}
					}
				},
				"error": null
			}`))
		} else if method == "getAsset" {
			_, _ = w.Write([]byte(`{
				"result": {
					"authorities": [],
					"content": {
						"metadata": {"uri": "https://arweave.net/123"},
						"links": {"twitter": "https://x.com/itswork"}
					}
				},
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	e := NewEnricher("test-key", nil)
	e.BaseURL = ts.URL

	payload := &HeliusPayload{
		MintAddress:    "mint123",
		CreatorAddress: "creator123",
	}

	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
	assert.True(t, payload.CreatorWalletAgeHours > 0)
	assert.True(t, payload.IsLpBurned)
	assert.True(t, payload.IsRenounced)
	assert.True(t, payload.HasSocials)
	assert.Equal(t, float32(50), payload.Top10HolderConcentrationPercent)
}

func TestEnricher_Enrich_NoSignatures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)
		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{
				"result": [],
				"error": null
			}`))
		} else if method == "getAsset" {
			_, _ = w.Write([]byte(`{
				"result": {
					"authorities": [{"address": "123"}]
				},
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	e := NewEnricher("test-key", nil)
	e.BaseURL = ts.URL

	payload := &HeliusPayload{
		MintAddress:    "mint123",
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
		_, _ = w.Write([]byte(`{bad json`))
	}))
	defer ts.Close()

	e := NewEnricher("test-key", nil)
	e.BaseURL = ts.URL

	payload := &HeliusPayload{
		MintAddress:    "mint123",
		CreatorAddress: "creator123",
	}

	err := e.Enrich(context.Background(), payload)
	assert.NoError(t, err)
}
func TestEnricher_CheckCreatorReputation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getAssetsByOwner" {
			params, _ := req["params"].(map[string]interface{})
			owner, _ := params["ownerAddress"].(string)

			if owner == "rugger123" {
				// Serial Rugger: many assets with names (failed projects)
				_, _ = w.Write([]byte(`{
					"result": {
						"items": [
							{"id": "d1", "content": {"metadata": {"name": "dead1"}}},
							{"id": "d2", "content": {"metadata": {"name": "dead2"}}},
							{"id": "d3", "content": {"metadata": {"name": "dead3"}}},
							{"id": "d4", "content": {"metadata": {"name": "dead4"}}},
							{"id": "d5", "content": {"metadata": {"name": "dead5"}}},
							{"id": "d6", "content": {"metadata": {"name": "dead6"}}}
						]
					}
				}`))
			} else {
				// Safe: few or no assets
				_, _ = w.Write([]byte(`{
					"result": {"items": []}
				}`))
			}
		}
	}))
	defer ts.Close()

	e := NewEnricher("test-key", nil)
	e.BaseURL = ts.URL

	t.Run("SafeCreator", func(t *testing.T) {
		rep, count := e.checkCreatorReputation(context.Background(), ts.URL, "safe123")
		assert.Equal(t, "TRUSTED", rep)
		assert.Equal(t, 0, count)
	})

	t.Run("SerialRugger", func(t *testing.T) {
		rep, count := e.checkCreatorReputation(context.Background(), ts.URL, "rugger123")
		assert.Equal(t, "SERIAL_RUGGER", rep)
		assert.Equal(t, 6, count)
	})
}

func TestEnricher_CheckGoldenWallets(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	e := NewEnricher("test-key", rdb)

	t.Run("FoundGoldens", func(t *testing.T) {
		mint := "goldmint123"
		wallet1 := "elite1"
		wallet2 := "elite2"

		_ = rdb.SAdd(context.Background(), "pump:launch:buyers:"+mint, wallet1, wallet2).Err()
		_ = rdb.Set(context.Background(), "winrate:wallet:"+wallet1, "90.5", 0).Err()
		_ = rdb.Set(context.Background(), "winrate:wallet:"+wallet2, "85.0", 0).Err()

		has, goldens := e.checkGoldenWallets(context.Background(), mint)
		assert.True(t, has)
		assert.Len(t, goldens, 2)
	})

	t.Run("NoBuyers", func(t *testing.T) {
		has, goldens := e.checkGoldenWallets(context.Background(), "empty")
		assert.False(t, has)
		assert.Nil(t, goldens)
	})
}
