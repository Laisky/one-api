package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestGetModelListFromPricingIncludesGPT56 verifies all current GPT-5.6 and Daybreak model IDs are discoverable.
func TestGetModelListFromPricingIncludesGPT56(t *testing.T) {
	t.Parallel()

	modelList := adaptor.GetModelListFromPricing(ModelRatios)
	gpt56Models := []string{
		"gpt-5.6",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.6-cyber",
		"gpt-daybreak-red-latest",
		"gpt-daybreak-blue-latest",
	}
	modelSet := make(map[string]bool, len(modelList))
	for _, modelName := range modelList {
		modelSet[modelName] = true
	}

	for _, modelName := range gpt56Models {
		require.True(t, modelSet[modelName], "GetModelListFromPricing must include %q", modelName)
	}
}

// TestGPT56Pricing verifies base, cache-write, and long-context pricing for GPT-5.6 and Daybreak aliases.
func TestGPT56Pricing(t *testing.T) {
	t.Parallel()

	type expectation struct {
		ratio            float64
		completionRatio  float64
		cachedInputRatio float64
		cacheWriteRatio  float64
		tierRatio        float64
		tierCompletion   float64
		tierCached       float64
		tierCacheWrite   float64
		contextLength    int32
	}

	expectations := map[string]expectation{
		"gpt-5.6":                  {4.0, 5.0, 0.4, 5.0, 8.0, 3.75, 0.8, 10.0, 1_050_000},
		"gpt-5.6-sol":              {4.0, 5.0, 0.4, 5.0, 8.0, 3.75, 0.8, 10.0, 1_050_000},
		"gpt-5.6-terra":            {2.0, 6.0, 0.2, 2.5, 4.0, 4.5, 0.4, 5.0, 1_050_000},
		"gpt-5.6-luna":             {0.2, 6.0, 0.02, 0.25, 0.4, 4.5, 0.04, 0.5, 1_050_000},
		"gpt-5.6-cyber":            {12.5, 6.0, 1.25, 15.625, 25.0, 4.5, 2.5, 31.25, 400_000},
		"gpt-daybreak-red-latest":  {12.5, 6.0, 1.25, 15.625, 25.0, 4.5, 2.5, 31.25, 400_000},
		"gpt-daybreak-blue-latest": {4.0, 5.0, 0.4, 5.0, 8.0, 3.75, 0.8, 10.0, 1_050_000},
	}

	for name, expected := range expectations {
		cfg, ok := ModelRatios[name]
		require.Truef(t, ok, "ModelRatios must contain %q", name)

		require.InDeltaf(t, expected.ratio*ratio.MilliTokensUsd, cfg.Ratio, 1e-12, "%s input ratio", name)
		require.InDeltaf(t, expected.completionRatio, cfg.CompletionRatio, 1e-9, "%s completion ratio", name)
		require.InDeltaf(t, expected.cachedInputRatio*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12, "%s cached input ratio", name)
		require.InDeltaf(t, expected.cacheWriteRatio*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12, "%s cache write ratio", name)
		require.Zerof(t, cfg.CacheWrite1hRatio, "%s must not advertise an unsupported 1h cache write price", name)
		require.Equalf(t, expected.contextLength, cfg.ContextLength, "%s context length", name)
		require.Equalf(t, int32(128_000), cfg.MaxOutputTokens, "%s max output tokens", name)
		require.Equalf(t, []string{"text", "image"}, cfg.InputModalities, "%s input modalities", name)

		require.Lenf(t, cfg.Tiers, 1, "%s must have a single long-context tier", name)
		tier := cfg.Tiers[0]
		require.Equalf(t, 272_001, tier.InputTokenThreshold, "%s long-context threshold", name)
		require.InDeltaf(t, expected.tierRatio*ratio.MilliTokensUsd, tier.Ratio, 1e-12, "%s long-context input ratio", name)
		require.InDeltaf(t, expected.tierCompletion, tier.CompletionRatio, 1e-9, "%s long-context completion ratio", name)
		require.InDeltaf(t, expected.tierCached*ratio.MilliTokensUsd, tier.CachedInputRatio, 1e-12, "%s long-context cached input ratio", name)
		require.InDeltaf(t, expected.tierCacheWrite*ratio.MilliTokensUsd, tier.CacheWrite5mRatio, 1e-12, "%s long-context cache write ratio", name)
		require.Zerof(t, tier.CacheWrite1hRatio, "%s long-context tier must not advertise an unsupported 1h cache write price", name)
	}
}

