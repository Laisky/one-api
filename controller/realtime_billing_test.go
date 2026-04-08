package controller

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

// ==========================================================================
// Section 1: Pricing configuration verification
//
// These tests verify that the constants.go pricing config matches
// the official OpenAI pricing page (https://developers.openai.com/api/docs/pricing).
// ==========================================================================

// usdToQuota converts a USD-per-1M-tokens price to quota per token.
// Example: $4.00/1M → 4.0 * 500000 / 1000000 = 2.0 quota/token
func usdToQuota(usdPer1M float64) float64 {
	return usdPer1M * ratio.QuotaPerUsd / 1_000_000
}

// expectedRealtimePricing defines the official pricing for a realtime model.
type expectedRealtimePricing struct {
	TextInputUSD       float64 // $/1M text input tokens
	TextOutputUSD      float64 // $/1M text output tokens
	CachedTextInputUSD float64 // $/1M cached text input tokens
	AudioInputUSD      float64 // $/1M audio input tokens
	AudioOutputUSD     float64 // $/1M audio output tokens
}

// Official pricing from https://developers.openai.com/api/docs/pricing
var officialRealtimePricing = map[string]expectedRealtimePricing{
	"gpt-realtime-1.5": {
		TextInputUSD: 4.00, TextOutputUSD: 16.00, CachedTextInputUSD: 0.40,
		AudioInputUSD: 32.00, AudioOutputUSD: 64.00,
	},
	"gpt-realtime-mini": {
		TextInputUSD: 0.60, TextOutputUSD: 2.40, CachedTextInputUSD: 0.06,
		AudioInputUSD: 10.00, AudioOutputUSD: 20.00,
	},
	"gpt-realtime": {
		TextInputUSD: 4.00, TextOutputUSD: 16.00, CachedTextInputUSD: 0.40,
		AudioInputUSD: 32.00, AudioOutputUSD: 64.00,
	},
	"gpt-4o-realtime-preview": {
		TextInputUSD: 5.00, TextOutputUSD: 20.00, CachedTextInputUSD: 2.50,
		AudioInputUSD: 40.00, AudioOutputUSD: 80.00,
	},
	"gpt-4o-mini-realtime-preview": {
		TextInputUSD: 0.60, TextOutputUSD: 2.40, CachedTextInputUSD: 0.30,
		AudioInputUSD: 10.00, AudioOutputUSD: 20.00,
	},
}

func TestRealtimePricingMatchesOfficialOpenAI(t *testing.T) {
	t.Parallel()

	for modelName, expected := range officialRealtimePricing {
		t.Run(modelName, func(t *testing.T) {
			cfg, ok := openai.ModelRatios[modelName]
			require.True(t, ok, "%s not found in ModelRatios", modelName)

			// Text input: Ratio = price_per_1M * MilliTokensUsd
			expectedTextInputRatio := expected.TextInputUSD * ratio.MilliTokensUsd
			require.InDelta(t, expectedTextInputRatio, cfg.Ratio, 1e-6,
				"%s text input: expected $%.2f/1M → ratio %.4f", modelName, expected.TextInputUSD, expectedTextInputRatio)

			// Text output: CompletionRatio = output_price / input_price
			expectedCompletionRatio := expected.TextOutputUSD / expected.TextInputUSD
			require.InDelta(t, expectedCompletionRatio, cfg.CompletionRatio, 1e-6,
				"%s text output: expected $%.2f/$%.2f = %.4f", modelName, expected.TextOutputUSD, expected.TextInputUSD, expectedCompletionRatio)

			// Cached text input
			expectedCachedRatio := expected.CachedTextInputUSD * ratio.MilliTokensUsd
			require.InDelta(t, expectedCachedRatio, cfg.CachedInputRatio, 1e-6,
				"%s cached text input: expected $%.2f/1M → ratio %.4f", modelName, expected.CachedTextInputUSD, expectedCachedRatio)

			// Audio pricing
			require.NotNil(t, cfg.Audio, "%s: Audio pricing config must be present", modelName)

			// Audio input: PromptRatio = audio_input_price / text_input_price
			expectedAudioPromptRatio := expected.AudioInputUSD / expected.TextInputUSD
			require.InDelta(t, expectedAudioPromptRatio, cfg.Audio.PromptRatio, 0.05,
				"%s audio input: expected $%.2f/$%.2f = %.2f", modelName, expected.AudioInputUSD, expected.TextInputUSD, expectedAudioPromptRatio)

			// Audio output: CompletionRatio = audio_output_price / audio_input_price
			expectedAudioCompletionRatio := expected.AudioOutputUSD / expected.AudioInputUSD
			require.InDelta(t, expectedAudioCompletionRatio, cfg.Audio.CompletionRatio, 0.05,
				"%s audio output: expected $%.2f/$%.2f = %.2f", modelName, expected.AudioOutputUSD, expected.AudioInputUSD, expectedAudioCompletionRatio)
		})
	}
}

