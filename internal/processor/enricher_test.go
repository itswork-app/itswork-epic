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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Missing Provider Config")
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

// --- Coverage Boost Tests ---

func TestDoWithRetry(t *testing.T) {
	e := NewEnricher("test-key", nil)

	t.Run("FailAllRetries", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost:0", nil) // Port 0 will refuse connection
		resp, err := e.doWithRetry(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer ts.Close()
		e.BaseURL = ts.URL

		req, _ := http.NewRequest("GET", ts.URL, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		resp, err := e.doWithRetry(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "canceled")
		assert.Nil(t, resp)
	})
}

func TestEnricher_JSONParseErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid-json`))
	}))
	defer ts.Close()

	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	e := NewEnricher("test-key", rdb)
	ctx := context.Background()

	age, err := e.fetchWalletAge(ctx, ts.URL, "creator_json_err")
	assert.Error(t, err)
	assert.Equal(t, int32(0), age)

	// parseRenounced just takes an asset map
	renounced := e.parseRenounced(nil)
	assert.True(t, renounced) // Empty array = true
}

func TestEnricher_ParseRenounced(t *testing.T) {
	e := NewEnricher("test-key", nil)

	t.Run("FullyRenounced", func(t *testing.T) {
		asset := map[string]interface{}{
			"authorities": []interface{}{},
		}
		assert.True(t, e.parseRenounced(asset))
	})

	t.Run("NotRenounced_Mint", func(t *testing.T) {
		asset := map[string]interface{}{
			"authorities": []interface{}{
				map[string]interface{}{
					"scopes": []interface{}{"mint"},
				},
			},
		}
		assert.False(t, e.parseRenounced(asset))
	})

	t.Run("NotRenounced_Freeze", func(t *testing.T) {
		asset := map[string]interface{}{
			"authorities": []interface{}{
				map[string]interface{}{
					"scopes": []interface{}{"freeze"},
				},
			},
		}
		assert.False(t, e.parseRenounced(asset))
	})

	t.Run("BadFormat", func(t *testing.T) {
		asset := map[string]interface{}{
			"authorities": []interface{}{
				"not-a-map",
				map[string]interface{}{
					"scopes": "not-a-slice",
				},
			},
		}
		assert.True(t, e.parseRenounced(asset)) // Skips bad formats, defaults true
	})
}

func TestEnricher_ParseSocials(t *testing.T) {
	e := NewEnricher("test-key", nil)

	t.Run("HasSocials_LinksEmptyURI", func(t *testing.T) {
		asset := map[string]interface{}{
			"content": map[string]interface{}{
				"metadata": map[string]interface{}{
					"uri": "",
				},
				"links": map[string]interface{}{
					"twitter": "https://twitter.com",
				},
			},
		}
		has := e.parseSocials(asset)
		assert.False(t, has) // Fails early because URI is empty
	})

	t.Run("HasSocials_WithURI", func(t *testing.T) {
		asset := map[string]interface{}{
			"content": map[string]interface{}{
				"metadata": map[string]interface{}{
					"uri": "https://example.com/metadata.json",
				},
				"links": map[string]interface{}{
					"twitter": "https://twitter.com",
				},
			},
		}
		has := e.parseSocials(asset)
		assert.True(t, has)
	})

	t.Run("NoSocials", func(t *testing.T) {
		asset := map[string]interface{}{
			"content": map[string]interface{}{
				"metadata": map[string]interface{}{
					"uri": "https://example.com/metadata.json",
				},
			},
		}
		has := e.parseSocials(asset)
		assert.False(t, has)
	})
}

func TestEnricher_CheckLpBurned(t *testing.T) {
	e := NewEnricher("test-key", nil)
	ctx := context.Background()

	t.Run("Burned", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{
				"result": {
					"ownership": {
						"frozen": false,
						"delegated": false
					},
					"authorities": []
				}
			}`))
		}))
		defer ts.Close()
		e.BaseURL = ts.URL

		burned, err := e.checkLpBurned(ctx, ts.URL, "mint1")
		assert.NoError(t, err)
		assert.True(t, burned)
	})

	t.Run("NotBurned", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{
				"result": {
					"ownership": {
						"frozen": true,
						"delegated": true
					},
					"authorities": [{"address":"someauth","scopes":["mint"]}]
				}
			}`))
		}))
		defer ts.Close()
		e.BaseURL = ts.URL

		burned, err := e.checkLpBurned(ctx, ts.URL, "mint2")
		assert.NoError(t, err)
		assert.False(t, burned)
	})

	t.Run("BadResp", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{notarray}`))
		}))
		defer ts.Close()
		e.BaseURL = ts.URL

		burned, err := e.checkLpBurned(ctx, ts.URL, "mint3")
		assert.Error(t, err)
		assert.False(t, burned)
	})
}

func TestEnricher_CheckFunding(t *testing.T) {
	e := NewEnricher("test-key", nil)
	ctx := context.Background()

	t.Run("FundedByExchange", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			method := req["method"].(string)

			if method == "getSignaturesForAddress" {
				_, _ = w.Write([]byte(`{
					"jsonrpc": "2.0",
					"result": [{"signature": "sig123"}],
					"id": 1
				}`))
			} else if method == "getTransaction" {
				_, _ = w.Write([]byte(`{
					"jsonrpc": "2.0",
					"result": {
						"transaction": {
							"message": {
								"accountKeys": [
									{"pubkey": "5tzFkiKscXHK5ZXCGbXZxdw7gTjjD1mBwuoFbhUvuAi9", "signer": true}
								]
							}
						}
					},
					"id": 1
				}`))
			}
		}))
		defer ts.Close()

		funded, err := e.checkFunding(ctx, ts.URL, "creator1")
		assert.NoError(t, err)
		assert.True(t, funded)
	})

	t.Run("NotFundedByExchange", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			method := req["method"].(string)

			if method == "getSignaturesForAddress" {
				_, _ = w.Write([]byte(`{
					"jsonrpc": "2.0",
					"result": [{"signature": "sig456"}],
					"id": 1
				}`))
			} else if method == "getTransaction" {
				_, _ = w.Write([]byte(`{
					"jsonrpc": "2.0",
					"result": {
						"transaction": {
							"message": {
								"accountKeys": [
									{"pubkey": "unknown_wallet", "signer": true}
								]
							}
						}
					},
					"id": 1
				}`))
			}
		}))
		defer ts.Close()

		funded, err := e.checkFunding(ctx, ts.URL, "creator2")
		assert.NoError(t, err)
		assert.False(t, funded)
	})
}
