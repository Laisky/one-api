package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
)

// TestClaude52VertexPricingGate prevents unverified defaults from admitting paid calls.
func TestClaude52VertexPricingGate(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(name, func(t *testing.T) {
			a := &Adaptor{}
			if name == "claude-fable-5-2" {
				require.Contains(t, a.GetModelList(), name, "corroborated Vertex catalog control")
			} else {
				require.NotContains(t, a.GetModelList(), name, "uncorroborated Mythos is not suggested")
			}
			require.NotNil(t, GetAdaptor(name), "explicit legacy configuration must retain dispatch")
			_, hasDefault := a.GetDefaultModelPricing()[name]
			require.False(t, hasDefault, "unexpected unverified Vertex default for %s", name)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			response, err := a.DoRequest(c, &meta.Meta{ActualModelName: name}, strings.NewReader(`{"messages":[]}`))
			require.Nil(t, response)
			require.ErrorContains(t, err, "explicit")
		})
	}
}

// TestClaude52VertexFastDispatch rejects premium requests before transport or authentication.
func TestClaude52VertexFastDispatch(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	response, err := (&Adaptor{}).DoRequest(c, &meta.Meta{ActualModelName: "claude-fable-5-2"}, strings.NewReader(`{"speed":"fast","messages":[]}`))
	require.Nil(t, response)
	require.ErrorContains(t, err, "speed")
}

// TestClaude52VertexExplicitTariffs accepts complete custom prices and rejects incomplete tariffs without rewriting data.
func TestClaude52VertexExplicitTariffs(t *testing.T) {
	base := dbmodel.ModelConfigLocal{Ratio: 7, CompletionRatio: 3, CachedInputRatio: .3, CacheWrite5mRatio: 8, CacheWrite1hRatio: 11}
	for _, tc := range []struct {
		name      string
		change    func(*dbmodel.ModelConfigLocal)
		wantError bool
	}{
		{"complete", func(*dbmodel.ModelConfigLocal) {}, false},
		{"missing_input", func(c *dbmodel.ModelConfigLocal) { c.Ratio = 0 }, true},
		{"missing_output", func(c *dbmodel.ModelConfigLocal) { c.CompletionRatio = 0 }, true},
		{"missing_cache_read", func(c *dbmodel.ModelConfigLocal) { c.CachedInputRatio = 0 }, true},
		{"missing_cache_write", func(c *dbmodel.ModelConfigLocal) { c.CacheWrite5mRatio = 0 }, true},
		{"missing_long_cache_write", func(c *dbmodel.ModelConfigLocal) { c.CacheWrite1hRatio = 0 }, true},
		{"negative_cache", func(c *dbmodel.ModelConfigLocal) { c.CachedInputRatio = -1 }, true},
		{"inherited_tier_cache", func(c *dbmodel.ModelConfigLocal) {
			c.Tiers = []dbmodel.ModelRatioTierLocal{{Ratio: 8, CompletionRatio: 4, InputTokenThreshold: 200000}}
		}, false},
		{"invalid_tier_cache", func(c *dbmodel.ModelConfigLocal) {
			c.Tiers = []dbmodel.ModelRatioTierLocal{{Ratio: 8, CompletionRatio: 4, CachedInputRatio: -1, InputTokenThreshold: 200000}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.change(&cfg)
			raw, err := json.Marshal(map[string]dbmodel.ModelConfigLocal{"claude-fable-5-2": cfg})
			require.NoError(t, err)
			saved := string(raw)
			ch := &dbmodel.Channel{ModelConfigs: &saved}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			c.Set(ctxkey.ChannelModel, ch)
			m := &meta.Meta{OriginModelName: "customer-alias", ActualModelName: "claude-fable-5-2", StartTime: time.Now()}
			err = (&Adaptor{}).ValidateRequestPricing(c, m)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, string(raw), *ch.ModelConfigs)
		})
	}
	require.NoError(t, (&Adaptor{}).ValidateRequestPricing(nil, &meta.Meta{ActualModelName: "claude-sonnet-5-5"}))
	require.Contains(t, (&Adaptor{}).GetDefaultModelPricing(), "claude-sonnet-5-5")
}

// TestClaude52VertexPricingNamesAndWindows matches billing's mapped name and request-start tariff selection.
func TestClaude52VertexPricingNamesAndWindows(t *testing.T) {
	cfg := dbmodel.ModelConfigLocal{Ratio: 7, CompletionRatio: 3, CachedInputRatio: .3, CacheWrite5mRatio: 8, CacheWrite1hRatio: 11}
	cfg.TimeWindows = []dbmodel.TimeWindowLocal{{
		Name: "invalid-cache-window", TimeZone: "UTC",
		Ranges:  []dbmodel.ClockRangeLocal{{Start: "12:00", End: "13:00"}},
		Overlay: dbmodel.ModelConfigLocal{CachedInputRatio: -1},
	}}
	raw, err := json.Marshal(map[string]dbmodel.ModelConfigLocal{"claude-fable-5-2": cfg})
	require.NoError(t, err)
	saved := string(raw)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Set(ctxkey.ChannelModel, &dbmodel.Channel{ModelConfigs: &saved})
	a := &Adaptor{}
	m := &meta.Meta{OriginModelName: "claude-fable-5-2", StartTime: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)}
	require.NoError(t, a.ValidateRequestPricing(c, m), "origin fallback must match input/output and cache resolvers")
	m.ActualModelName = "claude-fable-5-2"
	m.OriginModelName = "customer-alias"
	require.NoError(t, a.ValidateRequestPricing(c, m), "actual mapped model owns pricing")
	m.StartTime = time.Date(2026, 11, 10, 12, 30, 0, 0, time.UTC)
	require.ErrorContains(t, a.ValidateRequestPricing(c, m), "cache")
	require.Equal(t, string(raw), saved)
}
