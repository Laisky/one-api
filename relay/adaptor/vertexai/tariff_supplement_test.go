package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestVertexTariffSupplementMatchesDispatch is a differential test: for every
// Vertex model whose dispatch needs cache prices on top of the token tariff,
// the request-free token check followed by ValidateOperatorTariffSupplement
// (what the channel editor runs before save) must refuse exactly the saved
// configurations ValidateRequestPricing refuses, with the same cache message.
func TestVertexTariffSupplementMatchesDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	at := time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)
	complete := dbmodel.ModelConfigLocal{Ratio: 7, CompletionRatio: 3, CachedInputRatio: .3, CacheWrite5mRatio: 8, CacheWrite1hRatio: 11}
	variants := []struct {
		name   string
		change func(*dbmodel.ModelConfigLocal)
	}{
		{"complete", func(*dbmodel.ModelConfigLocal) {}},
		{"missing_input", func(c *dbmodel.ModelConfigLocal) { c.Ratio = 0 }},
		{"missing_cache_read", func(c *dbmodel.ModelConfigLocal) { c.CachedInputRatio = 0 }},
		{"missing_cache_write", func(c *dbmodel.ModelConfigLocal) { c.CacheWrite5mRatio = 0 }},
		{"missing_long_cache_write", func(c *dbmodel.ModelConfigLocal) { c.CacheWrite1hRatio = 0 }},
		{"no_cache_prices", func(c *dbmodel.ModelConfigLocal) {
			c.CachedInputRatio, c.CacheWrite5mRatio, c.CacheWrite1hRatio = 0, 0, 0
		}},
		{"inherited_tier_cache", func(c *dbmodel.ModelConfigLocal) {
			c.Tiers = []dbmodel.ModelRatioTierLocal{{Ratio: 8, CompletionRatio: 4, InputTokenThreshold: 200000}}
		}},
		{"invalid_tier_cache", func(c *dbmodel.ModelConfigLocal) {
			c.Tiers = []dbmodel.ModelRatioTierLocal{{Ratio: 8, CompletionRatio: 4, CachedInputRatio: -1, InputTokenThreshold: 200000}}
		}},
	}
	a := &Adaptor{}
	var supplementRefusals int
	for _, name := range []string{argonCyberModel, "claude-fable-5-2", "gemini-2.5-flash"} {
		for _, variant := range variants {
			t.Run(name+"/"+variant.name, func(t *testing.T) {
				cfg := complete
				variant.change(&cfg)
				configs := map[string]dbmodel.ModelConfigLocal{name: cfg}
				raw, err := json.Marshal(configs)
				require.NoError(t, err)
				saved := string(raw)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				c.Set(ctxkey.ChannelModel, &dbmodel.Channel{ModelConfigs: &saved})
				dispatchErr := a.ValidateRequestPricing(c, &meta.Meta{ActualModelName: name, StartTime: at})

				var draftErr error
				if adaptor.RequiresOperatorTariff(a, name) {
					draftErr = pricing.ValidateExplicitTokenTariffConfigs(name, configs, at)
					if draftErr == nil {
						draftErr = a.ValidateOperatorTariffSupplement(name, configs, at)
						if draftErr != nil {
							supplementRefusals++
							require.Equal(t, dispatchErr.Error(), draftErr.Error(), "the editor must quote dispatch's cache refusal")
						}
					}
				}
				require.Equal(t, dispatchErr != nil, draftErr != nil, "dispatch=%v draft=%v", dispatchErr, draftErr)
				if draftErr != nil {
					require.True(t, adaptor.IsRequestPricingNotConfigured(draftErr))
					require.True(t, adaptor.IsRequestPricingNotConfigured(dispatchErr))
				}
			})
		}
	}
	require.Positive(t, supplementRefusals, "control: some configurations must fail only the cache supplement")
	require.NoError(t, a.ValidateOperatorTariffSupplement("gemini-2.5-flash", nil, at), "models without a supplement are not refused")
}