func TestAllRealtimeModelsHaveAudioPricingConfig(t *testing.T) {
	t.Parallel()

	realtimeModels := []string{
		"gpt-realtime-1.5",
		"gpt-realtime-mini",
		"gpt-realtime",
		"gpt-4o-realtime-preview",
		"gpt-4o-realtime-preview-2025-06-03",
		"gpt-4o-mini-realtime-preview",
		"gpt-4o-mini-realtime-preview-2024-12-17",
	}

	for _, modelName := range realtimeModels {
		t.Run(modelName, func(t *testing.T) {
			cfg, ok := openai.ModelRatios[modelName]
			require.True(t, ok, "%s not in ModelRatios", modelName)
			require.NotNil(t, cfg.Audio, "%s missing Audio pricing config", modelName)
			require.Greater(t, cfg.Audio.PromptRatio, 1.0,
				"%s AudioPromptRatio should be > 1 (audio is more expensive than text)", modelName)
			require.Greater(t, cfg.Audio.CompletionRatio, 0.0,
				"%s AudioCompletionRatio should be > 0", modelName)
		})
	}
}

// ==========================================================================
// Section 2: End-to-end billing computation tests
//
// These tests simulate a complete billing flow: given known token counts,
// verify the final quota matches the expected USD cost.
// ==========================================================================

// computeRealtimeQuota simulates the full billing pipeline for a realtime session.
func computeRealtimeQuota(t *testing.T, modelName string, usage *rmodel.Usage, groupRatio float64) int64 {
	t.Helper()

	pricingAdaptor := relay.GetAdaptor(0) // OpenAI
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)

	// Apply audio surcharge (same as controller does)
	applyRealtimeAudioSurcharge(usage, modelName, modelRatio, groupRatio,
		nil, nil, pricingAdaptor, nil)

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: pricingAdaptor,
	})

	return result.TotalQuota
}

func TestBilling_TextOnlySession_GptRealtime15(t *testing.T) {
	t.Parallel()

	// Scenario: 10K text input, 5K text output, no audio
	// Expected: 10000 * $4/1M + 5000 * $16/1M = $0.04 + $0.08 = $0.12
	// Quota: $0.12 * 500000 = 60000
	usage := &rmodel.Usage{
		PromptTokens:     10_000,
		CompletionTokens: 5_000,
	}

	quota := computeRealtimeQuota(t, "gpt-realtime-1.5", usage, 1.0)

	expectedUSD := 10_000*4.0/1_000_000 + 5_000*16.0/1_000_000
	expectedQuota := int64(math.Ceil(expectedUSD * ratio.QuotaPerUsd))

	require.InDelta(t, expectedQuota, quota, 2,
		"text-only session: expected ~$%.4f = %d quota, got %d", expectedUSD, expectedQuota, quota)
}

