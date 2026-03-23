package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type Enricher struct {
	HeliusAPIKey string
	BaseURL      string
	client       *http.Client
}

func NewEnricher(apiKey string) *Enricher {
	return &Enricher{
		HeliusAPIKey: apiKey,
		BaseURL:      "https://mainnet.helius-rpc.com",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (e *Enricher) Enrich(ctx context.Context, payload *HeliusPayload) error {
	// Make sure we have some reasonable heuristics passed rather than 0
	if payload.Top10HolderConcentrationPercent == 0 {
		payload.Top10HolderConcentrationPercent = 50.0 // Default proxy if webhook omits it
	}

	if e.HeliusAPIKey == "" {
		log.Warn().Msg("HELIUS_API_KEY missing, skipping real enrichment (using fallback data)")
		return nil
	}

	rpcURL := fmt.Sprintf("%s/?api-key=%s", e.BaseURL, e.HeliusAPIKey)

	// Enrich 1: CreatorWalletAgeHours
	age, err := e.fetchWalletAge(ctx, rpcURL, payload.CreatorAddress)
	if err != nil {
		log.Error().Err(err).Str("creator", payload.CreatorAddress).Msg("Failed to enrich wallet age")
	} else {
		payload.CreatorWalletAgeHours = age
	}

	// Enrich 2: IsLpBurned (Using getAsset on Mint as proxy for token safety)
	// Fully resolving LP pair programmatically requires DEX SDKs, using getAsset as a base check.
	lpSafe, err := e.checkLpBurned(ctx, rpcURL, payload.MintAddress)
	if err != nil {
		log.Error().Err(err).Str("mint", payload.MintAddress).Msg("Failed to enrich LP status")
	} else {
		payload.IsLpBurned = lpSafe
	}

	return nil
}

func (e *Enricher) fetchWalletAge(ctx context.Context, rpcURL, address string) (int32, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSignaturesForAddress",
		"params": []interface{}{
			address,
			map[string]interface{}{"limit": 1000}, // Fetch up to 1000 most recent
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var res struct {
		Result []struct {
			BlockTime int64 `json:"blockTime"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}

	if len(res.Result) == 0 {
		return 0, nil // Brand new wallet, no transactions
	}

	// The oldest signature in this page is the last element
	oldest := res.Result[len(res.Result)-1]
	if oldest.BlockTime == 0 {
		return 0, nil
	}

	// Calculate age in hours
	ageSeconds := time.Now().Unix() - oldest.BlockTime
	ageHours := int32(ageSeconds / 3600)

	if ageHours < 0 {
		ageHours = 0
	}

	return ageHours, nil
}

func (e *Enricher) checkLpBurned(ctx context.Context, rpcURL, mint string) (bool, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getAsset",
		"params": map[string]interface{}{
			"id": mint,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var res struct {
		Result struct {
			Ownership struct {
				Frozen   bool `json:"frozen"`
				Delegated bool `json:"delegated"`
			} `json:"ownership"`
			Authorities []map[string]interface{} `json:"authorities"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return false, err
	}

	// As a heuristic via getAsset: if authorities are empty/revoked, it's safer.
	// For LP tokens specifically, it's usually sent to 11111111111111111111111111111111 or dead
	// Let's implement a safe assumption based on missing authorities or frozen states.
	if len(res.Result.Authorities) == 0 {
		return true, nil
	}

	return false, nil
}
