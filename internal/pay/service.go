package pay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type PayService struct {
	ProjectWallet  string
	ScanPrice      string // in SOL, e.g., "0.1"
	Bundle50Price  string
	Bundle100Price string
	SubProPrice    string
	HeliusAPIKey   string
	BaseURL        string
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func NewPayService() *PayService {
	scanPrice := os.Getenv("SCAN_PRICE_SOL")
	if scanPrice == "" {
		scanPrice = "0.01" // ~$0.91
	}
	bundle50 := os.Getenv("BUNDLE_50_PRICE_SOL")
	if bundle50 == "" {
		bundle50 = "0.4" // ~$36.40
	}
	bundle100 := os.Getenv("BUNDLE_100_PRICE_SOL")
	if bundle100 == "" {
		bundle100 = "0.7" // ~$63.70
	}
	subPro := os.Getenv("SUB_PRO_PRICE_SOL")
	if subPro == "" {
		subPro = "0.25" // ~$22.75
	}

	return &PayService{
		ProjectWallet:  os.Getenv("PROJECT_WALLET_ADDRESS"),
		ScanPrice:      scanPrice,
		Bundle50Price:  bundle50,
		Bundle100Price: bundle100,
		SubProPrice:    subPro,
		HeliusAPIKey:   os.Getenv("HELIUS_API_KEY"),
		BaseURL:        "https://mainnet.helius-rpc.com",
	}
}

// GeneratePaymentURL creates a Solana Pay compliant URL for single scans
func (s *PayService) GeneratePaymentURL(mint string) (string, string) {
	reference := uuid.New().String()

	address := s.ProjectWallet
	amount := s.ScanPrice
	label := url.QueryEscape("ItsWork AI Analysis")
	memo := url.QueryEscape(fmt.Sprintf("SCAN:%s", mint)) // Prefix for identification

	solanaURL := fmt.Sprintf("solana:%s?amount=%s&reference=%s&label=%s&memo=%s",
		address, amount, reference, label, memo)

	return solanaURL, reference
}

// GenerateBundlePaymentURL creates a URL for purchasing credit bundles
func (s *PayService) GenerateBundlePaymentURL(userID, bundleType string) (string, string) {
	reference := uuid.New().String()
	address := s.ProjectWallet

	var amount string
	var label string
	switch bundleType {
	case "BUNDLE_50":
		amount = s.Bundle50Price
		label = "ItsWork 50 Credits"
	case "BUNDLE_100":
		amount = s.Bundle100Price
		label = "ItsWork 100 Credits"
	default:
		amount = s.ScanPrice
		label = "ItsWork Credits"
	}

	memo := url.QueryEscape(fmt.Sprintf("BUNDLE:%s:%s:%s", bundleType, userID, reference))
	solanaURL := fmt.Sprintf("solana:%s?amount=%s&reference=%s&label=%s&memo=%s",
		address, amount, reference, url.QueryEscape(label), memo)

	return solanaURL, reference
}

// GenerateSubscriptionPaymentURL creates a URL for monthly subscription
func (s *PayService) GenerateSubscriptionPaymentURL(userID, planType string) (string, string) {
	reference := uuid.New().String()
	address := s.ProjectWallet

	var amount string
	var label string
	switch planType {
	case "SUB_MONTHLY_PRO":
		amount = s.SubProPrice
		label = "ItsWork Monthly Pro"
	default:
		amount = s.SubProPrice
		label = "ItsWork Subscription"
	}

	memo := url.QueryEscape(fmt.Sprintf("SUBSCRIPTION:%s:%s:%s", planType, userID, reference))
	solanaURL := fmt.Sprintf("solana:%s?amount=%s&reference=%s&label=%s&memo=%s",
		address, amount, reference, url.QueryEscape(label), memo)

	return solanaURL, reference
}

// VerifyTransaction checks if a transaction with the given reference exists and is finalized
func (s *PayService) VerifyTransaction(ctx context.Context, reference string) (bool, error) {
	if s.HeliusAPIKey == "" {
		return false, fmt.Errorf("HELIUS_API_KEY not configured")
	}

	rpcURL := fmt.Sprintf("%s/?api-key=%s", s.BaseURL, s.HeliusAPIKey)

	// Step 1: getSignaturesForAddress
	sigReqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSignaturesForAddress",
		"params": []interface{}{
			reference,
			map[string]interface{}{"limit": 1},
		},
	}

	sigBytes, _ := json.Marshal(sigReqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(sigBytes))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("reference", reference).Msg("Helius getSignaturesForAddress RPC failed")
		return false, nil // return false, nil to keep pending
	}
	defer resp.Body.Close()

	var sigRes struct {
		Result []struct {
			Signature string      `json:"signature"`
			Err       interface{} `json:"err"`
		} `json:"result"`
	}

	if decodeErr := json.NewDecoder(resp.Body).Decode(&sigRes); decodeErr != nil {
		log.Error().Err(decodeErr).Msg("Failed to decode getSignaturesForAddress response")
		return false, nil
	}

	if len(sigRes.Result) == 0 {
		return false, nil // No transaction found yet
	}

	signature := sigRes.Result[0].Signature

	// Step 2: getTransaction to verify finality and content
	var txRes struct {
		Result *struct {
			Meta struct {
				Err interface{} `json:"err"`
			} `json:"meta"`
		} `json:"result"`
	}

	txReqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTransaction",
		"params": []interface{}{
			signature,
			map[string]interface{}{
				"encoding":                       "jsonParsed",
				"commitment":                     "finalized",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}

	txBytes, _ := json.Marshal(txReqBody)
	reqTx, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewBuffer(txBytes))
	if err != nil {
		return false, err
	}
	reqTx.Header.Set("Content-Type", "application/json")

	respTx, err := httpClient.Do(reqTx)
	if err != nil {
		log.Error().Err(err).Str("signature", signature).Msg("Helius getTransaction RPC failed")
		return false, nil
	}
	defer respTx.Body.Close()

	if decodeErr := json.NewDecoder(respTx.Body).Decode(&txRes); decodeErr != nil {
		log.Error().Err(decodeErr).Msg("Failed to decode getTransaction response")
		return false, nil
	}

	if txRes.Result == nil {
		return false, nil // Transaction not yet finalized
	}

	if txRes.Result.Meta.Err != nil {
		log.Warn().Str("signature", signature).Interface("err", txRes.Result.Meta.Err).Msg("Transaction failed on-chain")
		return false, nil // Failed transaction, do not unlock
	}

	log.Info().Str("reference_key", reference).Str("signature", signature).Msg("Real On-Chain Payment Verified")
	log.Info().Str("reference_key", reference).Str("signature", signature).Msg("Real On-Chain Payment Verified")
	return true, nil
}