func TestBilling_AudioSession_GptRealtime15(t *testing.T) {
	t.Parallel()

	// Scenario: 1000 audio input, 500 audio output, 200 text input, 100 text output
	// Expected costs:
	//   Text input:  200 * $4/1M    = $0.0008
	//   Text output: 100 * $16/1M   = $0.0016
	//   Audio input:  1000 * $32/1M = $0.032
	//   Audio output: 500 * $64/1M  = $0.032
	//   Total: $0.0664
	usage := &rmodel.Usage{
		PromptTokens:     1200, // 1000 audio + 200 text
		CompletionTokens: 600,  // 500 audio + 100 text
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 1000,
			TextTokens:  200,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 500,
			TextTokens:  100,
		},
	}

	quota := computeRealtimeQuota(t, "gpt-realtime-1.5", usage, 1.0)

	// Compute expected cost manually
	textInputCost := 200.0 * 4.0 / 1_000_000
	textOutputCost := 100.0 * 16.0 / 1_000_000
	audioInputCost := 1000.0 * 32.0 / 1_000_000
	audioOutputCost := 500.0 * 64.0 / 1_000_000
	expectedUSD := textInputCost + textOutputCost + audioInputCost + audioOutputCost
	expectedQuota := int64(math.Ceil(expectedUSD * ratio.QuotaPerUsd))

	// Allow ±5% tolerance for rounding in the multi-step computation
	tolerance := float64(expectedQuota) * 0.05
	if tolerance < 2 {
		tolerance = 2
	}
	require.InDelta(t, expectedQuota, quota, tolerance,
		"audio session gpt-realtime-1.5: expected ~$%.6f = %d quota, got %d", expectedUSD, expectedQuota, quota)
}

func TestBilling_AudioSession_GptRealtimeMini(t *testing.T) {
	t.Parallel()

	// Scenario: 5000 audio input, 2000 audio output
	// Expected:
	//   Audio input:  5000 * $10/1M = $0.05
	//   Audio output: 2000 * $20/1M = $0.04
	//   Total: $0.09
	usage := &rmodel.Usage{
		PromptTokens:     5000,
		CompletionTokens: 2000,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 5000,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 2000,
		},
	}

	quota := computeRealtimeQuota(t, "gpt-realtime-mini", usage, 1.0)

	expectedUSD := 5000.0*10.0/1_000_000 + 2000.0*20.0/1_000_000
	expectedQuota := int64(math.Ceil(expectedUSD * ratio.QuotaPerUsd))

	tolerance := float64(expectedQuota) * 0.05
	if tolerance < 2 {
		tolerance = 2
	}
	require.InDelta(t, expectedQuota, quota, tolerance,
		"audio session gpt-realtime-mini: expected ~$%.6f = %d quota, got %d", expectedUSD, expectedQuota, quota)
}

func TestBilling_MixedSession_GptRealtimeMini(t *testing.T) {
	t.Parallel()

	// Scenario: Mixed audio+text in gpt-realtime-mini
	//   Input: 3000 audio + 1000 text = 4000 total
	//   Output: 1500 audio + 500 text = 2000 total
	// Expected:
	//   Text input:  1000 * $0.60/1M = $0.0006
	//   Text output: 500 * $2.40/1M  = $0.0012
	//   Audio input:  3000 * $10/1M  = $0.03
	//   Audio output: 1500 * $20/1M  = $0.03
	//   Total: $0.0618
	usage := &rmodel.Usage{
		PromptTokens:     4000,
		CompletionTokens: 2000,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 3000,
			TextTokens:  1000,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 1500,
			TextTokens:  500,
		},
	}

	quota := computeRealtimeQuota(t, "gpt-realtime-mini", usage, 1.0)

	textInputCost := 1000.0 * 0.6 / 1_000_000
	textOutputCost := 500.0 * 2.4 / 1_000_000
	audioInputCost := 3000.0 * 10.0 / 1_000_000
	audioOutputCost := 1500.0 * 20.0 / 1_000_000
	expectedUSD := textInputCost + textOutputCost + audioInputCost + audioOutputCost
	expectedQuota := int64(math.Ceil(expectedUSD * ratio.QuotaPerUsd))

	tolerance := float64(expectedQuota) * 0.05
	if tolerance < 2 {
		tolerance = 2
	}
	require.InDelta(t, expectedQuota, quota, tolerance,
		"mixed session gpt-realtime-mini: expected ~$%.6f = %d quota, got %d", expectedUSD, expectedQuota, quota)
}

func TestBilling_GroupRatioMultiplier(t *testing.T) {
	t.Parallel()

	// Same session but with group ratio of 2.0 (VIP pricing)
	usage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 1000,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 500,
		},
	}

	quotaGroup1 := computeRealtimeQuota(t, "gpt-realtime-1.5", cloneUsage(usage), 1.0)
	quotaGroup2 := computeRealtimeQuota(t, "gpt-realtime-1.5", cloneUsage(usage), 2.0)

	// With groupRatio=2.0, quota should be approximately 2x
	require.InDelta(t, float64(quotaGroup1)*2, float64(quotaGroup2), float64(quotaGroup1)*0.05,
		"group ratio 2x should double the quota: 1x=%d, 2x=%d", quotaGroup1, quotaGroup2)
}

