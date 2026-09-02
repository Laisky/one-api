package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
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
		assert.True(t, modelSet[modelName], "GetModelListFromPricing must include %q", modelName)
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

		assert.InDeltaf(t, expected.ratio*ratio.MilliTokensUsd, cfg.Ratio, 1e-12, "%s input ratio", name)
		assert.InDeltaf(t, expected.completionRatio, cfg.CompletionRatio, 1e-9, "%s completion ratio", name)
		assert.InDeltaf(t, expected.cachedInputRatio*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12, "%s cached input ratio", name)
		assert.InDeltaf(t, expected.cacheWriteRatio*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12, "%s cache write ratio", name)
		assert.Zerof(t, cfg.CacheWrite1hRatio, "%s must not advertise an unsupported 1h cache write price", name)
		assert.Equalf(t, expected.contextLength, cfg.ContextLength, "%s context length", name)
		assert.Equalf(t, int32(128_000), cfg.MaxOutputTokens, "%s max output tokens", name)
		assert.Equalf(t, []string{"text", "image"}, cfg.InputModalities, "%s input modalities", name)

		require.Lenf(t, cfg.Tiers, 1, "%s must have a single long-context tier", name)
		tier := cfg.Tiers[0]
		assert.Equalf(t, 272_001, tier.InputTokenThreshold, "%s long-context threshold", name)
		assert.InDeltaf(t, expected.tierRatio*ratio.MilliTokensUsd, tier.Ratio, 1e-12, "%s long-context input ratio", name)
		assert.InDeltaf(t, expected.tierCompletion, tier.CompletionRatio, 1e-9, "%s long-context completion ratio", name)
		assert.InDeltaf(t, expected.tierCached*ratio.MilliTokensUsd, tier.CachedInputRatio, 1e-12, "%s long-context cached input ratio", name)
		assert.InDeltaf(t, expected.tierCacheWrite*ratio.MilliTokensUsd, tier.CacheWrite5mRatio, 1e-12, "%s long-context cache write ratio", name)
		assert.Zerof(t, tier.CacheWrite1hRatio, "%s long-context tier must not advertise an unsupported 1h cache write price", name)
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
		assert.Truef(t, isModelSupportedReasoning(name), "%s must be recognized as a reasoning model", name)
		assert.Truef(t, isReasoningEffortAllowedForModel(name, "max"), "%s must allow the 'max' effort", name)
		assert.Truef(t, isReasoningEffortAllowedForModel(name, "xhigh"), "%s must allow the 'xhigh' effort", name)
		assert.Truef(t, isReasoningEffortAllowedForModel(name, "none"), "%s must allow the canonical 'none' effort", name)
		assert.Falsef(t, isReasoningEffortAllowedForModel(name, "minimal"), "%s metadata must not advertise the legacy 'minimal' spelling", name)
		assert.Falsef(t, isReasoningEffortAllowedForModel(name, "ultra"), "%s must reject an unknown effort", name)
		assert.Equalf(t, "medium", defaultReasoningEffortForModel(name), "%s default effort", name)
		assert.Falsef(t, isMediumOnlyReasoningModel(name), "%s is not a medium-only model", name)

		requested := "max"
		assert.Equalf(t, "max", *normalizeReasoningEffortForModel(name, &requested), "%s must pass 'max' through", name)
		legacy := "minimal"
		assert.Equalf(t, "none", *normalizeReasoningEffortForModel(name, &legacy), "%s must canonicalize legacy 'minimal' to 'none'", name)
		junk := "ultra"
		assert.Equalf(t, "medium", *normalizeReasoningEffortForModel(name, &junk), "%s must coerce an invalid effort to the default", name)
	}
}
