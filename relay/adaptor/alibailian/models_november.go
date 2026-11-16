package alibailian

import "github.com/Laisky/one-api/relay/adaptor"

// novemberModels returns independently owned Beijing defaults for new compatible
// API model IDs. It takes no arguments and returns a new map.
// Sources checked 2026-11-16:
// https://help.aliyun.com/zh/model-studio/model-pricing
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/qwen4-max
// https://help.aliyun.com/zh/model-studio/kimi-k3-1-pro
// https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions
func novemberModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen4-max": {
			Ratio: nativeRate(10), CompletionRatio: 3, CachedInputRatio: nativeRate(.5),
			ContextLength: 1_000_000, MaxOutputTokens: 262_144,
			InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description:                 "Qwen4 Max through Alibaba's compatible API, Beijing tariff. Maximum combined reasoning/output is 262144 tokens; non-thinking output is limited to 131072. The gateway does not currently forward this model's reasoning_effort. Published explicit cache creation is CNY 12.5/20 per million tokens for 5m/1h, but TTL-specific usage normalization remains unqualified and is not advertised as a gateway capability.",
		},
		"MiniMax/MiniMax-M3.1-Flash": {
			Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.1),
			ContextLength:   1_000_000,
			InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "MiniMax M3.1 Flash supplied through Alibaba Model Studio. Beijing CNY 1 input, 0.1 cached input and 4 output per million tokens. Sampling is fixed upstream; adjustable effort is not advertised because the gateway's compatible converter does not forward it for this ID. Hosted output limits are not copied from MiniMax's direct API.",
		},
		"kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			ContextLength: 1_048_576, MaxOutputTokens: 1_048_576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Alibaba-deployed Kimi K3.1 Pro, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Supports text and image, not video. Distinct from Moonshot-supplied kimi/kimi-k3.1-pro; the gateway does not forward this model's upstream effort controls.",
		},
		"kimi/kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro through Alibaba, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Hosted control vocabulary and limits are not copied from Moonshot's direct API; adjustable effort is not forwarded by the current gateway.",
		},
		"kimi/kimi-k3.1-pro-highspeed": {
			Ratio: nativeRate(26), CompletionRatio: 136.0 / 26, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro Highspeed through Alibaba, Beijing CNY 26 input, 1.3 cached input and 136 output per million tokens. The 5% cached-input tariff does not double with input/output. Adjustable effort is not forwarded by the current gateway.",
		},
		"glm-5.3-prime": {
			Ratio: nativeRate(16), CompletionRatio: 3.5, CachedInputRatio: nativeRate(4),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			Description: "Alibaba-deployed GLM 5.3 Prime, Beijing CNY 16 input, 4 cached input and 56 output per million tokens. Other deployment regions have different prices and cache fractions. Provider-specific limits and control vocabularies remain unspecified.",
		},
		"xiaomi/mimo-v2.6-pro-ultraspeed": {
			Ratio: nativeRate(30), CompletionRatio: 2,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			Description: "Xiaomi-supplied MiMo V2.6 Pro Ultraspeed through Alibaba, Beijing CNY 30 input and 60 output per million tokens up to 1M input. Cache price and generation limits remain unspecified; they are not inferred from Xiaomi's direct API.",
		},
		"deepseek-v4.1-flash-1017": {
			Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.02),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
			TimeWindows: []adaptor.TimeWindow{{
				Name: "beijing-peak", TimeZone: "Asia/Shanghai",
				Ranges:  []adaptor.ClockRange{{Start: "08:00", End: "22:00"}},
				Overlay: adaptor.ModelConfig{Ratio: nativeRate(2), CompletionRatio: 4, CachedInputRatio: nativeRate(.04)},
			}},
			Description: "Alibaba-deployed DeepSeek V4.1 Flash 1017, Beijing tariff. CNY/M input/cache/output is 2/0.04/8 from 08:00 to 22:00 Asia/Shanghai, otherwise 1/0.02/4. Direct DeepSeek USD tariffs, holidays and tool protocols do not apply.",
		},
	}
}
