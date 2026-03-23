package pay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratePaymentURL(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	os.Setenv("SCAN_PRICE_SOL", "0.1")

	s := NewPayService(nil)
	url, ref, amount := s.GeneratePaymentURL(context.Background(), "mint123")

	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "solana:7nEByo6E1")
	assert.Contains(t, url, "amount="+amount)
	assert.Contains(t, url, "reference="+ref)
}

func TestVerifyTransaction_MockParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)

		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{
				"result": [
					{"signature": "sig123", "err": null}
				],
				"error": null
			}`))
		} else if method == "getTransaction" {
			_, _ = w.Write([]byte(`{
				"result": {
					"meta": {
						"err": null
					}
				},
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.True(t, success)
}
func TestVerifyTransaction_NoSignature(t *testing.T) {
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
		}
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_FailedTx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{
				"result": [
					{"signature": "sig123", "err": null}
				],
				"error": null
			}`))
		} else if method == "getTransaction" {
			_, _ = w.Write([]byte(`{
				"result": {
					"meta": {
						"err": "InstructionError"
					}
				},
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_NoKey(t *testing.T) {
	s := NewPayService(nil)
	s.HeliusAPIKey = ""
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.Error(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_NetworkError(t *testing.T) {
	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil)
	s.BaseURL = "http://localhost:0" // invalid port logic prevents dial

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err) // service handles it gracefully by returning false, nil
	assert.False(t, success)
}

func TestVerifyTransaction_BadJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{bad json`))
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_TxNotFinalized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{"result": [{"signature": "sig123", "err": null}], "error": null}`))
		} else if method == "getTransaction" {
			// Returns null result for unfinalized
			_, _ = w.Write([]byte(`{"result": null, "error": null}`))
		}
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestGenerateBundlePaymentURL(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	s := NewPayService(nil)

	// Test BUNDLE_50
	url, ref, _ := s.GenerateBundlePaymentURL(context.Background(), "user123", "BUNDLE_50")
	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "amount=0.3838")
	assert.Contains(t, url, "memo=BUNDLE%3ABUNDLE_50%3Auser123%3A"+ref)

	// Test BUNDLE_100
	url, ref, _ = s.GenerateBundlePaymentURL(context.Background(), "user123", "BUNDLE_100")
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "amount=0.6579")
	assert.Contains(t, url, "memo=BUNDLE%3ABUNDLE_100%3Auser123%3A"+ref)
}

func TestGenerateSubscriptionPaymentURL(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	s := NewPayService(nil)

	// Test SUB_MONTHLY_PRO
	url, ref, _ := s.GenerateSubscriptionPaymentURL(context.Background(), "user123", "SUB_MONTHLY_PRO")
	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "amount=0.5373")
	assert.Contains(t, url, "memo=SUBSCRIPTION%3ASUB_MONTHLY_PRO%3Auser123%3A"+ref)
}
