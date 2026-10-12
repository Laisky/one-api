package zhipu_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zai"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// catalogView contains the model discovery and configuration exposed by an adaptor.
type catalogView struct {
	models  []string
	configs map[string]adaptor.ModelConfig
}

// publishedCatalogs returns both public adaptor catalogues without mutating their defaults.
func publishedCatalogs() map[string]catalogView {
	cn, international := &zhipu.Adaptor{}, &zai.Adaptor{}
	return map[string]catalogView{
		"bigmodel": {models: cn.GetModelList(), configs: cn.GetDefaultModelPricing()},
		"zai":     {models: international.GetModelList(), configs: international.GetDefaultModelPricing()},
	}
}

// TestGLM53FamilyPublishedMetadata verifies the documented public model metadata,
// including invalid capability exclusions and the serialized client-facing fields.
func TestGLM53FamilyPublishedMetadata(t *testing.T) {
	t.Parallel()
	for provider, catalog := range publishedCatalogs() {
		t.Run(provider, func(t *testing.T) {
			for _, id := range []string{"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx"} {
				t.Run(id, func(t *testing.T) {
					cfg, ok := catalog.configs[id]
					require.True(t, ok)
					t.Run("efforts", func(t *testing.T) {
						require.Equal(t, []string{"low", "high", "max"}, cfg.SupportedReasoningEfforts)
						require.Equal(t, "max", cfg.DefaultReasoningEffort)
					})
					t.Run("sampling", func(t *testing.T) {
						for _, parameter := range []string{"temperature", "top_p", "max_tokens", "stop", "reasoning_effort"} {
							require.Contains(t, cfg.SupportedSamplingParameters, parameter)
						}
						for _, parameter := range []string{"top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "seed", "logit_bias"} {
							require.NotContains(t, cfg.SupportedSamplingParameters, parameter)
						}
					})
					t.Run("features", func(t *testing.T) {
						// JSON mode does not promise provider-enforced JSON Schema.
						require.ElementsMatch(t, []string{"tools", "json_mode", "web_search", "reasoning"}, cfg.SupportedFeatures)
					})
					t.Run("limits", func(t *testing.T) {
						require.Equal(t, int32(1_000_000), cfg.ContextLength)
						require.Equal(t, int32(131_072), cfg.MaxOutputTokens)
						require.Equal(t, []string{"text"}, cfg.OutputModalities)
						inputs := []string{"text", "image", "video", "file"}
						if id == "glm-5.3" {
							inputs = []string{"text"}
						}
						require.Equal(t, inputs, cfg.InputModalities)
					})
					t.Run("weights", func(t *testing.T) {
						switch id {
						case "glm-5.3":
							require.Equal(t, "zai-org/GLM-5.3", cfg.HuggingFaceID)
						case "glm-5.3-flash":
							require.Equal(t, "zai-org/GLM-5.3-Flash", cfg.HuggingFaceID)
						default:
							// A serving-speed variant is not a separately verified weights release.
							require.Empty(t, cfg.HuggingFaceID)
						}
					})
					t.Run("discovery", func(t *testing.T) {
						count := 0
						for _, listed := range catalog.models {
							if listed == id {
								count++
							}
						}
						require.Equal(t, 1, count)
						payload, err := json.Marshal(cfg)
						require.NoError(t, err)
						var decoded adaptor.ModelConfig
						require.NoError(t, json.Unmarshal(payload, &decoded))
						require.Equal(t, cfg.SupportedReasoningEfforts, decoded.SupportedReasoningEfforts)
						require.Equal(t, cfg.DefaultReasoningEffort, decoded.DefaultReasoningEffort)
						require.Equal(t, cfg.SupportedFeatures, decoded.SupportedFeatures)
						require.Equal(t, cfg.SupportedSamplingParameters, decoded.SupportedSamplingParameters)
						require.Equal(t, cfg.HuggingFaceID, decoded.HuggingFaceID)
					})
				})
			}
		})
	}
}

