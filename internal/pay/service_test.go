package pay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestNewPayService_EnvVars(t *testing.T) {
	os.Setenv("SCAN_PRICE_SOL", "0.05")
	os.Setenv("BUNDLE_50_PRICE_SOL", "0.5")
	os.Setenv("BUNDLE_100_PRICE_SOL", "0.9")
	os.Setenv("SUB_PRO_PRICE_SOL", "0.3")
	os.Setenv("PROJECT_WALLET_ADDRESS", "W1")

	s := NewPayService(nil, nil, nil)
	assert.Equal(t, "0.05", s.ScanPrice)
	assert.Equal(t, "0.5", s.Bundle50Price)
	assert.Equal(t, "0.9", s.Bundle100Price)
	assert.Equal(t, "0.3", s.SubProPrice)
	assert.Equal(t, "W1", s.ProjectWallet)
}

func TestGeneratePaymentURL(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	os.Setenv("SCAN_PRICE_SOL", "0.1")

	s := NewPayService(nil, nil, nil)
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
	s := NewPayService(nil, nil, nil)
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
	s := NewPayService(nil, nil, nil)
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
	s := NewPayService(nil, nil, nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_NoKey(t *testing.T) {
	s := NewPayService(nil, nil, nil)
	s.HeliusAPIKey = ""
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.Error(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_NetworkError(t *testing.T) {
	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService(nil, nil, nil)
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
	s := NewPayService(nil, nil, nil)
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
	s := NewPayService(nil, nil, nil)
	s.BaseURL = ts.URL

	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestGeneratePaymentURL_SinglePrice(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	s := NewPayService(nil, nil, nil)

	// Single price should now be $0.50 => ~0.0054 SOL at $91.2 fallback
	url, ref, amount := s.GeneratePaymentURL(context.Background(), "mint123")
	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "amount="+amount)
	assert.Contains(t, url, "amount=0.0055") // $0.50 / $91.2 = 0.0055 SOL
}

func TestGenerateSubscriptionPaymentURL(t *testing.T) {
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	s := NewPayService(nil, nil, nil)

	// Test SUB_MONTHLY_PRO
	url, ref, _ := s.GenerateSubscriptionPaymentURL(context.Background(), "user123", "SUB_MONTHLY_PRO")
	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "amount=0.5373")
	assert.Contains(t, url, "memo=SUBSCRIPTION%3ASUB_MONTHLY_PRO%3Auser123%3A"+ref)

	// Test SUB_WEEKLY_PRO
	url, ref, _ = s.GenerateSubscriptionPaymentURL(context.Background(), "user123", "SUB_WEEKLY_PRO")
	assert.Contains(t, url, "amount=0.1645")
	assert.Contains(t, url, "memo=SUBSCRIPTION%3ASUB_WEEKLY_PRO%3Auser123%3A"+ref)

	// Test SUB_ULTRA_PRO
	url, ref, _ = s.GenerateSubscriptionPaymentURL(context.Background(), "user123", "SUB_ULTRA_PRO")
	assert.Contains(t, url, "amount=2.1820")
	assert.Contains(t, url, "memo=SUBSCRIPTION%3ASUB_ULTRA_PRO%3Auser123%3A"+ref)
}

func TestConvertUSDToSOL_Zero(t *testing.T) {
	amount := ConvertUSDToSOL(10.0, 0)
	assert.Equal(t, "0.01", amount)
}

func TestGetSolPriceUSD_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"SOL": {"price": "150.0"}}}`))
	}))
	defer ts.Close()

	oldURL := JupiterAPIURL
	JupiterAPIURL = ts.URL
	defer func() { JupiterAPIURL = oldURL }()

	s := NewPayService(nil, nil, nil)
	price := s.GetSolPriceUSD(context.Background())
	assert.Equal(t, 150.0, price)
}

func TestGetSolPriceUSD_BadData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"SOL": {"price": "invalid"}}}`))
	}))
	defer ts.Close()

	oldURL := JupiterAPIURL
	JupiterAPIURL = ts.URL
	defer func() { JupiterAPIURL = oldURL }()

	s := NewPayService(nil, nil, nil)
	price := s.GetSolPriceUSD(context.Background())
	assert.Equal(t, HardFallbackPrice, price)
}

func TestGetSolPriceUSD_RedisFallback(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Set price in Redis
	rdb.Set(context.Background(), RedisPriceKey, "123.45", 0)

	// Jupiter fails
	oldURL := JupiterAPIURL
	JupiterAPIURL = "http://localhost:0"
	defer func() { JupiterAPIURL = oldURL }()

	s := NewPayService(rdb, nil, nil)
	price := s.GetSolPriceUSD(context.Background())
	assert.Equal(t, 123.45, price)
}
