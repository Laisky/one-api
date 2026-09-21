package xai

import (
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

func TestGrok47CatalogMetadata(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["grok-4.7"]
	require.True(t, ok)
	assert.Equal(t, 2.0*ratio.MilliTokensUsd, cfg.Ratio)
	assert.Equal(t, 3.0, cfg.CompletionRatio)
	assert.Equal(t, 0.5*ratio.MilliTokensUsd, cfg.CachedInputRatio)
	assert.Equal(t, int32(500000), cfg.ContextLength)
	assert.Equal(t, []string{"text", "image"}, cfg.InputModalities)
	assert.Equal(t, []string{"text"}, cfg.OutputModalities)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"}, cfg.SupportedReasoningEfforts)
	assert.Equal(t, "high", cfg.DefaultReasoningEffort)

	require.Len(t, cfg.Tiers, 1)
	assert.Equal(t, 200000, cfg.Tiers[0].InputTokenThreshold)
	assert.Equal(t, 4.0*ratio.MilliTokensUsd, cfg.Tiers[0].Ratio)
	assert.Equal(t, 3.0, cfg.Tiers[0].CompletionRatio)
	assert.Equal(t, 1.0*ratio.MilliTokensUsd, cfg.Tiers[0].CachedInputRatio)
}

func TestOfficialAliasesMirrorCanonicalModels(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"grok-build-latest":                   "grok-4.5",
		"grok-4.3-latest":                     "grok-4.3",
		"grok-code-fast":                      "grok-build-0.1",
		"grok-code-fast-1-0825":               "grok-build-0.1",
		"grok-4.20-reasoning-latest":          "grok-4.20-0309-reasoning",
		"grok-4.20-non-reasoning-latest":      "grok-4.20-0309-non-reasoning",
		"grok-4.20-multi-agent-latest":        "grok-4.20-multi-agent-0309",
		"grok-4.20-beta-0309-non-reasoning":   "grok-4.20-0309-non-reasoning",
		"grok-4.20-multi-agent-beta-0309":     "grok-4.20-multi-agent-0309",
		"grok-4.20-experimental-beta-latest":  "grok-4.20-0309-reasoning",
		"grok-4.20-reasoning-gv2":             "grok-4.20-0309-reasoning",
		"grok-4.20-non-reasoning-gv2":         "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-latest-non-reasoning": "grok-4.20-0309-non-reasoning",
	}

	for alias, canonical := range tests {
		aliasName, canonicalName := alias, canonical
		t.Run(aliasName, func(t *testing.T) {
			t.Parallel()
			aliasCfg, aliasOK := ModelRatios[aliasName]
			canonicalCfg, canonicalOK := ModelRatios[canonicalName]
			require.True(t, aliasOK)
			require.True(t, canonicalOK)
			assert.Equal(t, canonicalCfg.Ratio, aliasCfg.Ratio)
			assert.Equal(t, canonicalCfg.CompletionRatio, aliasCfg.CompletionRatio)
			assert.Equal(t, canonicalCfg.CachedInputRatio, aliasCfg.CachedInputRatio)
			assert.Equal(t, canonicalCfg.ContextLength, aliasCfg.ContextLength)
			assert.Equal(t, canonicalCfg.SupportedReasoningEfforts, aliasCfg.SupportedReasoningEfforts)
		})
	}
}

func TestRefreshedReasoningEfforts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"grok-4.7", "grok-4.6", "grok-4.6-latest", "grok-4.5", "grok-4.5-latest"} {
		cfg, ok := ModelRatios[name]
		require.True(t, ok)
		assert.Contains(t, cfg.SupportedReasoningEfforts, "xhigh")
	}

	cfg, ok := ModelRatios["grok-4.3-latest"]
	require.True(t, ok)
	assert.Equal(t, []string{"none", "low", "medium", "high", "xhigh"}, cfg.SupportedReasoningEfforts)
}

func TestImaginePricingAndModalitiesRefresh(t *testing.T) {
	t.Parallel()

	image20, ok := ModelRatios["grok-imagine-image-2.0"]
	require.True(t, ok)
	require.NotNil(t, image20.Image)
	assert.Equal(t, "auto", image20.Image.DefaultQuality)
	assert.Equal(t, 1.0, image20.Image.QualitySizeMultipliers["low"]["1024x1024"])
	assert.Equal(t, 2.0, image20.Image.QualitySizeMultipliers["medium"]["2048x2048"])
	assert.NotContains(t, image20.Image.QualitySizeMultipliers["low"], "1536x1536")

	quality, ok := ModelRatios["grok-imagine-image-quality"]
	require.True(t, ok)
	require.NotNil(t, quality.Image)
	assert.Equal(t, 1.2, quality.Image.SizeMultipliers["1408x1408"])
	assert.Contains(t, quality.Description, "November 2")
	require.Len(t, quality.TimeWindows, 1)

	image1, ok := ModelRatios["grok-imagine-image"]
	require.True(t, ok)
	require.NotNil(t, image1.Image)
	assert.Equal(t, 1.0, image1.Image.SizeMultipliers["2048x2048"])

	video15, ok := ModelRatios["grok-imagine-video-1.5"]
	require.True(t, ok)
	assert.Equal(t, []string{"text", "image", "audio"}, video15.InputModalities)
	assert.Equal(t, []string{"video"}, video15.OutputModalities)

	classicVideo, ok := ModelRatios["grok-imagine-video"]
	require.True(t, ok)
	assert.Equal(t, []string{"text", "image", "video"}, classicVideo.InputModalities)
}

