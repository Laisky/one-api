package zhipu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestCurrentTextModelMetadata verifies current official model identifiers and
// exact context, output, and pricing metadata published by BigModel.
func TestCurrentTextModelMetadata(t *testing.T) {
	t.Parallel()

	glm53, ok := ModelRatios["glm-5.3"]
	require.True(t, ok)
	require.Equal(t, int32(1_000_000), glm53.ContextLength)
	require.Equal(t, int32(131_072), glm53.MaxOutputTokens)
	require.Equal(t, []string{"low", "high", "max"}, glm53.SupportedReasoningEfforts)
	require.Equal(t, "max", glm53.DefaultReasoningEffort)
	require.InDelta(t, 8*ratio.MilliTokensRmb, glm53.Ratio, 1e-12)

	glm52, ok := ModelRatios["glm-5.2"]
	require.True(t, ok)
	require.Equal(t, int32(1_000_000), glm52.ContextLength)
	require.Equal(t, int32(131_072), glm52.MaxOutputTokens)
	require.InDelta(t, 8*ratio.MilliTokensRmb, glm52.Ratio, 1e-12)
	require.InDelta(t, 28.0/8.0, glm52.CompletionRatio, 1e-12)
	require.InDelta(t, 2*ratio.MilliTokensRmb, glm52.CachedInputRatio, 1e-12)

	glm51, ok := ModelRatios["glm-5.1"]
	require.True(t, ok)
	require.Equal(t, int32(200_000), glm51.ContextLength)
	require.Equal(t, int32(131_072), glm51.MaxOutputTokens)
	require.Len(t, glm51.Tiers, 1)
	require.Equal(t, 32_000, glm51.Tiers[0].InputTokenThreshold)

	airSnapshot, ok := ModelRatios["glm-4-air-250414"]
	require.True(t, ok)
	require.Equal(t, int32(131_072), airSnapshot.ContextLength)
	require.Equal(t, int32(16_384), airSnapshot.MaxOutputTokens)
}

// TestCurrentVisionModelMetadata verifies exact limits and pricing thresholds
// for the current flagship visual models.
func TestCurrentVisionModelMetadata(t *testing.T) {
	t.Parallel()

	autoglm, ok := ModelRatios["autoglm-phone"]
	require.True(t, ok)
	require.Equal(t, int32(20_000), autoglm.ContextLength)
	require.Equal(t, int32(2_048), autoglm.MaxOutputTokens)
	require.Equal(t, 0.0, autoglm.Ratio)

	glm5v, ok := ModelRatios["glm-5v-turbo"]
	require.True(t, ok)
	require.Equal(t, int32(200_000), glm5v.ContextLength)
	require.Equal(t, int32(131_072), glm5v.MaxOutputTokens)
	require.Len(t, glm5v.Tiers, 1)
	require.Equal(t, 32_000, glm5v.Tiers[0].InputTokenThreshold)

	glm4v, ok := ModelRatios["glm-4v-plus-0111"]
	require.True(t, ok)
	require.Equal(t, int32(8_192), glm4v.MaxOutputTokens)

	// GLM-5.3-Flash's BASE price is the standard list price (¥0.8/¥0.23/¥2.8);
	// the two-week launch promotion is a time window, so quota returns to list
	// on its own once the promotion lapses.
	glm53Flash, ok := ModelRatios["glm-5.3-flash"]
	require.True(t, ok)
	require.Equal(t, int32(1_000_000), glm53Flash.ContextLength)
	require.Equal(t, int32(131_072), glm53Flash.MaxOutputTokens)
	require.Equal(t, []string{"text", "image", "video", "file"}, glm53Flash.InputModalities)
	require.Empty(t, glm53Flash.Tiers)
	require.InDelta(t, 0.8*ratio.MilliTokensRmb, glm53Flash.Ratio, 1e-12)
	require.InDelta(t, 2.8/0.8, glm53Flash.CompletionRatio, 1e-12)
	require.InDelta(t, 0.23*ratio.MilliTokensRmb, glm53Flash.CachedInputRatio, 1e-12)
}

