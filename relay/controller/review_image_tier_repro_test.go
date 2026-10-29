package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestNativeNanoImageTierUsesRenderedDefault checks the charged tier when native chat cannot request a larger render.
func TestNativeNanoImageTierUsesRenderedDefault(t *testing.T) {
	for _, tc := range []struct {
		name, model  string
		channel, api int
		expected     float64
		oneK         float64
	}{
		{"native_gemini", "gemini-nano-banana-2.1", channeltype.Gemini, apitype.Gemini, 1, 1},
		{"native_vertex", "gemini-nano-banana-2.1", channeltype.VertextAI, apitype.VertexAI, 1, 1},
		{"mapped_case_variant", "Gemini-Nano-Banana-2.1", channeltype.Gemini, apitype.Gemini, 1, 1},
		{"native_1k_override", "gemini-nano-banana-2.1", channeltype.Gemini, apitype.Gemini, 0.8, 0.8},
		{"non_gemini", "synthetic-other-image", channeltype.OpenAI, apitype.OpenAI, 1.5, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "2048x2048", DefaultQuality: "standard", SizeMultipliers: map[string]float64{"1024x1024": tc.oneK, "2048x2048": 1.5}}}
			channel := &dbmodel.Channel{}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{tc.model: cfg}))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.ChannelRatio, 1.0)
			c.Set(ctxkey.OutputImageCount, 1)
			c.Set(ctxkey.ChannelModel, channel)
			usage := &model.Usage{PromptTokens: 1}
			m := &meta.Meta{ActualModelName: tc.model, ChannelType: tc.channel, APIType: tc.api}
			applyOutputImageCharges(c, &usage, m)
			require.Equal(t, calculateImageBaseQuota(0.0336, 0, tc.expected, 1, 1), usage.ToolsCost)
		})
	}
}

// TestNativeNanoImageQualityUsesUnspecifiedDefault checks that a billing default cannot invent a provider quality selection.
func TestNativeNanoImageQualityUsesUnspecifiedDefault(t *testing.T) {
	for _, tc := range []struct {
		name, model  string
		channel, api int
		ratio        float64
	}{
		{"native", "gemini-nano-banana-2.1", channeltype.Gemini, apitype.Gemini, 1},
		{"non_gemini", "synthetic-other-image", channeltype.OpenAI, apitype.OpenAI, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "1024x1024", DefaultQuality: "hd", SizeMultipliers: map[string]float64{"1024x1024": 1}, QualityMultipliers: map[string]float64{"standard": 1, "hd": 2}}}
			channel := &dbmodel.Channel{}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{tc.model: cfg}))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.ChannelRatio, 1.0)
			c.Set(ctxkey.OutputImageCount, 1)
			c.Set(ctxkey.ChannelModel, channel)
			usage := &model.Usage{PromptTokens: 1}
			m := &meta.Meta{ActualModelName: tc.model, ChannelType: tc.channel, APIType: tc.api}
			applyOutputImageCharges(c, &usage, m)
			require.Equal(t, calculateImageBaseQuota(0.0336, 0, tc.ratio, 1, 1), usage.ToolsCost)
		})
	}
}

// TestNativeNanoExplicit1KTierOverride preserves an operator's explicit native 1K tier override.
func TestNativeNanoExplicit1KTierOverride(t *testing.T) {
	const name = "gemini-nano-banana-2.1"
	cfg := dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "2048x2048", DefaultQuality: "standard", SizeMultipliers: map[string]float64{"2048x2048": 1.5}}}
	channel := &dbmodel.Channel{}
	require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{name: cfg, "$image-tier:" + name + "|size=1024x1024": {Ratio: 0.8}}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.OutputImageCount, 1)
	c.Set(ctxkey.ChannelModel, channel)
	usage := &model.Usage{PromptTokens: 1}
	m := &meta.Meta{ActualModelName: name, ChannelType: channeltype.Gemini, APIType: apitype.Gemini}
	applyOutputImageCharges(c, &usage, m)
	require.Equal(t, calculateImageBaseQuota(0.0336, 0, 0.8, 1, 1), usage.ToolsCost)
}