func TestLegacyImagineQualityPricingTransitionsAtRedirectDate(t *testing.T) {
	t.Parallel()

	before := time.Date(2026, time.November, 1, 23, 59, 0, 0, time.UTC)
	after := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{
		"grok-imagine-image-quality",
		"grok-imagine-image-quality-20260403",
		"grok-imagine-image-quality-latest",
		"grok-imagine-image-pro",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			oldPricing, ok := pricing.ResolveImagePricing(name, nil, &Adaptor{}, before)
			require.True(t, ok)
			require.NotNil(t, oldPricing)
			assert.InDelta(t, 0.05, oldPricing.PricePerImageUsd, 1e-12)
			assert.InDelta(t, 1.2, oldPricing.SizeMultipliers["1408x1408"], 1e-12)
			assert.InDelta(t, 1.4, oldPricing.SizeMultipliers["2048x2048"], 1e-12)

			redirectPricing, ok := pricing.ResolveImagePricing(name, nil, &Adaptor{}, after)
			require.True(t, ok)
			require.NotNil(t, redirectPricing)
			assert.InDelta(t, 0.04, redirectPricing.PricePerImageUsd, 1e-12)
			assert.InDelta(t, 1.25, redirectPricing.SizeMultipliers["1408x1408"], 1e-12)
			assert.InDelta(t, 1.5, redirectPricing.SizeMultipliers["2048x2048"], 1e-12)
		})
	}
}

func TestGrok47RequestConversionUsesCatalogMetadata(t *testing.T) {
	t.Parallel()

	presence := 0.4
	frequency := 0.2
	effort := "xhigh"
	request := &model.GeneralOpenAIRequest{
		Model:            "grok-4.7",
		ReasoningEffort:  &effort,
		PresencePenalty:  &presence,
		FrequencyPenalty: &frequency,
		Stop:             []string{"END"},
		Messages:         []model.Message{{Role: "user", Content: "hello"}},
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	got, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, got.ReasoningEffort)
	assert.Equal(t, "xhigh", *got.ReasoningEffort)
	assert.Nil(t, got.PresencePenalty)
	assert.Nil(t, got.FrequencyPenalty)
	assert.Nil(t, got.Stop)
}

func TestGrok47StripsUnsupportedReasoningEffort(t *testing.T) {
	t.Parallel()

	effort := "none"
	request := &model.GeneralOpenAIRequest{
		Model:           "grok-4.7",
		ReasoningEffort: &effort,
		Messages:        []model.Message{{Role: "user", Content: "hello"}},
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	got, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Nil(t, got.ReasoningEffort)
}

func TestLegacyPenaltyFilteringRemainsCompatible(t *testing.T) {
	t.Parallel()

	presence := 0.4
	frequency := 0.2
	request := &model.GeneralOpenAIRequest{
		Model:            "grok-4-0709",
		PresencePenalty:  &presence,
		FrequencyPenalty: &frequency,
		Messages:         []model.Message{{Role: "user", Content: "hello"}},
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	got, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Nil(t, got.PresencePenalty)
	assert.Nil(t, got.FrequencyPenalty)
}

func TestImagineImage20RequestConversion(t *testing.T) {
	t.Parallel()

	request := &model.ImageRequest{
		Model:   "grok-imagine-image-2.0",
		Prompt:  "an astronaut in Ottawa",
		Quality: "medium",
		Size:    "2048x2048",
		Style:   "vivid",
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertImageRequest(ctx, request)
	require.NoError(t, err)

	got, ok := converted.(*model.ImageRequest)
	require.True(t, ok)
	assert.Equal(t, "2k", got.Resolution)
	assert.Equal(t, "medium", got.Quality)
	assert.Empty(t, got.Size)
	assert.Empty(t, got.Style)
}

func TestImagineImageQualityParameterCompatibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		model       string
		quality     string
		wantQuality string
	}{
		{name: "image 2.0 accepts low", model: "grok-imagine-image-2.0", quality: "low", wantQuality: "low"},
		{name: "image 2.0 accepts auto", model: "grok-imagine-image-2.0", quality: "auto", wantQuality: "auto"},
		{name: "image 2.0 rejects unknown quality", model: "grok-imagine-image-2.0", quality: "hd"},
		{name: "legacy model strips quality", model: "grok-imagine-image", quality: "medium"},
	}

	for _, tt := range tests {
		t := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := &model.ImageRequest{Model: tt.model, Prompt: "test", Quality: tt.quality}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			converted, err := (&Adaptor{}).ConvertImageRequest(ctx, request)
			require.NoError(t, err)

			got, ok := converted.(*model.ImageRequest)
			require.True(t, ok)
			assert.Equal(t, tt.wantQuality, got.Quality)
		})
	}
}

func TestModelListContainsPublishedCatalogEntries(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	for _, name := range []string{
		"grok-4.7",
		"grok-4.3-latest",
		"grok-build-latest",
		"grok-code-fast",
		"grok-4.20-reasoning-latest",
		"grok-4.20-non-reasoning-latest",
		"grok-4.20-multi-agent-latest",
	} {
		assert.True(t, slices.Contains(models, name), name)
	}
	assert.True(t, strings.Contains(ModelRatios["grok-4.7"].Description, "SpaceXAI"))
}
