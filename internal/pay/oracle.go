package pay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/rs/zerolog/log"
)

var (
	JupiterAPIURL = "https://api.jup.ag/price/v2?ids=SOL"
)

const (
	RedisPriceKey     = "solana_cutoff_price"
	HardFallbackPrice = 91.2
)

type JupiterPriceResponse struct {
	Data map[string]struct {
		Price string `json:"price"`
	} `json:"data"`
}

// GetSolPriceUSD retrieves the current SOL/USD price.
func (s *PayService) GetSolPriceUSD(ctx context.Context) float64 {
	// 1. Try Jupiter API
	if price := s.fetchJupiterPrice(ctx); price > 0 {
		return price
	}

	// 2. Fallback to Binance API
	if price := s.fetchBinancePrice(ctx); price > 0 {
		return price
	}

	// 3. Fallback to Redis
	if price := s.getRedisPrice(ctx); price > 0 {
		return price
	}

	// 4. Hard Fallback
	log.Warn().Float64("fallback", HardFallbackPrice).Msg("Using Hard Fallback Price ($91.2)")
	sentry.CaptureMessage(fmt.Sprintf("Using Hard Fallback Price: %f", HardFallbackPrice))
	return HardFallbackPrice
}

func (s *PayService) fetchJupiterPrice(ctx context.Context) float64 {
	resp, err := httpClient.Get(JupiterAPIURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return 0
	}
	defer resp.Body.Close()

	var jupResp JupiterPriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&jupResp); err != nil {
		return 0
	}

	if solData, ok := jupResp.Data["SOL"]; ok {
		if price, err := strconv.ParseFloat(solData.Price, 64); err == nil && price > 0 {
			s.updateRedisPrice(ctx, solData.Price)
			return price
		}
	}
	return 0
}

func (s *PayService) fetchBinancePrice(ctx context.Context) float64 {
	binanceURL := "https://api.binance.com/api/v3/ticker/price?symbol=SOLUSDC"
	resp, err := http.Get(binanceURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return 0
	}
	defer resp.Body.Close()

	var binResp struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&binResp); err != nil {
		return 0
	}

	if price, err := strconv.ParseFloat(binResp.Price, 64); err == nil && price > 0 {
		log.Info().Float64("price", price).Msg("Using Binance Fallback Price")
		s.updateRedisPrice(ctx, binResp.Price)
		return price
	}
	return 0
}

func (s *PayService) getRedisPrice(ctx context.Context) float64 {
	if s.Redis == nil {
		return 0
	}
	cutoff, err := s.Redis.Get(ctx, RedisPriceKey).Result()
	if err != nil {
		return 0
	}
	if price, err := strconv.ParseFloat(cutoff, 64); err == nil && price > 0 {
		log.Info().Float64("price", price).Msg("Using Redis Cut-off Price")
		return price
	}
	return 0
}

func (s *PayService) updateRedisPrice(ctx context.Context, priceStr string) {
	if s.Redis != nil {
		s.Redis.Set(ctx, RedisPriceKey, priceStr, 24*time.Hour)
	}
}

// ConvertUSDToSOL handles the conversion with 4 decimal precision
func ConvertUSDToSOL(usd float64, solPrice float64) string {
	if solPrice <= 0 {
		return "0.01" // Safety default
	}
	sol := usd / solPrice
	return fmt.Sprintf("%.4f", sol)
}