// TestGLM53FamilyPublishedPricing verifies current input, output and cached-input
// tariffs through the public catalogues and real time-window/tier resolution.
func TestGLM53FamilyPublishedPricing(t *testing.T) {
	t.Parallel()
	catalogs := publishedCatalogs()
	for _, tc := range []struct {
		provider, model      string
		input, output, cache float64
		unit                 float64
	}{
		{"bigmodel", "glm-5.3", 8, 28, 2, ratio.MilliTokensRmb},
		{"bigmodel", "glm-5.3-flash", .8, 2.8, .23, ratio.MilliTokensRmb},
		{"bigmodel", "glm-5.3-flashx", 2, 7, .57, ratio.MilliTokensRmb},
		{"zai", "glm-5.3", 1.4, 4.4, .26, ratio.MilliTokensUsd},
		{"zai", "glm-5.3-flash", .15, .5, .03, ratio.MilliTokensUsd},
		{"zai", "glm-5.3-flashx", .37, 1.25, .075, ratio.MilliTokensUsd},
	} {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			cfg, ok := catalogs[tc.provider].configs[tc.model]
			require.True(t, ok)
			current := pricing.ApplyTimeWindow(cfg, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
			require.InDelta(t, tc.input*tc.unit, current.Ratio, 1e-12)
			require.InDelta(t, tc.output*tc.unit, current.Ratio*current.CompletionRatio, 1e-12)
			require.InDelta(t, tc.cache*tc.unit, current.CachedInputRatio, 1e-12)
			require.Empty(t, current.Tiers)
			for _, usage := range [][2]int{{1, 1}, {31_999, 199}, {31_999, 200}, {32_000, 200}, {900_000, 10_000}} {
				resolved := pricing.ResolveEffectivePricingForUsageFromConfig(usage[0], usage[1], current)
				require.InDelta(t, tc.input*tc.unit, resolved.InputRatio, 1e-12)
				require.InDelta(t, tc.output*tc.unit, resolved.OutputRatio, 1e-12)
			}
			if tc.model != "glm-5.3-flash" {
				require.Empty(t, cfg.TimeWindows)
			}
		})
	}
}

// TestGLM53FlashPromotionExpiry verifies that the historical promotion ends at
// the exact UTC equivalent of midnight UTC+8 without leaking between providers.
func TestGLM53FlashPromotionExpiry(t *testing.T) {
	t.Parallel()
	expiry := time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC)
	catalogs := publishedCatalogs()
	for _, tc := range []struct {
		provider             string
		input, output, cache float64
		unit                 float64
	}{
		{"bigmodel", .8, 2.8, .23, ratio.MilliTokensRmb},
		{"zai", .15, .5, .03, ratio.MilliTokensUsd},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := catalogs[tc.provider].configs["glm-5.3-flash"]
			for _, boundary := range []struct {
				name       string
				at         time.Time
				multiplier float64
			}{
				{"last_discounted_nanosecond", expiry.Add(-time.Nanosecond), .5},
				{"first_full_price_nanosecond", expiry, 1},
			} {
				t.Run(boundary.name, func(t *testing.T) {
					resolved := pricing.ApplyTimeWindow(cfg, boundary.at)
					require.InDelta(t, tc.input*tc.unit*boundary.multiplier, resolved.Ratio, 1e-12)
					require.InDelta(t, tc.output*tc.unit*boundary.multiplier, resolved.Ratio*resolved.CompletionRatio, 1e-12)
					require.InDelta(t, tc.cache*tc.unit*boundary.multiplier, resolved.CachedInputRatio, 1e-12)
				})
			}
		})
	}
}

// TestGLM53CatalogueIsolation verifies independent metadata backing slices and
// preserves the older BigModel length tiers versus Z.AI's flat USD prices.
func TestGLM53CatalogueIsolation(t *testing.T) {
	t.Parallel()
	catalogs := publishedCatalogs()
	for _, id := range []string{"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx"} {
		t.Run(id, func(t *testing.T) {
			cn, international := catalogs["bigmodel"].configs[id], catalogs["zai"].configs[id]
			require.NotEmpty(t, cn.SupportedReasoningEfforts)
			require.NotEmpty(t, international.SupportedReasoningEfforts)
			require.NotSame(t, &cn.SupportedReasoningEfforts[0], &international.SupportedReasoningEfforts[0])
			require.NotSame(t, &cn.SupportedFeatures[0], &international.SupportedFeatures[0])
			require.NotSame(t, &cn.SupportedSamplingParameters[0], &international.SupportedSamplingParameters[0])
		})
	}
	cn := catalogs["bigmodel"].configs["glm-5.1"]
	international := catalogs["zai"].configs["glm-5.1"]
	require.Len(t, cn.Tiers, 1)
	require.Empty(t, international.Tiers)
	short := pricing.ResolveEffectivePricingForUsageFromConfig(31_999, 200, cn)
	long := pricing.ResolveEffectivePricingForUsageFromConfig(32_000, 200, cn)
	require.InDelta(t, 6*ratio.MilliTokensRmb, short.InputRatio, 1e-12)
	require.InDelta(t, 24*ratio.MilliTokensRmb, short.OutputRatio, 1e-12)
	require.InDelta(t, 8*ratio.MilliTokensRmb, long.InputRatio, 1e-12)
	require.InDelta(t, 28*ratio.MilliTokensRmb, long.OutputRatio, 1e-12)
	for _, tokens := range []int{31_999, 32_000} {
		flat := pricing.ResolveEffectivePricingForUsageFromConfig(tokens, 200, international)
		require.InDelta(t, 1.4*ratio.MilliTokensUsd, flat.InputRatio, 1e-12)
		require.InDelta(t, 4.4*ratio.MilliTokensUsd, flat.OutputRatio, 1e-12)
	}
}
