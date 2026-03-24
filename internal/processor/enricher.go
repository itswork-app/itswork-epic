package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

type Enricher struct {
	HeliusAPIKey string
	BaseURL      string
	client       *http.Client
	redis        *redis.Client
}

func NewEnricher(apiKey string, rdb *redis.Client) *Enricher {
	return &Enricher{
		HeliusAPIKey: apiKey,
		BaseURL:      "https://mainnet.helius-rpc.com",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		redis: rdb,
	}
}

// doWithRetry (Audit PR-FIX-V1) implements exponential backoff for Helius RPC 429 errors.
func (e *Enricher) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastResp *http.Response
	var err error

	backoff := 100 * time.Millisecond
	for i := 0; i < 3; i++ {
		lastResp, err = e.client.Do(req)
		if err != nil {
			return nil, err
		}

		if lastResp.StatusCode != http.StatusTooManyRequests {
			return lastResp, nil
		}

		// Close body before retry to prevent leaks
		lastResp.Body.Close()

		log.Warn().
			Int("retry", i+1).
			Dur("wait", backoff).
			Msg("Helius Rate Limit (429) hit, retrying with backoff...")

		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return lastResp, nil
}

// checkGoldenWallets (PR-NEXUS-ELITE Alpha) identifying high win-rate buyers in first 10 slots.
func (e *Enricher) checkGoldenWallets(ctx context.Context, mint string) (bool, []string) {
	if e.redis == nil {
		return false, nil
	}

	// 1. Fetch Top 10 Buyers (Mocked source for Pump.fun launch)
	// In production, this would call a Pump.fun API or scrape the first blocks.
	// For Alpha, we'll check a dedicated Redis set "pump:launch:buyers:<mint>"
	buyers, err := e.redis.SMembers(ctx, fmt.Sprintf("pump:launch:buyers:%s", mint)).Result()
	if err != nil || len(buyers) == 0 {
		return false, nil
	}

	var goldens []string
	for _, wallet := range buyers {
		// 2. Cross-reference win-rate from Redis cache (pre-populated by off-chain worker)
		winRate, err := e.redis.Get(ctx, fmt.Sprintf("winrate:wallet:%s", wallet)).Float64()
		if err == nil && winRate > 75.0 {
			goldens = append(goldens, wallet)
		}
	}

	return len(goldens) > 0, goldens
}

// checkCreatorReputation (PR-NEXUS-INTELLIGENCE) checks for failed past projects using Helius getAssetsByOwner.
func (e *Enricher) checkCreatorReputation(ctx context.Context, rpcURL, creator string) (string, int) {
	// 1. Redis Cache Look-side (6 Hour TTL as per PR-NEXUS-INTELLIGENCE)
	cacheKey := fmt.Sprintf("reputation:creator:%s", creator)
	if e.redis != nil {
		cached, err := e.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result struct {
				Label string `json:"label"`
				Count int    `json:"count"`
			}
			if err := json.Unmarshal([]byte(cached), &result); err == nil {
				return result.Label, result.Count
			}
		}
	}

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getAssetsByOwner",
		"params": map[string]interface{}{
			"ownerAddress": creator,
			"page":         1,
			"limit":        100,
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "UNKNOWN", 0
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.doWithRetry(ctx, req)
	if err != nil {
		return "UNKNOWN", 0
	}
	defer resp.Body.Close()

	var res struct {
		Result struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "UNKNOWN", 0
	}

	failedProjects := 0
	for _, item := range res.Result.Items {
		// Heuristic (PR-NEXUS-INTELLIGENCE): Count assets as indicators of past activity.
		// In a deeper implementation, we would cross-reference liquidity via DexScreener/Helius Price API.
		content, _ := item["content"].(map[string]interface{})
		metadata, _ := content["metadata"].(map[string]interface{})
		if metadata != nil && metadata["name"] != "" {
			failedProjects++
		}
	}

	var label string
	if failedProjects > 5 {
		label = "SERIAL_RUGGER"
	} else if failedProjects > 0 {
		label = "UNKNOWN" // Heuristic: suspicious history but not yet a serial offender
	} else {
		label = "TRUSTED" // Heuristic: clean history
	}

	// 2. Save to Cache (6 Hours)
	if e.redis != nil {
		data, _ := json.Marshal(map[string]interface{}{"label": label, "count": failedProjects})
		e.redis.Set(ctx, cacheKey, data, 6*time.Hour)
	}

	return label, failedProjects
}

