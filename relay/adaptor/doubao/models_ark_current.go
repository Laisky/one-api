package doubao

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// arkChatConfig builds independent non-audio Standard chat metadata. Parameters
// are CNY per-million prices, advisory token limits, modalities and features;
// it returns a new configuration without account quotas or invented effort levels.
func arkChatConfig(input, cached, output float64, context, maxOutput int32, modalities, features []string, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{Ratio: input * ratio.MilliTokensRmb, CachedInputRatio: cached * ratio.MilliTokensRmb, CompletionRatio: output / input,
		ContextLength: context, MaxOutputTokens: maxOutput, InputModalities: append([]string(nil), modalities...), OutputModalities: []string{"text"},
		SupportedFeatures: append([]string(nil), features...), SupportedSamplingParameters: []string{"max_tokens", "max_completion_tokens"},
		Description: description + " Ark Standard non-audio tariff; native CNY per million tokens. Advisory K limits use the existing decimal-thousand convention. Low-latency/low-priority/batch and cache-storage fees are separate. No deployment entitlement is implied."}
}

// currentArkModels returns exact published chat identifiers and their independent
// host tariffs. It takes no parameters and returns a new map. Sources checked
// 2026-10-12: https://www.volcengine.com/docs/82379/1330310 and
// https://www.volcengine.com/docs/82379/1544106.
func currentArkModels() map[string]adaptor.ModelConfig {
	text := []string{"text"}
	vision := []string{"text", "image"}
	multimodal := []string{"text", "image", "video"}
	reasoning := []string{"tools", "reasoning"}
	structured := []string{"tools", "reasoning", "structured_outputs"}
	models := map[string]adaptor.ModelConfig{
		"doubao-seed-evolving":           arkChatConfig(6, 1.2, 30, 1024000, 256000, multimodal, structured, "Rapidly evolving Coding/Agent model; version changes upstream without a new ID."),
		"doubao-seed-2-1-pro-260915":     arkChatConfig(6, 1.2, 30, 1024000, 256000, multimodal, structured, "Seed 2.1 Pro September snapshot with 1024K context."),
		"doubao-seed-2-1-lite-260915":    arkChatConfig(.8, .16, 2.7, 1024000, 256000, multimodal, structured, "Seed 2.1 Lite September snapshot. This gateway entry advertises non-audio use only. Native audio costs CNY 12 input and 2.4 cached input per million; modality-specific cached-audio settlement requires separate integration."),
		"doubao-seed-2-1-turbo-260628":   arkChatConfig(3, .6, 15, 256000, 256000, multimodal, structured, "Seed 2.1 Turbo June snapshot with 256K context."),
		"doubao-seed-2-1-pro-260628":     arkChatConfig(6, 1.2, 30, 256000, 256000, multimodal, structured, "Earlier Seed 2.1 Pro June snapshot; retains its own 256K context."),
		"glm-5-3-flash-260828":           arkChatConfig(.8, .23, 2.8, 1024000, 128000, vision, reasoning, "Ark-hosted GLM-5.3-Flash August snapshot. Structured outputs are not advertised by this host's model table."),
		"glm-5-2-260617":                 arkChatConfig(8, 2, 28, 1024000, 128000, text, reasoning, "Ark-hosted GLM-5.2 June snapshot with text-only input."),
		"deepseek-v4-pro-ga-260813":      arkChatConfig(9, .30, 27, 1024000, 384000, text, structured, "Ark-hosted DeepSeek V4 Pro GA snapshot at current Standard prices, not native DeepSeek off-peak rates."),
		"deepseek-v4-flash-ga-260731":    arkChatConfig(3, .10, 9, 1024000, 384000, text, structured, "Ark-hosted DeepSeek V4 Flash GA snapshot. Uses the rate effective since 2026-08-21, not the expired CNY 1/0.2/2 tariff."),
		"doubao-seed-translation-250915": arkChatConfig(1.2, 0, 3.6, 4000, 3000, text, nil, "Translation-specialized model. No cache discount, tools or reasoning is advertised for Standard inference."),
	}
	return models
}