// TestGLM53FlashLaunchPromoWindow verifies the two-week 50% launch promotion is
// expressed as a self-expiring time window over the list price: discounted
// through 2026-09-09 24:00 Asia/Shanghai, back to list immediately afterwards.
func TestGLM53FlashLaunchPromoWindow(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["glm-5.3-flash"]
	require.True(t, ok)
	require.Len(t, cfg.TimeWindows, 1)
	require.Equal(t, "glm-5.3-flash-launch-promo", cfg.TimeWindows[0].Name)
	require.Equal(t, "Asia/Shanghai", cfg.TimeWindows[0].TimeZone)
	require.Equal(t, "2026-09-10", cfg.TimeWindows[0].DateTo)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	// Last discounted minute: 2026-09-09 23:59 UTC+8.
	promo := pricing.ApplyTimeWindow(cfg, time.Date(2026, 9, 9, 23, 59, 0, 0, shanghai))
	require.InDelta(t, 0.4*ratio.MilliTokensRmb, promo.Ratio, 1e-12)
	require.InDelta(t, 0.115*ratio.MilliTokensRmb, promo.CachedInputRatio, 1e-12)
	// Output is 50% off through the inherited completion ratio: 0.4 * 3.5 = 1.4.
	require.InDelta(t, 1.4*ratio.MilliTokensRmb, promo.Ratio*promo.CompletionRatio, 1e-12)

	// First minute after expiry: 2026-09-10 00:00 UTC+8 bills at list again.
	expired := pricing.ApplyTimeWindow(cfg, time.Date(2026, 9, 10, 0, 0, 0, 0, shanghai))
	require.InDelta(t, 0.8*ratio.MilliTokensRmb, expired.Ratio, 1e-12)
	require.InDelta(t, 0.23*ratio.MilliTokensRmb, expired.CachedInputRatio, 1e-12)
	require.InDelta(t, 2.8*ratio.MilliTokensRmb, expired.Ratio*expired.CompletionRatio, 1e-12)
}

// TestImageGenerationPerCallPricing verifies BigModel's image models are billed
// per rendered image (元/次) through Image.PricePerImageUsd rather than being
// mistaken for per-token models, which would under-charge by ~1e6x.
func TestImageGenerationPerCallPricing(t *testing.T) {
	t.Parallel()

	for name, cny := range map[string]float64{
		"glm-image":      0.1,
		"cogview-4":      0.06,
		"cogview-3-plus": 0.08,
		"cogview-3":      0.04,
	} {
		cfg, ok := ModelRatios[name]
		require.True(t, ok, name)
		require.Zero(t, cfg.Ratio, name)
		require.NotNil(t, cfg.Image, name)
		require.InDelta(t, cny/ratio.ExchangeRateRmb, cfg.Image.PricePerImageUsd, 1e-12, name)
	}

	free, ok := ModelRatios["cogview-3-flash"]
	require.True(t, ok)
	require.Zero(t, free.Ratio)
	require.Nil(t, free.Image)
}

// TestCogVideoXPerCallPricing verifies the CogVideoX video models carry the
// per-call pricing the video relay bills from.
func TestCogVideoXPerCallPricing(t *testing.T) {
	t.Parallel()

	for name, cny := range map[string]float64{
		"cogvideox-3": 1,
		"cogvideox-2": 0.5,
	} {
		cfg, ok := ModelRatios[name]
		require.True(t, ok, name)
		require.NotNil(t, cfg.PerCall, name)
		require.InDelta(t, cny*ratio.QuotaPerRMB, cfg.Ratio, 1e-9, name)
		require.InDelta(t, cny/7*1000, cfg.PerCall.UsdPerThousandCalls, 1e-9, name)
	}
}

// TestViduPerCallPricing verifies the published per-call prices for Vidu Q1 and
// Vidu 2 video generation models, encoded both as quota (Ratio) and per-call USD.
func TestViduPerCallPricing(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"viduq1-image", "viduq1-start-end", "viduq1-text", "vidu2-reference"} {
		cfg, ok := ModelRatios[model]
		require.True(t, ok)
		require.NotNil(t, cfg.PerCall)
		require.InDelta(t, 2.5*ratio.QuotaPerRMB, cfg.Ratio, 1e-9)
		require.InDelta(t, 2.5/7*1000, cfg.PerCall.UsdPerThousandCalls, 1e-9)
	}

	for _, model := range []string{"vidu2-image", "vidu2-start-end"} {
		cfg, ok := ModelRatios[model]
		require.True(t, ok)
		require.NotNil(t, cfg.PerCall)
		require.InDelta(t, 1.25*ratio.QuotaPerRMB, cfg.Ratio, 1e-9)
		require.InDelta(t, 1.25/7*1000, cfg.PerCall.UsdPerThousandCalls, 1e-9)
	}
}

