package pay

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratePaymentURL(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	os.Setenv("SCAN_PRICE_SOL", "0.1")
	
	s := NewPayService()
	url, ref := s.GeneratePaymentURL("mint123")
	
	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "solana:7nEByo6E1")
	assert.Contains(t, url, "amount=0.1")
	assert.Contains(t, url, "reference="+ref)
}

func TestVerifyTransaction_Mock(t *testing.T) {
	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService()
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.True(t, success)
}
