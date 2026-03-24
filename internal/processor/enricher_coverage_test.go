package processor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"itswork.app/api/proto"
)

// MockBrainger for deterministic testing
type MockBrainger struct{}

func (m *MockBrainger) AnalyzeToken(
	ctx context.Context, mint, creator string, walletAge int32,
	isLpBurned bool, concentration float32, fundingPassed bool,
	isRenounced bool, hasSocials bool,
	bondingProgress, tradeVelocity float32,
	hasGoldens bool, goldens []string,
	reputation string, failedCount int32, insiderRisk string,
) (*proto.VerdictResponse, error) {
	return &proto.VerdictResponse{
		Score:   95,
		Verdict: "SAFE",
	}, nil
}

func TestPortalSubscriber_HandleNewToken_Full_Coverage(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	e := NewEnricher("test", rdb)

	t.Run("Success_WithBrain", func(t *testing.T) {
		ps := NewPortalSubscriber(rdb, &MockBrainger{}, e)
		ps.handleNewToken(PortalMessage{
			Mint: "m1", Trader: "t1", TxType: "create",
		})
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("Failure_NilBrain", func(t *testing.T) {
		ps := NewPortalSubscriber(rdb, nil, e)
		ps.handleNewToken(PortalMessage{
			Mint: "m2", Trader: "t2", TxType: "create",
		})
		time.Sleep(50 * time.Millisecond)
	})
}

func TestPortalSubscriber_HandleTrade_Coverage(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ps := NewPortalSubscriber(rdb, nil, nil)
	mint := "mint1"
	state := &TokenState{Mint: mint, StartTime: time.Now()}
	ps.tokenStats.Store(mint, state)

	ps.handleTrade(PortalMessage{
		Mint: mint, TxType: "buy", VSolInBondingCurve: 10.5,
	})
	assert.Equal(t, 1, state.TradeCount)
}

func TestEnricher_Enrich_CacheHit(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Set HeliusAPIKey to avoid the early return
	e := NewEnricher("real_api_key", rdb)
	// Point BaseURL at a test server that returns errors
	// so the function doesn't hang on real RPC calls
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer ts.Close()
	e.BaseURL = ts.URL

	ctx := context.Background()
	cacheKey := "enrich:mint:m_cached"
	data := map[string]interface{}{"lp_burned": true}
	b, _ := json.Marshal(data)
	rdb.Set(ctx, cacheKey, b, 0)

	payload := &HeliusPayload{
		MintAddress:    "m_cached",
		CreatorAddress: "creator1",
	}
	err := e.Enrich(ctx, payload)
	assert.NoError(t, err)
	assert.True(t, payload.IsLpBurned)
}

func TestEnricher_ComponentErrors(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer ts.Close()

	e := NewEnricher("test", nil)
	e.BaseURL = ts.URL
	ctx := context.Background()

	_, _ = e.fetchWalletAge(ctx, ts.URL, "w")
	_, _ = e.checkLpBurned(ctx, ts.URL, "m")
	_, _ = e.checkFunding(ctx, ts.URL, "f")
	rep, _ := e.checkCreatorReputation(ctx, ts.URL, "u")
	assert.Equal(t, "UNKNOWN", rep)
}
