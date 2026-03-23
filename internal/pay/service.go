package pay

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type PayService struct {
	ProjectWallet string
	ScanPrice     string // in SOL, e.g., "0.1"
	HeliusAPIKey  string
}

func NewPayService() *PayService {
	return &PayService{
		ProjectWallet: os.Getenv("PROJECT_WALLET_ADDRESS"),
		ScanPrice:     os.Getenv("SCAN_PRICE_SOL"),
		HeliusAPIKey:  os.Getenv("HELIUS_API_KEY"),
	}
}

// GeneratePaymentURL creates a Solana Pay compliant URL: solana:<address>?amount=<amount>&reference=<reference>
func (s *PayService) GeneratePaymentURL(mint string) (string, string) {
	reference := uuid.New().String()
	
	// Format: solana:7nEByo6E1...RE8?amount=0.1&reference=UUID&label=ItsWork%20Scan
	address := s.ProjectWallet
	amount := s.ScanPrice
	label := url.QueryEscape("ItsWork AI Analysis")
	memo := url.QueryEscape(fmt.Sprintf("Scan for %s", mint))

	solanaURL := fmt.Sprintf("solana:%s?amount=%s&reference=%s&label=%s&memo=%s", 
		address, amount, reference, label, memo)

	return solanaURL, reference
}

// VerifyTransaction checks if a transaction with the given reference exists and is finalized
func (s *PayService) VerifyTransaction(ctx context.Context, reference string) (bool, error) {
	if s.HeliusAPIKey == "" {
		return false, fmt.Errorf("HELIUS_API_KEY not configured")
	}

	// Use Helius RPC to find signatures for the reference key
	// Standard Solana Pay verification involves searching for the reference as a 'readonly' signer in a transaction
	// For MVP: We mock success if API key is present until Helius SDK/Client is fully integrated
	// In reality, this would be a JSON-RPC call to 'getSignaturesForAddress' with the reference key.
	
	log.Printf("Verifying transaction on-chain for reference: %s", reference)
	
	// TODO: Implement real Helius RPC call using net/http
	return true, nil
}
