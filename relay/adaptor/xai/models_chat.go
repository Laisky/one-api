package xai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// chatModels returns the complete xAI chat catalog, including retained historical IDs.
// It takes no parameters and keeps the effective definitions together by capability.
// Sources: https://docs.x.ai/developers/models and https://docs.x.ai/developers/pricing
// https://docs.x.ai/developers/grok-4-7
// https://docs.x.ai/developers/models/grok-4.5
// https://docs.x.ai/developers/models/grok-build-0.1
// https://docs.x.ai/developers/model-capabilities/text/reasoning
func chatModels() map[string]adaptor.ModelConfig {
	models := map[string]adaptor.ModelConfig{
		"grok-2-vision-1212": {
			Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 2.0, // $2.00 input, $10.00 output
			ContextLength:   32768,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesLegacy, SupportedSamplingParameters: grokSamplingParams,
			Description: "Grok 2 Vision 1212 — multimodal model with image understanding (UNVERIFIED: no longer listed in docs.x.ai/developers/models as of May 2026).",
		},
		"grok-3": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
			Description: "Grok 3 — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=none.",
		},
		"grok-3-mini": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			// xAI legacy docs published {"low","high"} for grok-3-mini; post-redirect to grok-4.3
			// the full {none,low,medium,high} set is accepted. We expose the wider set.
			SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
			Description: "Grok 3 Mini — alias for grok-4.3 post May 15, 2026 (1M context, vision). Historical reasoning_effort {low,high} is now accepted as a subset of {none,low,medium,high}.",
		},
		"grok-4-0709": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			// Retired May 15, 2026 — auto-redirects to grok-4.3 with reasoning_effort=low.
			Description: "Grok 4 (0709) — RETIRED May 15, 2026; requests auto-redirect to grok-4.3 at grok-4.3 pricing.",
		},
		"grok-4-1-fast-non-reasoning": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
			Description: "Grok 4.1 Fast (non-reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=none.",
		},
		"grok-4-1-fast-reasoning": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
			Description: "Grok 4.1 Fast (reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=low.",
		},
		"grok-4-fast-non-reasoning": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
			Description: "Grok 4 Fast (non-reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=none.",
		},
		"grok-4-fast-reasoning": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
			Description: "Grok 4 Fast (reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=low.",
		},
		"grok-4.20-0309-non-reasoning": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
			Description: "Grok 4.20 (March 9 snapshot, non-reasoning) — 1M-context model with reasoning disabled for low-latency tool calling.",
		},
		"grok-4.20-0309-reasoning": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
			Description: "Grok 4.20 (March 9 snapshot, reasoning) — 1M-context model with extended thinking and reasoning_effort {none,low,medium,high}.",
		},
		"grok-4.20-multi-agent-0309": {
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			// Multi-agent reasoning.effort governs agent count (4 on low/medium, 16 on high/xhigh), not depth.
			SupportedReasoningEfforts: grokMultiAgentEfforts, DefaultReasoningEffort: "low",
			Description: "Grok 4.20 Multi-Agent (March 9 snapshot) — 1M-context parallel-agent variant for deep research (4 agents on low/medium, 16 on high/xhigh).",
		},
		"grok-4.3": (adaptor.ModelConfig{
			Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.5, 5.0, 0.4),
			},
			ContextLength:   1000000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: []string{"none", "low", "medium", "high", "xhigh"}, DefaultReasoningEffort: "low",
			Description: "Grok 4.3 is xAI's flagship reasoning model with a 1M-token context, vision input, agentic tool calling, structured outputs, and toggleable reasoning_effort {none,low,medium,high}.",
		}).Clone(),
		"grok-4.5": (adaptor.ModelConfig{
			Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(4.0, 12.0, 0.6),
			},
			ContextLength:   500000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokXHighReasoningEfforts, DefaultReasoningEffort: "high",
			Description: "Grok 4.5 is xAI's frontier reasoning model with a 500K-token context, vision input, agentic tool calling, structured outputs, and non-disableable reasoning_effort {low,medium,high} (default high).",
		}).Clone(),
		"grok-4.5-latest": (adaptor.ModelConfig{
			Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(4.0, 12.0, 0.6),
			},
			ContextLength:   500000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokXHighReasoningEfforts, DefaultReasoningEffort: "high",
			Description: "Alias tracking the latest Grok 4.5 snapshot (same pricing and capabilities).",
		}).Clone(),
		"grok-4.6": (adaptor.ModelConfig{
			Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(4.0, 12.0, 1.0),
			},
			ContextLength:   500000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokXHighReasoningEfforts, DefaultReasoningEffort: "high",
			Description: "Grok 4.6 is xAI's current flagship for coding, agentic tasks, and knowledge work (500K context, vision, function calling, structured outputs, and configurable reasoning).",
		}).Clone(),
		"grok-4.6-latest": (adaptor.ModelConfig{
			Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(4.0, 12.0, 1.0),
			},
			ContextLength:   500000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokXHighReasoningEfforts, DefaultReasoningEffort: "high",
			Description: "Alias for the current Grok 4.6 release.",
		}).Clone(),
		"grok-4.7": {
			Ratio:            2.0 * ratio.MilliTokensUsd,
			CompletionRatio:  6.0 / 2.0,
			CachedInputRatio: 0.5 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(4.0, 12.0, 1.0),
			},
			ContextLength:               500000,
			InputModalities:             append([]string(nil), grokVisionInputs...),
			OutputModalities:            append([]string(nil), grokTextOutputs...),
			SupportedFeatures:           append([]string(nil), grokFeaturesReasoning...),
			SupportedSamplingParameters: append([]string(nil), grokReasoningSamplingParams...),
			SupportedReasoningEfforts:   append([]string(nil), grokXHighReasoningEfforts...),
			DefaultReasoningEffort:      "high",
			Description:                 "Grok 4.7 is SpaceXAI's public-API flagship for coding, agentic tasks, and knowledge work (500K context, text and image input, configurable reasoning through xhigh).",
		},
		"grok-build-0.1": {
			Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.0, 4.0, 0.4),
			},
			ContextLength:   256000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
			Description: "Grok Build 0.1 — fast agentic coding model (256K context, vision input); $1.00 in / $0.20 cached / $2.00 out. Aliases: grok-code-fast-1, grok-code-fast, grok-code-fast-1-0825.",
		},
		"grok-code-fast-1": {
			Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
			Tiers: []adaptor.ModelRatioTier{
				grokLongContextTier(2.0, 4.0, 0.4),
			},
			ContextLength:   256000,
			InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
			SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
			SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
			Description: "Grok Code Fast 1 — RETIRED May 15, 2026; auto-redirects to grok-build-0.1 ($1.00 in / $0.20 cached / $2.00 out, 256K context).",
		},
	}

	// Official aliases own deep copies of the canonical configuration.
	// Source: https://docs.x.ai/developers/models (verified 2026-09-21).
	for alias, canonical := range map[string]string{
		"grok-4.20":                                        "grok-4.20-0309-reasoning",
		"grok-4.20-0309":                                   "grok-4.20-0309-reasoning",
		"grok-4.20-beta":                                   "grok-4.20-0309-reasoning",
		"grok-4.20-beta-0309":                              "grok-4.20-0309-reasoning",
		"grok-4.20-beta-0309-non-reasoning":                "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-0309-reasoning":                    "grok-4.20-0309-reasoning",
		"grok-4.20-beta-latest":                            "grok-4.20-0309-reasoning",
		"grok-4.20-beta-latest-non-reasoning":              "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-latest-reasoning":                  "grok-4.20-0309-reasoning",
		"grok-4.20-beta-non-reasoning":                     "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-reasoning":                         "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-0304":                 "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-0304-non-reasoning":   "grok-4.20-0309-non-reasoning",
		"grok-4.20-experimental-beta-0304-reasoning":       "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-latest":               "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-non-reasoning-latest": "grok-4.20-0309-non-reasoning",
		"grok-4.20-experimental-beta-reasoning-latest":     "grok-4.20-0309-reasoning",
		"grok-4.20-multi-agent":                            "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-beta-0309":                  "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-beta-latest":                "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-experimental-beta-0304":     "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-experimental-beta-latest":   "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-latest":                     "grok-4.20-multi-agent-0309",
		"grok-4.20-non-reasoning":                          "grok-4.20-0309-non-reasoning",
		"grok-4.20-non-reasoning-gv2":                      "grok-4.20-0309-non-reasoning",
		"grok-4.20-non-reasoning-latest":                   "grok-4.20-0309-non-reasoning",
		"grok-4.20-reasoning":                              "grok-4.20-0309-reasoning",
		"grok-4.20-reasoning-gv2":                          "grok-4.20-0309-reasoning",
		"grok-4.20-reasoning-latest":                       "grok-4.20-0309-reasoning",
		"grok-4.3-latest":                                  "grok-4.3",
		"grok-build-latest":                                "grok-4.5",
		"grok-code-fast":                                   "grok-build-0.1",
		"grok-code-fast-1-0825":                            "grok-build-0.1",
	} {
		cfg, ok := models[canonical]
		if !ok {
			panic("xai: missing canonical model " + canonical)
		}
		if _, duplicate := models[alias]; duplicate {
			panic("xai: duplicate model alias " + alias)
		}
		cfg = cfg.Clone()
		cfg.Description = "Official SpaceXAI alias for " + canonical + "."
		models[alias] = cfg
	}
	return models
}