// checkInsiderDistribution (PR-NEXUS-REPUTATION) marks tokens where >30% supply went to new wallets in first 5 mins.
func (e *Enricher) checkInsiderDistribution(ctx context.Context, rpcURL, mint string) string {
	// Simplified: Check for early high concentration in non-associated wallets.
	// Production logic would analyze getSignaturesForAddress and map to transfers.
	return "Low" // Placeholder for Alpha
}

func (e *Enricher) Enrich(ctx context.Context, payload *HeliusPayload) error {
	// Make sure we have some reasonable heuristics passed rather than 0
	if payload.Top10HolderConcentrationPercent == 0 {
		payload.Top10HolderConcentrationPercent = 50.0 // Default proxy if webhook omits it
	}

	if e.HeliusAPIKey == "" {
		log.Error().Msg("HELIUS_API_KEY missing - Critical Provider Config Failure")
		return fmt.Errorf("Missing Provider Config: HELIUS_API_KEY is required for real-time enrichment")
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
	// PR-NEXUS-ELITE: Implement Enrichment Cache (5m TTL)
	cacheKey := fmt.Sprintf("enrich:mint:%s", payload.MintAddress)
	if e.redis != nil {
		cached, rerr := e.redis.Get(ctx, cacheKey).Result()
		if rerr == nil {
			var cachedData struct {
				IsLpBurned bool `json:"lp_burned"`
			}
			if json.Unmarshal([]byte(cached), &cachedData) == nil {
				payload.IsLpBurned = cachedData.IsLpBurned
				goto skipLpRPC
			}
		}
	}

	{
		lpSafe, lperr := e.checkLpBurned(ctx, rpcURL, payload.MintAddress)
		if lperr != nil {
			log.Error().Err(lperr).Str("mint", payload.MintAddress).Msg("Failed to enrich LP status")
		} else {
			payload.IsLpBurned = lpSafe
			if e.redis != nil {
				data, _ := json.Marshal(map[string]bool{"lp_burned": lperr == nil && lpSafe})
				e.redis.Set(ctx, cacheKey, string(data), 5*time.Minute)
			}
		}
	}
skipLpRPC:

	// Enrich 3: FundingSourceCheckPassed
	fundingPassed, err := e.checkFunding(ctx, rpcURL, payload.CreatorAddress)
	if err != nil {
		log.Error().Err(err).Str("creator", payload.CreatorAddress).Msg("Failed to enrich funding status")
		payload.FundingSourceCheckPassed = true // Fallback to true if RPC fails to avoid false negatives
	} else {
		payload.FundingSourceCheckPassed = fundingPassed
	}

	// Enrich 4: IsRenounced & HasSocials
	assetData, err := e.fetchAssetData(ctx, rpcURL, payload.MintAddress)
	if err != nil {
		log.Error().Err(err).Str("mint", payload.MintAddress).Msg("Failed to fetch asset data for renouncement/socials")
		payload.IsRenounced = false // Pessimistic fallback
		payload.HasSocials = false
	} else {
		payload.IsRenounced = e.parseRenounced(assetData)
		payload.HasSocials = e.parseSocials(assetData)
	}

	// Enrichment 4: Golden Wallet Alpha Detection
	hasGoldens, goldens := e.checkGoldenWallets(ctx, payload.MintAddress)
	payload.HasGoldenWallets = hasGoldens
	payload.GoldenWallets = goldens

	// Enrichment 5: Creator Reputation (PR-NEXUS-REPUTATION)
	reputation, failedCount := e.checkCreatorReputation(ctx, rpcURL, payload.CreatorAddress)
	payload.CreatorReputation = reputation
	payload.FailedProjectsCount = failedCount

	// Enrichment 6: Insider Risk (PR-NEXUS-REPUTATION)
	payload.InsiderRisk = e.checkInsiderDistribution(ctx, rpcURL, payload.MintAddress)

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

	resp, err := e.doWithRetry(ctx, req)
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

func (e *Enricher) fetchAssetData(ctx context.Context, rpcURL, mint string) (map[string]interface{}, error) {
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
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.doWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Result map[string]interface{} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Result, nil
}

func (e *Enricher) parseRenounced(asset map[string]interface{}) bool {
	authorities, ok := asset["authorities"].([]interface{})
	if !ok || len(authorities) == 0 {
		return true
	}

	for _, a := range authorities {
		authMap, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		scopes, ok := authMap["scopes"].([]interface{})
		if !ok {
			continue
		}
		for _, s := range scopes {
			scopeStr, _ := s.(string)
			// If mint or freeze authority still exists, it's not fully renounced
			if scopeStr == "mint" || scopeStr == "freeze" {
				return false
			}
		}
	}
	return true
}

func (e *Enricher) parseSocials(asset map[string]interface{}) bool {
	content, ok := asset["content"].(map[string]interface{})
	if !ok {
		return false
	}
	metadata, ok := content["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	uri, _ := metadata["uri"].(string)
	if uri == "" {
		return false
	}

	// Heuristic: Check if URI or links contain common social patterns
	// Helius getAsset also sometimes includes 'links'
	links, ok := content["links"].(map[string]interface{})
	if ok && len(links) > 0 {
		return true
	}

	// Basic check on description or other fields if available
	return false
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

	resp, err := e.doWithRetry(ctx, req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var res struct {
		Result struct {
			Ownership struct {
				Frozen    bool `json:"frozen"`
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

// Known Exchanges for Solana
var KnownExchanges = map[string]string{
	"5tzFkiKscXHK5ZXCGbXZxdw7gTjjD1mBwuoFbhUvuAi9": "Binance",
	"GJRs4FwHtemZ5ZE9x3FNvJ8TMwitKTh21yxdRPqn7npE": "Coinbase",
	"is6MTRHEgyFLNTfYcuV4QBWLjrZBfmhVNYR6ccgr8KV":  "OKX",
}

func (e *Enricher) checkFunding(ctx context.Context, rpcURL, address string) (bool, error) {
	// 1. Get transaction signatures back to the start
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSignaturesForAddress",
		"params": []interface{}{
			address,
			map[string]interface{}{"limit": 10}, // We just need the very oldest one in a batch of 10 usually
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.doWithRetry(ctx, req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	var sigRes struct {
		Result []struct {
			Signature string `json:"signature"`
		} `json:"result"`
	}
	err = json.NewDecoder(resp.Body).Decode(&sigRes)
	if err != nil {
		return true, err
	}

	if len(sigRes.Result) == 0 {
		return true, nil // No transactions = no funding yet
	}

	// The last one is the oldest in this set
	oldestSig := sigRes.Result[len(sigRes.Result)-1].Signature

	// 2. Get transaction details to find the sender
	txReqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTransaction",
		"params": []interface{}{
			oldestSig,
			map[string]interface{}{
				"encoding":                       "jsonParsed",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}

	txBodyBytes, _ := json.Marshal(txReqBody)
	txReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(txBodyBytes))
	txReq.Header.Set("Content-Type", "application/json")

	txResp, err := e.doWithRetry(ctx, txReq)
	if err != nil {
		return true, err
	}
	defer txResp.Body.Close()

	var txRes struct {
		Result struct {
			Transaction struct {
				Message struct {
					AccountKeys []struct {
						Pubkey string `json:"pubkey"`
						Signer bool   `json:"signer"`
					} `json:"accountKeys"`
				} `json:"message"`
			} `json:"transaction"`
		} `json:"result"`
	}

	err = json.NewDecoder(txResp.Body).Decode(&txRes)
	if err != nil {
		return true, err
	}

	// The first signer is typically the funder for a SOL transfer
	var firstSigner string
	for _, acc := range txRes.Result.Transaction.Message.AccountKeys {
		if acc.Signer {
			firstSigner = acc.Pubkey
			break
		}
	}

	if firstSigner == "" {
		return true, nil
	}

	// Check if the signer is a known exchange
	if _, ok := KnownExchanges[firstSigner]; ok {
		log.Info().Str("exchange", KnownExchanges[firstSigner]).Str("creator", address).Msg("Creator funded by known exchange")
		return true, nil
	}

	// Common rugger pattern: funded by a fresh personal wallet
	log.Warn().Str("funder", firstSigner).Str("creator", address).Msg("Creator funded by non-exchange wallet (potential serial rugger)")
	return false, nil
}