// cloneUsage creates a deep copy of Usage for tests that modify it.
func cloneUsage(u *rmodel.Usage) *rmodel.Usage {
	c := *u
	if u.PromptTokensDetails != nil {
		d := *u.PromptTokensDetails
		c.PromptTokensDetails = &d
	}
	if u.CompletionTokensDetails != nil {
		d := *u.CompletionTokensDetails
		c.CompletionTokensDetails = &d
	}
	return &c
}

// ==========================================================================
// Section 3: Audio surcharge correctness
// ==========================================================================

func TestAudioSurcharge_ExactValue_GptRealtime15(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0)
	modelName := "gpt-realtime-1.5"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)
	groupRatio := 1.0

	// 1000 audio input tokens, 500 audio output tokens
	usage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 1000,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 500,
		},
	}

	applyRealtimeAudioSurcharge(usage, modelName, modelRatio, groupRatio,
		nil, nil, pricingAdaptor, nil)

	// Input surcharge: 1000 tokens * modelRatio * groupRatio * (8 - 1)
	//   = 1000 * 2.0 * 1.0 * 7 = 14000
	// Output surcharge: 500 tokens * modelRatio * groupRatio * (8*2 - 4)
	//   = 500 * 2.0 * 1.0 * 12 = 12000
	// Total surcharge: 26000
	expectedInputSurcharge := 1000.0 * modelRatio * groupRatio * (8.0 - 1.0)
	expectedOutputSurcharge := 500.0 * modelRatio * groupRatio * (8.0*2.0 - 4.0) // 4.0 = CompletionRatio
	expectedSurcharge := int64(math.Ceil(expectedInputSurcharge + expectedOutputSurcharge))

	require.Equal(t, expectedSurcharge, usage.ToolsCost,
		"audio surcharge mismatch: expected %d, got %d", expectedSurcharge, usage.ToolsCost)
}

func TestAudioSurcharge_AllAudioVsAllText_CostRatio(t *testing.T) {
	t.Parallel()

	// For gpt-realtime-1.5:
	// All-audio: input $32/1M + output $64/1M
	// All-text:  input $4/1M + output $16/1M
	// Ratio should be (32+64)/(4+16) = 96/20 = 4.8x for equal in/out tokens

	modelName := "gpt-realtime-1.5"

	audioUsage := &rmodel.Usage{
		PromptTokens:     10000,
		CompletionTokens: 10000,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 10000,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 10000,
		},
	}
	textUsage := &rmodel.Usage{
		PromptTokens:     10000,
		CompletionTokens: 10000,
	}

	audioQuota := computeRealtimeQuota(t, modelName, audioUsage, 1.0)
	textQuota := computeRealtimeQuota(t, modelName, textUsage, 1.0)

	actualRatio := float64(audioQuota) / float64(textQuota)
	expectedRatio := (32.0 + 64.0) / (4.0 + 16.0) // 4.8

	require.InDelta(t, expectedRatio, actualRatio, 0.3,
		"all-audio vs all-text cost ratio: expected ~%.1fx, got %.1fx (audio=%d, text=%d)",
		expectedRatio, actualRatio, audioQuota, textQuota)
}

// ==========================================================================
// Section 4: Billing detail construction for logs
// ==========================================================================

func TestBillingDetail_ContainsCorrectFields(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0)
	modelName := "gpt-realtime-1.5"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)
	groupRatio := 1.0

	usage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 800,
			TextTokens:  200,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 400,
			TextTokens:  100,
		},
	}

	// Apply audio surcharge
	applyRealtimeAudioSurcharge(usage, modelName, modelRatio, groupRatio,
		nil, nil, pricingAdaptor, nil)

	// Compute quota
	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: pricingAdaptor,
	})

	// Verify the billing detail has all expected fields
	require.Equal(t, 1000, result.PromptTokens, "prompt tokens")
	require.Equal(t, 500, result.CompletionTokens, "completion tokens")
	require.Greater(t, result.TotalQuota, int64(0), "total quota")
	require.Greater(t, result.UsedModelRatio, 0.0, "model ratio")
	require.Greater(t, result.UsedCompletionRatio, 0.0, "completion ratio")

	// ToolsCost should be included in TotalQuota
	require.Greater(t, usage.ToolsCost, int64(0), "audio surcharge should be positive")

	// Total should be higher than text-only billing
	textOnlyUsage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}
	textOnlyResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:          textOnlyUsage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: pricingAdaptor,
	})
	require.Greater(t, result.TotalQuota, textOnlyResult.TotalQuota,
		"audio billing (%d) should exceed text-only billing (%d)", result.TotalQuota, textOnlyResult.TotalQuota)
}

