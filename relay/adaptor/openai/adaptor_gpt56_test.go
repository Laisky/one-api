package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ---------------------------------------------------------------------------
// GPT-5.6 family: presence in the pricing-derived model list
// ---------------------------------------------------------------------------

func TestGetModelListFromPricingIncludesGPT56(t *testing.T) {
	t.Parallel()

	modelList := adaptor.GetModelListFromPricing(ModelRatios)

	gpt56Models := []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}
	modelSet := make(map[string]bool, len(modelList))
	for _, m := range modelList {
		modelSet[m] = true
	}

	for _, modelName := range gpt56Models {
		assert.True(t, modelSet[modelName], "GetModelListFromPricing must include %q", modelName)
	}
}

// ---------------------------------------------------------------------------
// GPT-5.6 family: pricing, context window, and long-context tiers
// ---------------------------------------------------------------------------

func TestGPT56Pricing(t *testing.T) {
	t.Parallel()

	type expectation struct {
		ratio            float64 // input $/1M
		completionRatio  float64 // output/input multiplier
		cachedInputRatio float64 // cached input $/1M
		cacheWriteRatio  float64 // cache write $/1M
		tierRatio        float64 // >272K input $/1M
		tierCompletion   float64 // >272K output/input multiplier
		tierCached       float64 // >272K cached input $/1M
		tierCacheWrite   float64 // >272K cache write $/1M
	}

	// gpt-5.6 is the alias for gpt-5.6-sol and must price identically.
	expectations := map[string]expectation{
		"gpt-5.6":       {5.0, 6.0, 0.5, 6.25, 10.0, 4.5, 1.0, 12.5},
		"gpt-5.6-sol":   {5.0, 6.0, 0.5, 6.25, 10.0, 4.5, 1.0, 12.5},
		"gpt-5.6-terra": {2.0, 6.0, 0.2, 2.5, 4.0, 4.5, 0.4, 5.0},
		"gpt-5.6-luna":  {0.2, 6.0, 0.02, 0.25, 0.4, 4.5, 0.04, 0.5},
	}

	for name, exp := range expectations {
		cfg, ok := ModelRatios[name]
		require.Truef(t, ok, "ModelRatios must contain %q", name)

		assert.InDeltaf(t, exp.ratio*ratio.MilliTokensUsd, cfg.Ratio, 1e-12,
			"%s input ratio", name)
		assert.InDeltaf(t, exp.completionRatio, cfg.CompletionRatio, 1e-9,
			"%s completion ratio", name)
		assert.InDeltaf(t, exp.cachedInputRatio*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12,
			"%s cached input ratio", name)
		assert.InDeltaf(t, exp.cacheWriteRatio*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12,
			"%s cache write ratio", name)
		assert.Zerof(t, cfg.CacheWrite1hRatio, "%s must not advertise an unsupported 1h cache write price", name)

		assert.Equalf(t, int32(1_050_000), cfg.ContextLength, "%s context length", name)
		assert.Equalf(t, int32(128000), cfg.MaxOutputTokens, "%s max output tokens", name)
		assert.Equalf(t, []string{"text", "image"}, cfg.InputModalities, "%s input modalities", name)

		require.Lenf(t, cfg.Tiers, 1, "%s must have a single long-context tier", name)
		tier := cfg.Tiers[0]
		assert.Equalf(t, 272_001, tier.InputTokenThreshold,
			"%s long-context threshold", name)
		assert.InDeltaf(t, exp.tierRatio*ratio.MilliTokensUsd, tier.Ratio, 1e-12,
			"%s long-context input ratio", name)
		assert.InDeltaf(t, exp.tierCompletion, tier.CompletionRatio, 1e-9,
			"%s long-context completion ratio", name)
		assert.InDeltaf(t, exp.tierCached*ratio.MilliTokensUsd, tier.CachedInputRatio, 1e-12,
			"%s long-context cached input ratio", name)
		assert.InDeltaf(t, exp.tierCacheWrite*ratio.MilliTokensUsd, tier.CacheWrite5mRatio, 1e-12,
			"%s long-context cache write ratio", name)
		assert.Zerof(t, tier.CacheWrite1hRatio, "%s long-context tier must not advertise an unsupported 1h cache write price", name)
	}
}

// ---------------------------------------------------------------------------
// GPT-5.6 family: reasoning-effort handling incl. the new "max" level
// ---------------------------------------------------------------------------

func TestGPT56ReasoningEfforts(t *testing.T) {
	t.Parallel()

	models := []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}

	for _, name := range models {
		// "max" is the level introduced with GPT-5.6 and must be accepted.
		assert.Truef(t, isReasoningEffortAllowedForModel(name, "max"),
			"%s must allow the 'max' effort", name)
		assert.Truef(t, isReasoningEffortAllowedForModel(name, "xhigh"),
			"%s must allow the 'xhigh' effort", name)
		assert.Truef(t, isReasoningEffortAllowedForModel(name, "minimal"),
			"%s must allow the legacy 'minimal' effort", name)
		assert.Falsef(t, isReasoningEffortAllowedForModel(name, "ultra"),
			"%s must reject an unknown effort", name)

		assert.Equalf(t, "medium", defaultReasoningEffortForModel(name),
			"%s default effort", name)
		assert.Falsef(t, isMediumOnlyReasoningModel(name),
			"%s is not a medium-only model", name)

		// The body path honors an explicit "max"/"xhigh" and coerces junk to default.
		requested := "max"
		assert.Equalf(t, "max", *normalizeReasoningEffortForModel(name, &requested),
			"%s must pass 'max' through", name)
		junk := "ultra"
		assert.Equalf(t, "medium", *normalizeReasoningEffortForModel(name, &junk),
			"%s must coerce an invalid effort to the default", name)
	}
}
