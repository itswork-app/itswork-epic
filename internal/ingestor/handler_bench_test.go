package ingestor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"itswork.app/internal/processor"
	"itswork.app/internal/repository"
)

func BenchmarkSniperVerdictHandler(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	portalSub := processor.NewPortalSubscriber(rdb, nil)

	// Pre-populate with a token
	pm := processor.PortalMessage{
		TxType:             "create",
		Mint:               "benchmint",
		Trader:             "benchtrader",
		VSolInBondingCurve: 10.0,
	}
	portalSub.HandleMessage(pm)

	payRepo := repository.NewPaymentRepository(nil, rdb)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		c.Params = []gin.Param{{Key: "mint", Value: "benchmint"}}
		c.Set("userID", "benchuser")
		mr.Set("free:user:benchuser:api", "0")

		SniperVerdictHandler(c, portalSub, payRepo)

		if w.Code != http.StatusOK {
			b.Errorf("expected 200, got %d", w.Code)
		}
	}
}
