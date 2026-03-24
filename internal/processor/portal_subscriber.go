package processor

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// PortalSubscriber handles high-velocity WebSocket streaming from Pump Portal.
type PortalSubscriber struct {
	url         string
	redisClient *redis.Client
	brainClient Brainger
	conn        *websocket.Conn

	// Token state tracking
	tokenCreators sync.Map // mint -> creatorAddr
	tokenStats    sync.Map // mint -> *TokenState
}

type TokenState struct {
	Mint           string
	Creator        string
	StartTime      time.Time
	TradeCount     int
	LastProgress   float32
	DevSniped      bool
	TradesPerMin   float32
	FunderAddr     string
	IsHighMomentum bool
}

type PortalMessage struct {
	Method    string `json:"method,omitempty"`
	Signature string `json:"signature"`
	Mint      string `json:"mint"`
	Trader    string `json:"trader"`
	TxType    string `json:"txType"` // "create", "buy", "sell"

	// Bonding Curve Progress
	VSolInBondingCurve    float32 `json:"vSolInBondingCurve,string"`
	VTokensInBondingCurve float32 `json:"vTokensInBondingCurve,string"`

	// Metadata (for 'create' messages)
	Symbol string `json:"symbol"`
	URI    string `json:"uri"`
}

func NewPortalSubscriber(redis *redis.Client, brain Brainger) *PortalSubscriber {
	return &PortalSubscriber{
		url:         "wss://pumpportal.fun/api/data",
		redisClient: redis,
		brainClient: brain,
	}
}

func (s *PortalSubscriber) Start(ctx context.Context) error {
	for {
		err := s.connectAndListen(ctx)
		if err != nil {
			log.Error().Err(err).Msg("PortalSubscriber connection lost, reconnecting in 5s...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
	}
}

func (s *PortalSubscriber) connectAndListen(ctx context.Context) error {
	log.Info().Str("url", s.url).Msg("Connecting to Pump Portal WebSocket...")

	conn, _, err := websocket.DefaultDialer.Dial(s.url, nil)
	if err != nil {
		return err
	}
	s.conn = conn
	defer conn.Close()

	// 1. Subscribe to New Tokens
	if err := conn.WriteJSON(map[string]string{"method": "subscribeNewToken"}); err != nil {
		return err
	}

	// 2. Subscribe to ALL Trades for velocity tracking (High Volume)
	if err := conn.WriteJSON(map[string]string{"method": "subscribeAllTokenTrades"}); err != nil {
		// Fallback to subscribeTokenTrade if all is not available, but usually it is
		log.Warn().Msg("Failed to subscribeAllTokenTrades, attempting fallback...")
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var pm PortalMessage
		if err := json.Unmarshal(message, &pm); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal portal message")
			continue
		}

		go s.HandleMessage(pm)
	}
}

func (s *PortalSubscriber) HandleMessage(pm PortalMessage) {
	if pm.TxType == "create" {
		s.handleNewToken(pm)
		return
	}

	// For buys/sells
	s.handleTrade(pm)
}

func (s *PortalSubscriber) handleNewToken(pm PortalMessage) {
	log.Info().Str("mint", pm.Mint).Str("creator", pm.Trader).Msg("Detik ke-0: New Token Detected via Pump Portal")

	s.tokenCreators.Store(pm.Mint, pm.Trader)

	state := &TokenState{
		Mint:      pm.Mint,
		Creator:   pm.Trader,
		StartTime: time.Now(),
	}
	s.tokenStats.Store(pm.Mint, state)

	// Push to Redis immediately
	s.cacheState(pm.Mint, state)
}

func (s *PortalSubscriber) handleTrade(pm PortalMessage) {
	val, ok := s.tokenStats.Load(pm.Mint)
	if !ok {
		return // Not tracking or too old
	}
	state := val.(*TokenState)

	state.TradeCount++

	// Calculate Bonding Progress (Target 85 SOL usually)
	progress := (pm.VSolInBondingCurve / 85.0) * 100.0
	state.LastProgress = progress

	// Velocity Check: 50% in < 2 mins
	duration := time.Since(state.StartTime)
	if progress >= 50.0 && duration < 2*time.Minute {
		state.IsHighMomentum = true
	}

	// Dev Sniping Check: first 10 trades
	if state.TradeCount <= 10 && pm.Trader == state.Creator {
		state.DevSniped = true
		log.Warn().Str("mint", pm.Mint).Msg("DANGER: Dev Sniping Detected!")
	}

	// Calculate velocity (trades/min)
	if duration.Seconds() > 0 {
		state.TradesPerMin = float32(float64(state.TradeCount) / duration.Minutes())
	}

	// Cache updated state in Redis with TTL 60s
	s.cacheState(pm.Mint, state)
}

func (s *PortalSubscriber) cacheState(mint string, state *TokenState) {
	data, _ := json.Marshal(state)
	err := s.redisClient.Set(context.Background(), "sniper:v1:"+mint, data, 60*time.Second).Err()
	if err != nil {
		log.Error().Err(err).Msg("Failed to cache sniper state to Redis")
	}
}

// GetSniperVerdict is an internal helper for the API handler
func (s *PortalSubscriber) GetSniperVerdict(mint string) (*TokenState, bool) {
	val, ok := s.tokenStats.Load(mint)
	if !ok {
		return nil, false
	}
	return val.(*TokenState), true
}
