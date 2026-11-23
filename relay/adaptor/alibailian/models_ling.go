package alibailian

import "github.com/Laisky/one-api/relay/adaptor"

// lingModels returns independently owned AntDigital text-model defaults for
// AliBailian. It takes no arguments and returns the complete Ling family.
// Sources checked 2026-11-22:
// https://help.aliyun.com/zh/model-studio/ling-3-1-flash-by-antdigital
// https://help.aliyun.com/en/model-studio/antdigital
// https://help.aliyun.com/zh/model-studio/model-pricing
// https://help.aliyun.com/zh/model-studio/context-cache
func lingModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"antdigital/Ling-3.1-flash": {
			Ratio: nativeRate(.5), CompletionRatio: 3, CachedInputRatio: nativeRate(.1),
			ContextLength: 1_048_576, MaxOutputTokens: 262_144,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "reasoning"},
			Description:       "AntDigital Ling 3.1 Flash through Alibaba's Beijing OpenAI-compatible endpoint; configure the workspace-specific https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com BaseURL. Text-only hybrid thinking, enabled by default. Current public 75% promotion gives CNY 0.5 input, 0.1 cached input and 1.5 output per million tokens; list prices are 2/0.4/6. No promotion expiry is published. Supports JSON object output, not strict JSON Schema; reasoning_effort, parallel_tool_calls and web search are unsupported. Native DashScope transport is not documented.",
		},
	}
}
