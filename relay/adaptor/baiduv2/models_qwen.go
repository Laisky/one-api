package baiduv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenModels returns independently owned hosted Qwen defaults.
// It takes no arguments and returns the complete family catalog.
// Sources checked 2026-10-12 and 2026-11-16:
// https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j
// https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
func qwenModels() map[string]adaptor.ModelConfig {
	models := make(map[string]adaptor.ModelConfig)
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