// ==========================================================================
// Section 5: Edge cases and safety
// ==========================================================================

func TestBilling_ZeroAudioTokens_NoSurcharge(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 0,
			TextTokens:  1000,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 0,
			TextTokens:  500,
		},
	}

	pricingAdaptor := relay.GetAdaptor(0)
	modelRatio := pricing.GetModelRatioWithThreeLayers("gpt-realtime-1.5", nil, pricingAdaptor)

	applyRealtimeAudioSurcharge(usage, "gpt-realtime-1.5", modelRatio, 1.0,
		nil, nil, pricingAdaptor, nil)

	require.Equal(t, int64(0), usage.ToolsCost,
		"zero audio tokens should produce zero surcharge")
}

func TestBilling_NilDetails_NoSurcharge(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}

	pricingAdaptor := relay.GetAdaptor(0)
	modelRatio := pricing.GetModelRatioWithThreeLayers("gpt-realtime-1.5", nil, pricingAdaptor)

	applyRealtimeAudioSurcharge(usage, "gpt-realtime-1.5", modelRatio, 1.0,
		nil, nil, pricingAdaptor, nil)

	require.Equal(t, int64(0), usage.ToolsCost)
}

func TestBilling_NilUsage_NoPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		applyRealtimeAudioSurcharge(nil, "gpt-realtime-1.5", 1.0, 1.0,
			nil, nil, nil, nil)
	})
}

func TestBilling_ModelWithoutAudioConfig_NoSurcharge(t *testing.T) {
	t.Parallel()

	// gpt-4o is not a realtime model, has no Audio config
	usage := &rmodel.Usage{
		PromptTokens: 1000,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 1000,
		},
	}

	pricingAdaptor := relay.GetAdaptor(0)
	applyRealtimeAudioSurcharge(usage, "gpt-4o", 1.0, 1.0,
		nil, nil, pricingAdaptor, nil)

	require.Equal(t, int64(0), usage.ToolsCost,
		"model without audio config should not add surcharge")
}

// ==========================================================================
// Section 6: Audio pricing resolves via adaptor
// ==========================================================================

func TestAudioPricingResolvesViaAdaptor(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0) // OpenAI

	for _, modelName := range []string{
		"gpt-realtime-1.5",
		"gpt-realtime-mini",
		"gpt-4o-realtime-preview",
		"gpt-4o-mini-realtime-preview",
	} {
		t.Run(modelName, func(t *testing.T) {
			cfg, ok := pricing.ResolveAudioPricing(modelName, nil, pricingAdaptor)
			require.True(t, ok, "%s: ResolveAudioPricing should succeed", modelName)
			require.NotNil(t, cfg)
			require.Greater(t, cfg.PromptRatio, 1.0,
				"%s: audio prompt ratio should be > 1", modelName)
			require.Greater(t, cfg.CompletionRatio, 0.0,
				"%s: audio completion ratio should be > 0", modelName)
		})
	}
}

// Verify adaptor.GetDefaultModelPricing returns Audio config for realtime models
func TestAdaptorDefaultPricingIncludesAudio(t *testing.T) {
	t.Parallel()

	adaptorInstance := relay.GetAdaptor(0)
	require.NotNil(t, adaptorInstance)

	// Verify it implements the Adaptor interface
	var _ adaptor.Adaptor = adaptorInstance

	defaults := adaptorInstance.GetDefaultModelPricing()
	require.NotNil(t, defaults)

	for _, modelName := range []string{
		"gpt-realtime-1.5",
		"gpt-realtime-mini",
	} {
		cfg, ok := defaults[modelName]
		require.True(t, ok, "%s not in default pricing", modelName)
		require.NotNil(t, cfg.Audio, "%s: Audio config missing in default pricing", modelName)
	}
}
