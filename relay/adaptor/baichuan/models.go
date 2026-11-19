package baichuan

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Model definitions are maintained together in this central catalog.
// Provider-specific prices, unknown fields, and retained historical IDs remain distinct.
// Source: https://platform.baichuan-ai.com/prices
// ModelRatios contains each provider model exactly once, with no patch overlays.
var ModelRatios = map[string]adaptor.ModelConfig{
	"Baichuan-M2": {
		Ratio:                       nativeRate(2),
		CompletionRatio:             10,
		ContextLength:               32000,
		MaxOutputTokens:             32000,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens"},
		Description:                 "Baichuan-M2 on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled. Native medical API documents max_tokens in [1,32000]. Sampling controls are temperature, top_p and top_k. Native file/evidence controls and search fees are separate from this text-token catalogue.",
	},
	"Baichuan-M2-Plus": {
		Ratio:                       nativeRate(10),
		CompletionRatio:             3,
		ContextLength:               32000,
		MaxOutputTokens:             32000,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens"},
		Description:                 "Baichuan-M2-Plus on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled. Native medical API documents max_tokens in [1,32000]. Sampling controls are temperature, top_p and top_k. Native file/evidence controls and search fees are separate from this text-token catalogue.",
	},
	"Baichuan-M3": {
		Ratio:                       nativeRate(10),
		CompletionRatio:             3,
		ContextLength:               32000,
		MaxOutputTokens:             32000,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens"},
		Description:                 "Baichuan-M3 on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled. Native medical API documents max_tokens in [1,32000]. Sampling controls are temperature, top_p and top_k. Native file/evidence controls and search fees are separate from this text-token catalogue.",
	},
	"Baichuan-M3-Plus": {
		Ratio:                       nativeRate(5),
		CompletionRatio:             1.8,
		ContextLength:               32000,
		MaxOutputTokens:             32000,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens"},
		Description:                 "Baichuan-M3-Plus on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled. Native medical API documents max_tokens in [1,32000]. Sampling controls are temperature, top_p and top_k. Native file/evidence controls and search fees are separate from this text-token catalogue.",
	},
	"Baichuan-Text-Embedding": {
		Ratio:           nativeRate(0.5),
		CompletionRatio: 0,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: nativeRate(0.5),
		},
		ContextLength:    512,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Baichuan-Text-Embedding on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan2-53B": {
		Ratio:           nativeRate(20),
		CompletionRatio: 1,
		TimeWindows: []adaptor.TimeWindow{{
			Name:     "baichuan-daypart",
			TimeZone: "Asia/Shanghai",
			Ranges: []adaptor.ClockRange{{
				Start: "00:00",
				End:   "08:00",
			},
			},
			Overlay: adaptor.ModelConfig{
				Ratio:           nativeRate(10),
				CompletionRatio: 1,
			},
		},
			{
				Name:     "baichuan-daypart",
				TimeZone: "Asia/Shanghai",
				Ranges: []adaptor.ClockRange{{
					Start: "08:00",
					End:   "00:00",
				},
				},
				Overlay: adaptor.ModelConfig{
					Ratio:           nativeRate(20),
					CompletionRatio: 1,
				},
			},
		},
		ContextLength: 32000,
		Description:   "Baichuan2-53B on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan2-Turbo": {
		Ratio:                       nativeRate(8),
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"},
		Quantization:                "bf16",
		HuggingFaceID:               "baichuan-inc/Baichuan2-13B-Chat",
		Description:                 "Baichuan2-Turbo on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan3-Turbo": {
		Ratio:                       nativeRate(12),
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"},
		Description:                 "Baichuan3-Turbo on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan3-Turbo-128k": {
		Ratio:                       nativeRate(24),
		CompletionRatio:             1,
		ContextLength:               128000,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"},
		Description:                 "Baichuan3-Turbo-128k on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan4": {
		Ratio:                       nativeRate(100),
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"},
		Description:                 "Baichuan4 on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan4-Air": {
		Ratio:                       nativeRate(0.98),
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"},
		Description:                 "Baichuan4-Air on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
	"Baichuan4-Turbo": {
		Ratio:                       nativeRate(15),
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"},
		Description:                 "Baichuan4-Turbo on baichuan; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
	},
}

// ModelList retains the existing channel discovery interface.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// BaichuanToolingDefaults retains this provider's separately metered tool defaults.
var BaichuanToolingDefaults = adaptor.ChannelToolConfig{}

// nativeRate converts a CNY per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensRmb }
