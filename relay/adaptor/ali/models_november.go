package ali

import "github.com/Laisky/one-api/relay/adaptor"

// novemberModels returns independently owned Beijing defaults for newly
// published DashScope model IDs. It takes no arguments and returns a new map.
// Sources checked 2026-11-16:
// https://help.aliyun.com/zh/model-studio/model-pricing
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/qwen4-max
// https://help.aliyun.com/zh/model-studio/kimi-k3-1-pro
// https://help.aliyun.com/zh/model-studio/qwen-api-via-dashscope
func novemberModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen4-max": {
			Ratio: nativeRate(10), CompletionRatio: 3, CachedInputRatio: nativeRate(.5),
			ContextLength: 1_000_000, MaxOutputTokens: 262_144,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Qwen4 Max on DashScope, Beijing tariff. Upstream supports 262144 combined reasoning/output tokens, or 131072 output tokens without thinking. Native Ali currently forwards text messages only and does not forward reasoning_effort, explicit cache blocks or reasoning history. Published cache creation prices are CNY 12.5/20 per million tokens for 5m/1h; these unsupported native write modes are not enabled by this entry.",
		},
		"MiniMax/MiniMax-M3.1-Flash": {
			Ratio: nativeRate(1), CompletionRatio: 4, CachedInputRatio: nativeRate(.1),
			ContextLength:   1_000_000,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "MiniMax M3.1 Flash supplied through Alibaba Model Studio. Beijing CNY 1 input, 0.1 cached input and 4 output per million tokens. Native Ali forwards text only; its converter does not expose this model's upstream effort controls. Hosted output limits are not inferred from MiniMax's direct API.",
		},
		"kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			ContextLength: 1_048_576, MaxOutputTokens: 1_048_576,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Alibaba-deployed Kimi K3.1 Pro, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Upstream supports text and image input; native Ali forwards text only and does not forward the thinking switch or effort. This ID is distinct from Moonshot-supplied kimi/kimi-k3.1-pro.",
		},
		"kimi/kimi-k3.1-pro": {
			Ratio: nativeRate(13), CompletionRatio: 68.0 / 13, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro through Alibaba, Beijing CNY 13 input, 1.3 cached input and 68 output per million tokens. Native Ali forwards text only. Hosted control vocabulary and limits are not copied from Moonshot's direct API.",
		},
		"kimi/kimi-k3.1-pro-highspeed": {
			Ratio: nativeRate(26), CompletionRatio: 136.0 / 26, CachedInputRatio: nativeRate(1.3),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			Description: "Moonshot-supplied Kimi K3.1 Pro Highspeed through Alibaba, Beijing CNY 26 input, 1.3 cached input and 136 output per million tokens. The 5% cached-input tariff does not double with input/output. Native Ali forwards text only.",
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
			SupportedFeatures: []string{"reasoning"}, SupportedSamplingParameters: []string{"max_tokens"},
			TimeWindows: []adaptor.TimeWindow{{
				Name: "beijing-peak", TimeZone: "Asia/Shanghai",
				Ranges:  []adaptor.ClockRange{{Start: "08:00", End: "22:00"}},
				Overlay: adaptor.ModelConfig{Ratio: nativeRate(2), CompletionRatio: 4, CachedInputRatio: nativeRate(.04)},
			}},
			Description: "Alibaba-deployed DeepSeek V4.1 Flash 1017, Beijing tariff. CNY/M input/cache/output is 2/0.04/8 from 08:00 to 22:00 Asia/Shanghai, otherwise 1/0.02/4. Native Ali forwards text only; direct DeepSeek USD tariffs, holidays and tool protocols do not apply.",
		},
	}
}
