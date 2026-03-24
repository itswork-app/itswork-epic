package processor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestPortalSubscriber_HandleMessage(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	sub := NewPortalSubscriber(rdb, nil, nil)

	t.Run("NewToken", func(t *testing.T) {
		pm := PortalMessage{
			TxType: "create",
			Mint:   "mint1",
			Trader: "creator1",
			URI:    "metadata_uri",
		}
		sub.HandleMessage(pm)

		// Verify state created
		state, ok := sub.GetSniperVerdict("mint1")
		assert.True(t, ok)
		assert.Equal(t, "mint1", state.Mint)
		assert.Equal(t, "creator1", state.Creator)

		// Verify cached in Redis
		val, err := rdb.Get(context.Background(), "sniper:v1:mint1").Result()
		assert.NoError(t, err)
		assert.NotEmpty(t, val)
	})

	t.Run("Trade", func(t *testing.T) {
		pm := PortalMessage{
			TxType:             "buy",
			Mint:               "mint1",
			Trader:             "trader1",
			VSolInBondingCurve: 42.5, // 50% of 85
		}
		sub.HandleMessage(pm)

		state, _ := sub.GetSniperVerdict("mint1")
		assert.Equal(t, 1, state.TradeCount)
		assert.Equal(t, float32(50), state.LastProgress)
	})

	t.Run("DevSniped", func(t *testing.T) {
		// Reset state
		pmCreate := PortalMessage{
			TxType: "create",
			Mint:   "mint_sniped",
			Trader: "dev1",
		}
		sub.HandleMessage(pmCreate)

		pmTrade := PortalMessage{
			TxType: "buy",
			Mint:   "mint_sniped",
			Trader: "dev1",
		}
		sub.HandleMessage(pmTrade)

		state, _ := sub.GetSniperVerdict("mint_sniped")
		assert.True(t, state.DevSniped)
	})
}

func TestPortalSubscriber_Lifecycle(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	sub := NewPortalSubscriber(rdb, nil, nil)

	t.Run("StartAndStop", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Start will fail connection immediately because url is wrong
		sub.url = "ws://localhost:0"
		err := sub.Start(ctx)
		// It should loop and eventually return ctx.Err()
		assert.Error(t, err)
		assert.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("ConnectAndListen_Fail", func(t *testing.T) {
		sub.url = "http://invalid-url"
		err := sub.connectAndListen(context.Background())
		assert.Error(t, err)
	})

	t.Run("WebSocket_Flow", func(t *testing.T) {
		upgrader := websocket.Upgrader{}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, _ := upgrader.Upgrade(w, r, nil)
			defer conn.Close()

			// Send a mock message
			msg := PortalMessage{
				TxType: "create",
				Mint:   "ws_mint",
				Trader: "ws_creator",
			}
			data, _ := json.Marshal(msg)
			_ = conn.WriteMessage(websocket.TextMessage, data)

			// Wait a bit then close
			time.Sleep(50 * time.Millisecond)
		}))
		defer ts.Close()

		sub.url = "ws" + ts.URL[4:]
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// This will run until context timeout or server close
		err := sub.connectAndListen(ctx)
		if err != nil && err != context.DeadlineExceeded {
			// Some errors are expected on close
			t.Logf("Expected some error on close: %v", err)
		}

		// Verify message was processed
		state, ok := sub.GetSniperVerdict("ws_mint")
		assert.True(t, ok)
		assert.Equal(t, "ws_mint", state.Mint)
	})
}
