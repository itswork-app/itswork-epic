package ingestor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/pay"
	"itswork.app/internal/repository"
)

func TestCreatePaymentHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	os.Setenv("SCAN_PRICE_SOL", "0.1")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	mock.ExpectQuery("INSERT INTO payments").
		WithArgs("user123", "mint123", sqlmock.AnyArg(), "pending", 0.1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("uuid-123"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/pay/create?mint=mint123", nil)
	c.Request.Header.Set("X-User-Id", "user123")

	CreatePaymentHandler(c, payService, payRepo)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp["payment_url"])
	assert.NotEmpty(t, resp["reference"])
}

func TestCreatePaymentHandler_MissingMint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/pay/create", nil)

	CreatePaymentHandler(c, nil, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreatePaymentHandler_NoUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/pay/create?mint=mint123", nil)

	CreatePaymentHandler(c, nil, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyPaymentHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("HELIUS_API_KEY", "test-key")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{"result": [{"signature": "sig123", "err": null}], "error": null}`))
		} else if method == "getTransaction" {
			_, _ = w.Write([]byte(`{"result": {"meta": {"err": null}}, "error": null}`))
		}
	}))
	defer ts.Close()
	payService.BaseURL = ts.URL

	// VerifyTransaction currently returns true, so it will call UpdatePaymentStatus
	mock.ExpectQuery("UPDATE payments").
		WithArgs("success", "ref123").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "mint_address", "amount_sol"}).AddRow("user123", "mint123", 0.1))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/pay/verify/ref123", nil)
	c.Params = []gin.Param{{Key: "reference", Value: "ref123"}}

	VerifyPaymentHandler(c, payService, payRepo)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestVerifyPaymentHandler_MissingRef(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/pay/verify/", nil)

	VerifyPaymentHandler(c, nil, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreatePaymentHandler_DBError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")
	os.Setenv("SCAN_PRICE_SOL", "0.1")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	mock.ExpectQuery("INSERT INTO payments").
		WillReturnError(assert.AnError)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/pay/create?mint=mint123", nil)
	c.Request.Header.Set("X-User-Id", "user123")

	CreatePaymentHandler(c, payService, payRepo)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestVerifyPaymentHandler_DBError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("HELIUS_API_KEY", "test-key")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{"result": [{"signature": "sig123", "err": null}], "error": null}`))
		} else if method == "getTransaction" {
			_, _ = w.Write([]byte(`{"result": {"meta": {"err": null}}, "error": null}`))
		}
	}))
	defer ts.Close()
	payService.BaseURL = ts.URL

	mock.ExpectQuery("UPDATE payments").
		WillReturnError(assert.AnError)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/pay/verify/ref123", nil)
	c.Params = []gin.Param{{Key: "reference", Value: "ref123"}}

	VerifyPaymentHandler(c, payService, payRepo)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestVerifyPaymentHandler_ServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("HELIUS_API_KEY", "") // this will cause PayService.VerifyTransaction to return an error

	payService := pay.NewPayService()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/pay/verify/ref123", nil)
	c.Params = []gin.Param{{Key: "reference", Value: "ref123"}}

	VerifyPaymentHandler(c, payService, nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestVerifyPaymentHandler_Pending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("HELIUS_API_KEY", "test-key")

	payService := pay.NewPayService()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{"result": [], "error": null}`)) // empty signatures returns false, nil
		}
	}))
	defer ts.Close()
	payService.BaseURL = ts.URL

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/pay/verify/ref123", nil)
	c.Params = []gin.Param{{Key: "reference", Value: "ref123"}}

	VerifyPaymentHandler(c, payService, nil)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestVerifyPaymentHandler_UpdateDBError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("HELIUS_API_KEY", "test-key")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)

		if method == "getSignaturesForAddress" {
			_, _ = w.Write([]byte(`{"result": [{"signature": "sig123", "err": null}], "error": null}`))
		} else if method == "getTransaction" {
			_, _ = w.Write([]byte(`{"result": {"meta": {"err": null}}, "error": null}`))
		}
	}))
	defer ts.Close()
	payService.BaseURL = ts.URL

	mock.ExpectQuery("UPDATE payments").
		WithArgs("success", "ref123").
		WillReturnError(assert.AnError)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/pay/verify/ref123", nil)
	c.Params = []gin.Param{{Key: "reference", Value: "ref123"}}

	VerifyPaymentHandler(c, payService, payRepo)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateBundlePaymentHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	// Expect SavePayment
	mock.ExpectQuery("INSERT INTO payments").
		WithArgs("user123", "BUNDLE_50", sqlmock.AnyArg(), "pending", 0.4).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("uuid-bundle"))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/pay/bundle?type=BUNDLE_50", nil)
	c.Request.Header.Set("X-User-Id", "user123")

	CreateBundlePaymentHandler(c, payService, payRepo)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp["payment_url"])
	assert.Equal(t, "BUNDLE_50", resp["type"])
}

func TestCreateSubscriptionPaymentHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("PROJECT_WALLET_ADDRESS", "7nEByo6E1RzE1H31RE8RE7RE8RE7RE8RE7RE8RE7RE8")

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	payRepo := repository.NewPaymentRepository(db, nil)
	payService := pay.NewPayService()

	// Expect SavePayment
	mock.ExpectQuery("INSERT INTO payments").
		WithArgs("user123", "SUB_MONTHLY_PRO", sqlmock.AnyArg(), "pending", 0.25).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("uuid-sub"))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/pay/subscribe?plan=SUB_MONTHLY_PRO", nil)
	c.Request.Header.Set("X-User-Id", "user123")

	CreateSubscriptionPaymentHandler(c, payService, payRepo)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp["payment_url"])
	assert.Equal(t, "SUB_MONTHLY_PRO", resp["plan"])
}
