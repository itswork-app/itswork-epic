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
	resp, err := e.client.Do(req)
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
	// If any authority exists, it's not fully renounced
	return false
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

	resp, err := e.client.Do(req)
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

	resp, err := e.client.Do(req)
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

	txResp, err := e.client.Do(txReq)
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
