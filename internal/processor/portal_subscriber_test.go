package processor

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestPortalSubscriber_HandleMessage(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	brain := &mockBrainger{}
	
	s := NewPortalSubscriber(rdb, brain)

	t.Run("HandleNewToken", func(t *testing.T) {
		pm := PortalMessage{
			TxType: "create",
			Mint:   "mint123",
			Trader: "creator123",
		}
		s.HandleMessage(pm)

		// Check internal state
		val, ok := s.GetSniperVerdict("mint123")
		assert.True(t, ok)
		assert.Equal(t, "creator123", val.Creator)

		// Check Redis
		data, err := rdb.Get(context.Background(), "sniper:v1:mint123").Result()
		assert.NoError(t, err)
		assert.Contains(t, data, "mint123")
	})

	t.Run("HandleTrade_VelocityAndSniping", func(t *testing.T) {
		mint := "mint456"
		creator := "creator456"
		
		// Setup initial state
		state := &TokenState{
			Mint:      mint,
			Creator:   creator,
			StartTime: time.Now().Add(-1 * time.Minute), // 1 min ago
		}
		s.tokenStats.Store(mint, state)
		s.tokenCreators.Store(mint, creator)

		// 1. Normal Trade
		pm := PortalMessage{
			TxType:             "buy",
			Mint:               mint,
			Trader:             "buyer1",
			VSolInBondingCurve: 10.0,
		}
		s.HandleMessage(pm)
		
		val, _ := s.GetSniperVerdict(mint)
		assert.Equal(t, 1, val.TradeCount)
		assert.InDelta(t, 11.76, val.LastProgress, 0.1) // 10/85 * 100
		assert.False(t, val.DevSniped)

		// 2. Dev Sniping Check (Trader is Creator)
		pmSniped := PortalMessage{
			TxType: "buy",
			Mint:   mint,
			Trader: creator,
		}
		s.HandleMessage(pmSniped)
		assert.True(t, val.DevSniped)

		// 3. High Momentum Check (50% in < 2 mins)
		pmMomentum := PortalMessage{
			TxType:             "buy",
			Mint:               mint,
			Trader:             "buyer2",
			VSolInBondingCurve: 45.0, // > 50% of 85
		}
		s.HandleMessage(pmMomentum)
		assert.True(t, val.IsHighMomentum)
	})
}

func TestPortalSubscriber_GetSniperVerdict_NotFound(t *testing.T) {
	s := NewPortalSubscriber(nil, nil)
	_, ok := s.GetSniperVerdict("missing")
	assert.False(t, ok)
}