// TestAudioModelMetadata verifies the registered GLM audio models and their
// placeholder pricing metadata.
func TestAudioModelMetadata(t *testing.T) {
	t.Parallel()

	tts, ok := ModelRatios["glm-tts"]
	require.True(t, ok)
	require.Equal(t, []string{"text"}, tts.InputModalities)
	require.Equal(t, []string{"audio"}, tts.OutputModalities)
	require.NotNil(t, tts.Audio)
	require.InDelta(t, (0.2/3000)*ratio.QuotaPerRMB, tts.Ratio, 1e-9)

	asr, ok := ModelRatios["glm-asr-2512"]
	require.True(t, ok)
	require.Equal(t, []string{"audio"}, asr.InputModalities)
	require.Equal(t, []string{"text"}, asr.OutputModalities)
	require.NotNil(t, asr.Audio)
	require.InDelta(t, 10.0, asr.Audio.PromptTokensPerSecond, 1e-9)
	require.InDelta(t, (0.5/600)*ratio.QuotaPerRMB, asr.Ratio, 1e-9)

	clone, ok := ModelRatios["glm-tts-clone"]
	require.True(t, ok)
	require.NotNil(t, clone.PerCall)
	require.InDelta(t, 2*ratio.QuotaPerRMB, clone.Ratio, 1e-9)
	require.InDelta(t, 2.0/7*1000, clone.PerCall.UsdPerThousandCalls, 1e-9)
}

// TestRealtimeModelMetadata verifies the GLM-Realtime models carry pricing
// consistent with BigModel's published per-minute audio rates.
func TestRealtimeModelMetadata(t *testing.T) {
	t.Parallel()

	flash, ok := ModelRatios["glm-realtime-flash"]
	require.True(t, ok)
	require.Equal(t, []string{"text", "audio", "video"}, flash.InputModalities)
	require.Equal(t, []string{"audio"}, flash.OutputModalities)
	require.InDelta(t, (0.18/1800)*ratio.QuotaPerRMB, flash.Ratio, 1e-12)

	air, ok := ModelRatios["glm-realtime-air"]
	require.True(t, ok)
	require.Equal(t, []string{"text", "audio", "video"}, air.InputModalities)
	require.InDelta(t, (0.3/1800)*ratio.QuotaPerRMB, air.Ratio, 1e-12)
}

// TestZhipuTieredPricingResolution verifies BigModel's 32K input and 0.2K
// output boundaries without conflating thousands of tokens with raw tokens.
func TestZhipuTieredPricingResolution(t *testing.T) {
	t.Parallel()

	glm47 := ModelRatios["glm-4.7"]
	short := pricing.ResolveEffectivePricingForUsageFromConfig(31_999, 199, glm47)
	require.InDelta(t, 2*ratio.MilliTokensRmb, short.InputRatio, 1e-12)
	require.InDelta(t, 8*ratio.MilliTokensRmb, short.OutputRatio, 1e-12)

	longOutput := pricing.ResolveEffectivePricingForUsageFromConfig(31_999, 200, glm47)
	require.InDelta(t, 3*ratio.MilliTokensRmb, longOutput.InputRatio, 1e-12)
	require.InDelta(t, 14*ratio.MilliTokensRmb, longOutput.OutputRatio, 1e-12)
	require.Equal(t, 200, longOutput.AppliedOutputTierThreshold)

	longInput := pricing.ResolveEffectivePricingForUsageFromConfig(32_000, 199, glm47)
	require.InDelta(t, 4*ratio.MilliTokensRmb, longInput.InputRatio, 1e-12)
	require.InDelta(t, 16*ratio.MilliTokensRmb, longInput.OutputRatio, 1e-12)
	require.Equal(t, 32_000, longInput.AppliedTierThreshold)
}
