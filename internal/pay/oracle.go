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

const (
	JupiterAPIURL      = "https://api.jup.ag/price/v2?ids=SOL"
	RedisPriceKey      = "solana_cutoff_price"
	HardFallbackPrice  = 91.2
)

type JupiterPriceResponse struct {
	Data map[string]struct {
		Price string `json:"price"`
	} `json:"data"`
}

// GetSolPriceUSD retrieves the current SOL/USD price.
// 1. Tries Jupiter API.
// 2. Fallbacks to Redis cutoff (24h).
// 3. Final fallback to $91.2.
func (s *PayService) GetSolPriceUSD(ctx context.Context) float64 {
	// 1. Try Jupiter API
	resp, err := httpClient.Get(JupiterAPIURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		var jupResp JupiterPriceResponse
		if err := json.NewDecoder(resp.Body).Decode(&jupResp); err == nil {
			if solData, ok := jupResp.Data["SOL"]; ok {
				price, err := strconv.ParseFloat(solData.Price, 64)
				if err == nil && price > 0 {
					// Update Redis cutoff
					if s.Redis != nil {
						s.Redis.Set(ctx, RedisPriceKey, solData.Price, 24*time.Hour)
					}
					resp.Body.Close()
					return price
				}
			}
		}
		resp.Body.Close()
	}

	// 2. Fallback to Redis
	if err != nil || (resp != nil && resp.StatusCode != http.StatusOK) {
		log.Error().Err(err).Msg("Jupiter API failed, falling back to Redis cutoff")
		sentry.CaptureException(fmt.Errorf("Jupiter API Failure: %v", err))
	}

	if s.Redis != nil {
		cutoff, err := s.Redis.Get(ctx, RedisPriceKey).Result()
		if err == nil {
			price, err := strconv.ParseFloat(cutoff, 64)
			if err == nil && price > 0 {
				log.Info().Float64("price", price).Msg("Using Redis Cut-off Price")
				return price
			}
		}
	}

	// 3. Hard Fallback
	log.Warn().Float64("fallback", HardFallbackPrice).Msg("Using Hard Fallback Price ($91.2)")
	sentry.CaptureMessage(fmt.Sprintf("Using Hard Fallback Price: %f", HardFallbackPrice))
	return HardFallbackPrice
}

// ConvertUSDToSOL handles the conversion with 4 decimal precision
func ConvertUSDToSOL(usd float64, solPrice float64) string {
	if solPrice <= 0 {
		return "0.01" // Safety default
	}
	sol := usd / solPrice
	return fmt.Sprintf("%.4f", sol)
}
