package xai

import (
	"slices"

	"github.com/Laisky/one-api/relay/adaptor"
	ratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// This file contains catalog changes published after the main table was last
// refreshed. Keeping the changes in one place makes the upstream evidence and
// compatibility aliases easy to review without deleting legacy slugs.
//
// Official sources (last verified 2026-09-21):
//   - https://docs.x.ai/developers/grok-4-7
//   - https://docs.x.ai/developers/models
//   - https://docs.x.ai/developers/pricing
//   - https://docs.x.ai/developers/model-capabilities/text/reasoning
//   - https://docs.x.ai/developers/models/grok-4.3
//   - https://docs.x.ai/developers/models/grok-4.20
//   - https://docs.x.ai/developers/models/grok-4.20-non-reasoning
//   - https://docs.x.ai/developers/models/grok-4.20-multi-agent-0309
//   - https://docs.x.ai/developers/models/grok-build-0.1
//   - https://docs.x.ai/developers/models/grok-imagine-image-2.0
//   - https://docs.x.ai/developers/models/grok-imagine-video-1.5-preview
//   - https://docs.x.ai/developers/migration/imagine-image-quality-nov-2

var grokXHighReasoningEfforts = []string{"low", "medium", "high", "xhigh"}

func init() {
	refreshCatalog20260921()
	// ModelList is initialized from ModelRatios in constants.go before init
	// functions run, so rebuild it after adding the newly published entries.
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}

func refreshCatalog20260921() {
	ModelRatios["grok-4.7"] = adaptor.ModelConfig{
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
	}

	// Grok 4.6 and Grok 4.3 explicitly advertise xhigh. Grok 4.5
	// accepts xhigh as an alias for high, so keep it in the accepted-value list.
	setReasoningEfforts([]string{"grok-4.6", "grok-4.6-latest"}, grokXHighReasoningEfforts)
	setReasoningEfforts([]string{"grok-4.5", "grok-4.5-latest"}, grokXHighReasoningEfforts)
	setReasoningEfforts([]string{"grok-4.3"}, []string{"none", "low", "medium", "high", "xhigh"})

	addOfficialAliases(map[string]string{
		"grok-build-latest":                                "grok-4.5",
		"grok-4.3-latest":                                  "grok-4.3",
		"grok-code-fast":                                   "grok-build-0.1",
		"grok-code-fast-1-0825":                            "grok-build-0.1",
		"grok-4.20-reasoning-latest":                       "grok-4.20-0309-reasoning",
		"grok-4.20":                                        "grok-4.20-0309-reasoning",
		"grok-4.20-reasoning":                              "grok-4.20-0309-reasoning",
		"grok-4.20-0309":                                   "grok-4.20-0309-reasoning",
		"grok-4.20-beta-0309-reasoning":                    "grok-4.20-0309-reasoning",
		"grok-4.20-beta":                                   "grok-4.20-0309-reasoning",
		"grok-4.20-beta-0309":                              "grok-4.20-0309-reasoning",
		"grok-4.20-beta-latest":                            "grok-4.20-0309-reasoning",
		"grok-4.20-beta-latest-reasoning":                  "grok-4.20-0309-reasoning",
		"grok-4.20-beta-reasoning":                         "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-0304-reasoning":       "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-0304":                 "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-reasoning-latest":     "grok-4.20-0309-reasoning",
		"grok-4.20-experimental-beta-latest":               "grok-4.20-0309-reasoning",
		"grok-4.20-reasoning-gv2":                          "grok-4.20-0309-reasoning",
		"grok-4.20-non-reasoning":                          "grok-4.20-0309-non-reasoning",
		"grok-4.20-non-reasoning-latest":                   "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-non-reasoning":                     "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-latest-non-reasoning":              "grok-4.20-0309-non-reasoning",
		"grok-4.20-experimental-beta-0304-non-reasoning":   "grok-4.20-0309-non-reasoning",
		"grok-4.20-experimental-beta-non-reasoning-latest": "grok-4.20-0309-non-reasoning",
		"grok-4.20-beta-0309-non-reasoning":                "grok-4.20-0309-non-reasoning",
		"grok-4.20-non-reasoning-gv2":                      "grok-4.20-0309-non-reasoning",
		"grok-4.20-multi-agent":                            "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-latest":                     "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-beta-latest":                "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-experimental-beta-0304":     "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-experimental-beta-latest":   "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent-beta-0309":                  "grok-4.20-multi-agent-0309",
	})

	refreshImagineImageMetadata()
	refreshImagineVideoMetadata()
}

func setReasoningEfforts(models []string, efforts []string) {
	for _, name := range models {
		cfg, ok := ModelRatios[name]
		if !ok {
			continue
		}
		cfg = cfg.Clone()
		cfg.SupportedReasoningEfforts = append([]string(nil), efforts...)
		ModelRatios[name] = cfg
	}
}

func addOfficialAliases(aliases map[string]string) {
	for alias, canonical := range aliases {
		cfg, ok := ModelRatios[canonical]
		if !ok {
			continue
		}
		cfg = cfg.Clone()
		cfg.Description = "Official SpaceXAI alias for " + canonical + "."
		ModelRatios[alias] = cfg
	}
}

func refreshImagineImageMetadata() {
	for _, name := range []string{"grok-imagine-image-2.0"} {
		cfg, ok := ModelRatios[name]
		if !ok || cfg.Image == nil {
			continue
		}
		cfg = cfg.Clone()
		cfg.Image.DefaultQuality = "auto"
		cfg.Image.QualitySizeMultipliers = map[string]map[string]float64{
			"low": {
				"1024x1024": 1.00,
				"2048x2048": 1.50,
			},
			// For image generation, auto currently resolves to low. Image editing
			// resolves auto to medium upstream; one-api's shared metadata cannot
			// express endpoint-specific defaults, so explicit medium remains below.
			"auto": {
				"1024x1024": 1.00,
				"2048x2048": 1.50,
			},
			"medium": {
				"1024x1024": 1.50,
				"2048x2048": 2.00,
			},
		}
		cfg.Description = "Grok Imagine Image 2.0 supports text/image input and low, medium, or auto quality. Official output pricing spans 1K, 1.5K, and 2K tiers ($0.04-$0.08 per image); one-api currently exposes the upstream 1k and 2k resolution values."
		ModelRatios[name] = cfg
	}

	for _, name := range []string{"grok-imagine-image-quality", "grok-imagine-image-quality-20260403", "grok-imagine-image-quality-latest", "grok-imagine-image-pro"} {
		cfg, ok := ModelRatios[name]
		if !ok || cfg.Image == nil {
			continue
		}
		cfg = cfg.Clone()
		cfg.Image.SizeMultipliers = map[string]float64{
			"1024x1024": 1.0,
			"1408x1408": 1.2,
			"2048x2048": 1.4,
		}
		cfg.Description = "Legacy Imagine quality slug: $0.05/$0.06/$0.07 per image at 1K/1.5K/2K. It is scheduled to redirect to grok-imagine-image-2.0 at low quality on November 2, 2026."
		ModelRatios[name] = cfg
	}

	for _, name := range []string{"grok-imagine-image", "grok-imagine-image-2026-03-02"} {
		cfg, ok := ModelRatios[name]
		if !ok || cfg.Image == nil {
			continue
		}
		cfg = cfg.Clone()
		cfg.Image.SizeMultipliers = map[string]float64{
			"1024x1024": 1.0,
			"2048x2048": 1.0,
		}
		ModelRatios[name] = cfg
	}
}

func refreshImagineVideoMetadata() {
	for _, name := range []string{"grok-imagine-video-1.5", "grok-imagine-video-1.5-preview", "grok-imagine-video-1.5-2026-05-30"} {
		cfg, ok := ModelRatios[name]
		if !ok {
			continue
		}
		cfg = cfg.Clone()
		cfg.InputModalities = []string{"text", "image", "audio"}
		cfg.OutputModalities = []string{"video"}
		cfg.Description = "Grok Imagine Video 1.5 accepts text, image, and preset-voice audio references and generates 480p, 720p, or 1080p video at $0.08/$0.14/$0.25 per second."
		ModelRatios[name] = cfg
	}

	for _, name := range []string{"grok-imagine-video", "grok-imagine-video-2026-01-20"} {
		cfg, ok := ModelRatios[name]
		if !ok {
			continue
		}
		cfg = cfg.Clone()
		cfg.InputModalities = []string{"text", "image", "video"}
		cfg.OutputModalities = []string{"video"}
		ModelRatios[name] = cfg
	}
}

func supportsSamplingParameter(parameters []string, name string) bool {
	return slices.Contains(parameters, name)
}

func isLegacyPenaltyRestrictedModel(model string) bool {
	switch model {
	case "grok-4-0709",
		"grok-4-1-fast-reasoning",
		"grok-4-1-fast-non-reasoning",
		"grok-4-fast-reasoning",
		"grok-4-fast-non-reasoning",
		"grok-code-fast-1":
		return true
	default:
		return false
	}
}
