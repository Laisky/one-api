package baiduv2

import "github.com/Laisky/one-api/relay/adaptor"

// publishedChatModels returns independent configurations for the exact model
// parameters in Qianfan's current catalogue. It accepts nothing and returns a
// new map. Prices are CNY per million tokens, not model-creator USD tariffs.
// Sources verified 2026-10-12:
// https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j
// https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
func publishedChatModels() map[string]adaptor.ModelConfig {
	models := map[string]adaptor.ModelConfig{
		"glm-5.3-flash": {
			Ratio: nativeRate(.8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(.23),
			ContextLength: 1000000, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "GLM-5.3-Flash on Qianfan v2. Published lowercase API ID; 1M context and 131072 total output tokens including reasoning. CNY 0.80 input, 0.23 cached input and 2.80 output per million tokens. No Z.AI-specific reasoning-effort, vision or tool controls are inferred.",
		},
		"glm-5.3": {
			Ratio: nativeRate(8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(2),
			ContextLength: 1000000, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "GLM-5.3 on Qianfan v2 using the documented lowercase model parameter. 1M context and 131072 total output tokens. Current Standard CNY 8 input, 2 cached input and 28 output per million; the earlier holiday promotion is over. The retained uppercase entry is historical compatibility metadata, not an automatic request rewrite.",
		},
		"glm-5.2": {
			Ratio: nativeRate(8), CompletionRatio: 3.5, CachedInputRatio: nativeRate(2),
			ContextLength: 1000000, MaxOutputTokens: 131072,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "GLM-5.2 on Qianfan v2 using the exact lowercase API ID. 1M context, 131072 total output tokens, CNY 8 input, 2 cached input and 28 output per million. Host-specific tool and effort vocabularies are not inferred from another provider.",
		},
	}
	for _, spec := range []struct {
		id                                   string
		input, output, longInput, longOutput float64
	}{
		{"qwen3.5-397b-a17b", 1.2, 7.2, 3, 18}, {"qwen3.5-122b-a10b", .8, 6.4, 2, 16},
		{"qwen3.5-27b", .6, 4.8, 1.8, 14.4}, {"qwen3.5-35b-a3b", .4, 3.2, 1.6, 12.8},
	} {
		models[spec.id] = adaptor.ModelConfig{
			Ratio: nativeRate(spec.input), CompletionRatio: spec.output / spec.input,
			Tiers:         []adaptor.ModelRatioTier{{InputTokenThreshold: 128001, Ratio: nativeRate(spec.longInput), CompletionRatio: spec.longOutput / spec.longInput}},
			ContextLength: 256000, MaxOutputTokens: 65536,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Qwen3.5 on Qianfan v2 using the published lowercase API ID. Host CNY tariff increases above 128K input; K is normalized to 1000 in these advisory defaults. Published output ceiling is 65536 including reasoning. No inherited Alibaba cache, vision, effort or tool policy is implied.",
		}
	}
	return models
}
