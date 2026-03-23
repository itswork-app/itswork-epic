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

	s := NewPayService()
	url, ref := s.GeneratePaymentURL("mint123")

	assert.NotEmpty(t, url)
	assert.NotEmpty(t, ref)
	assert.Contains(t, url, "solana:7nEByo6E1")
	assert.Contains(t, url, "amount=0.1")
	assert.Contains(t, url, "reference="+ref)
}

func TestVerifyTransaction_MockParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		
		method, _ := req["method"].(string)
		
		if method == "getSignaturesForAddress" {
			w.Write([]byte(`{
				"result": [
					{"signature": "sig123", "err": null}
				],
				"error": null
			}`))
		} else if method == "getTransaction" {
			w.Write([]byte(`{
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
	s := NewPayService()
	s.BaseURL = ts.URL
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.True(t, success)
}
func TestVerifyTransaction_NoSignature(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)
		
		if method == "getSignaturesForAddress" {
			w.Write([]byte(`{
				"result": [],
				"error": null
			}`))
		}
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService()
	s.BaseURL = ts.URL
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_FailedTx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)
		
		if method == "getSignaturesForAddress" {
			w.Write([]byte(`{
				"result": [
					{"signature": "sig123", "err": null}
				],
				"error": null
			}`))
		} else if method == "getTransaction" {
			w.Write([]byte(`{
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
	s := NewPayService()
	s.BaseURL = ts.URL
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_NoKey(t *testing.T) {
	s := NewPayService()
	s.HeliusAPIKey = ""
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.Error(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_NetworkError(t *testing.T) {
	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService()
	s.BaseURL = "http://localhost:0" // invalid port logic prevents dial
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err) // service handles it gracefully by returning false, nil
	assert.False(t, success)
}

func TestVerifyTransaction_BadJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{bad json`))
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService()
	s.BaseURL = ts.URL
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}

func TestVerifyTransaction_TxNotFinalized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)
		
		if method == "getSignaturesForAddress" {
			w.Write([]byte(`{"result": [{"signature": "sig123", "err": null}], "error": null}`))
		} else if method == "getTransaction" {
			// Returns null result for unfinalized
			w.Write([]byte(`{"result": null, "error": null}`))
		}
	}))
	defer ts.Close()

	os.Setenv("HELIUS_API_KEY", "test-key")
	s := NewPayService()
	s.BaseURL = ts.URL
	
	success, err := s.VerifyTransaction(context.Background(), "ref123")
	assert.NoError(t, err)
	assert.False(t, success)
}