// TestGPT56ReasoningEfforts verifies canonical GPT-5.6 effort metadata and legacy request normalization.
func TestGPT56ReasoningEfforts(t *testing.T) {
	t.Parallel()

	models := []string{
		"gpt-5.6",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.6-cyber",
		"gpt-daybreak-red-latest",
		"gpt-daybreak-blue-latest",
	}

	for _, name := range models {
		cfg, ok := ModelRatios[name]
		require.Truef(t, ok, "ModelRatios must contain %q", name)
		require.Equalf(t, gpt56CanonicalReasoningEfforts, cfg.SupportedReasoningEfforts, "%s canonical efforts", name)
		require.Truef(t, isModelSupportedReasoning(name), "%s must be recognized as a reasoning model", name)
		require.Truef(t, isReasoningEffortAllowedForModel(name, "max"), "%s must allow the 'max' effort", name)
		require.Truef(t, isReasoningEffortAllowedForModel(name, "xhigh"), "%s must allow the 'xhigh' effort", name)
		require.Truef(t, isReasoningEffortAllowedForModel(name, "none"), "%s must allow the canonical 'none' effort", name)
		require.Truef(t, isReasoningEffortAllowedForModel(name, "minimal"), "%s must accept the legacy 'minimal' alias", name)
		require.Falsef(t, isReasoningEffortAllowedForModel(name, "ultra"), "%s must reject an unknown effort", name)
		require.Equalf(t, "medium", defaultReasoningEffortForModel(name), "%s default effort", name)
		require.Falsef(t, isMediumOnlyReasoningModel(name), "%s is not a medium-only model", name)

		requested := "max"
		require.Equalf(t, "max", *normalizeReasoningEffortForModel(name, &requested), "%s must pass 'max' through", name)
		legacy := "minimal"
		require.Equalf(t, "none", *normalizeReasoningEffortForModel(name, &legacy), "%s must canonicalize legacy 'minimal' to 'none'", name)
		junk := "ultra"
		require.Equalf(t, "medium", *normalizeReasoningEffortForModel(name, &junk), "%s must coerce an invalid effort to the default", name)
	}

	legacy := "minimal"
	require.True(t, isModelSupportedReasoning(" GPT-5.6 "))
	require.True(t, isReasoningEffortAllowedForModel(" GPT-5.6 ", "max"))
	require.Equal(t, "none", *normalizeReasoningEffortForModel(" GPT-5.6 ", &legacy))
}

// TestGlobalFallbackPricingExcludesNativeSolSchedule verifies the explicit
// catalog boundaries: only an adaptor bound to channeltype.OpenAI bills
// GPT-5.6 Sol with its December schedule; an unbound adaptor (an embedder such
// as Azure, or a catalog sweep) prices the shared catalog; and every OpenAI
// adaptor contributes the unscheduled shared catalog to the global fallback
// layer, while a compatible channel contributes its own table. Parameter t
// reports a leaked or missing schedule.
func TestGlobalFallbackPricingExcludesNativeSolSchedule(t *testing.T) {
	t.Parallel()

	native := &Adaptor{ChannelType: channeltype.OpenAI}
	unbound := &Adaptor{}
	for _, name := range []string{"gpt-5.6-sol", "gpt-5.6"} {
		require.NotEmpty(t, native.GetDefaultModelPricing()[name].TimeWindows, "native billing keeps the schedule for %s", name)
		require.Empty(t, unbound.GetDefaultModelPricing()[name].TimeWindows, "an unbound adaptor is no evidence of native OpenAI for %s", name)
		require.Equal(t, ModelRatios[name], unbound.GetDefaultModelPricing()[name], name)
		for _, provider := range []*Adaptor{native, unbound} {
			shared, ok := provider.GetGlobalFallbackModelPricing()[name]
			require.True(t, ok, name)
			require.Empty(t, shared.TimeWindows, "the global contribution carries no native schedule for %s", name)
			require.Equal(t, ModelRatios[name].Ratio, shared.Ratio, name)
		}
	}
	require.Equal(t, len(ModelRatios), len(native.GetGlobalFallbackModelPricing()))
	groq := &Adaptor{ChannelType: channeltype.Groq}
	require.Equal(t, len(groq.GetDefaultModelPricing()), len(groq.GetGlobalFallbackModelPricing()))
	var _ adaptor.GlobalPricingContributor = (*Adaptor)(nil)
}
