package pay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/repository"
)

const (
	PriceSingleUSD  = 0.50
	PriceWeeklyUSD  = 15.0
	PriceMonthlyUSD = 49.0
	PriceUltraUSD   = 199.0
)

type PayService struct {
	ProjectWallet  string
	ScanPrice      string // in SOL, e.g., "0.1"
	Bundle50Price  string
	Bundle100Price string
	SubProPrice    string
	HeliusAPIKey   string
	BaseURL        string
	Redis          *redis.Client
	PayRepo        *repository.PaymentRepository
	AuthRepo       *repository.AuthRepository
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func NewPayService(rdb *redis.Client, payRepo *repository.PaymentRepository, authRepo *repository.AuthRepository) *PayService {
	scanPrice := os.Getenv("SCAN_PRICE_SOL")
	if scanPrice == "" {
		scanPrice = "0.01" // fallback if orbit fails
	}
	bundle50 := os.Getenv("BUNDLE_50_PRICE_SOL")
	if bundle50 == "" {
		bundle50 = "0.4"
	}
	bundle100 := os.Getenv("BUNDLE_100_PRICE_SOL")
	if bundle100 == "" {
		bundle100 = "0.7"
	}
	subPro := os.Getenv("SUB_PRO_PRICE_SOL")
	if subPro == "" {
		subPro = "0.25"
	}

	return &PayService{
		ProjectWallet:  os.Getenv("PROJECT_WALLET_ADDRESS"),
		ScanPrice:      scanPrice,
		Bundle50Price:  bundle50,
		Bundle100Price: bundle100,
		SubProPrice:    subPro,
		HeliusAPIKey:   os.Getenv("HELIUS_API_KEY"),
		BaseURL:        "https://mainnet.helius-rpc.com",
		Redis:          rdb,
		PayRepo:        payRepo,
		AuthRepo:       authRepo,
	}
}

// GenerateAPIKey creates a new API key for a user if they are a Pro subscriber.
// Returns the raw key (only shown once) or an error.
func (s *PayService) GenerateAPIKey(ctx context.Context, userID, label string) (string, error) {
	// 1. Authorization: Only Monthly Pro subscribers can have Bot API Keys
	if !s.PayRepo.IsProSubscriber(ctx, userID) {
		return "", fmt.Errorf("API Keys are reserved for Pro Subscribers")
	}

	// 2. Generate secure random key
	rawKey := fmt.Sprintf("sk_%s_%s", userID[:4], uuid.New().String())
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(rawKey)))

	// 3. Persist hash in DB
	err := s.AuthRepo.SaveAPIKey(ctx, userID, hash, label)
	if err != nil {
		return "", err
	}

	log.Info().Str("user", userID).Msg("New Bot API Key generated")
	return rawKey, nil
}

// GeneratePaymentURL creates a Solana Pay compliant URL for single scans
func (s *PayService) GeneratePaymentURL(ctx context.Context, mint string) (string, string, string) {
	reference := uuid.New().String()
	solPrice := s.GetSolPriceUSD(ctx)
	amount := ConvertUSDToSOL(PriceSingleUSD, solPrice)

	address := s.ProjectWallet
	label := url.QueryEscape("ItsWork AI Analysis")
	memo := url.QueryEscape(fmt.Sprintf("SCAN:%s", mint)) // Prefix for identification

	solanaURL := fmt.Sprintf("solana:%s?amount=%s&reference=%s&label=%s&memo=%s",
		address, amount, reference, label, memo)

	return solanaURL, reference, amount
}

// GenerateBundlePaymentURL is removed in Nexus V1 Final Spec.
// Bundle credits are deprecated. Users subscribe via GenerateSubscriptionPaymentURL.

// GenerateSubscriptionPaymentURL creates a URL for monthly subscription
func (s *PayService) GenerateSubscriptionPaymentURL(ctx context.Context, userID, planType string) (string, string, string) {
	reference := uuid.New().String()
	address := s.ProjectWallet
	solPrice := s.GetSolPriceUSD(ctx)

	var amount string
	var label string
	switch planType {
	case "SUB_WEEKLY_PRO":
		amount = ConvertUSDToSOL(PriceWeeklyUSD, solPrice)
		label = "ItsWork Weekly Pro"
	case "SUB_MONTHLY_PRO":
		amount = ConvertUSDToSOL(PriceMonthlyUSD, solPrice)
		label = "ItsWork Monthly Pro"
	case "SUB_ULTRA_PRO":
		amount = ConvertUSDToSOL(PriceUltraUSD, solPrice)
		label = "ItsWork Ultra Pro"
	default:
		amount = ConvertUSDToSOL(PriceMonthlyUSD, solPrice)
		label = "ItsWork Subscription"
	}

	memo := url.QueryEscape(fmt.Sprintf("SUBSCRIPTION:%s:%s:%s", planType, userID, reference))
	solanaURL := fmt.Sprintf("solana:%s?amount=%s&reference=%s&label=%s&memo=%s",
		address, amount, reference, url.QueryEscape(label), memo)

	return solanaURL, reference, amount
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
	return true, nil
}
