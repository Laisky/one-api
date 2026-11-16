package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// mistralaiModels returns independently owned mistralai defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func mistralaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"mistralai/Mistral-Nemo-Instruct-2407": {
			Ratio:                       nativeRate(0.029),
			CompletionRatio:             (0.03 * 1.0) / 0.029,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "mistralai/Mistral-Nemo-Instruct-2407 on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified.",
			Quantization:                "fp8",
		},
		"mistralai/Mistral-Small-24B-Instruct-2501": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             1.5999999999999999,
			ContextLength:               32768,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "mistralai/Mistral-Small-24B-Instruct-2501 on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified.",
			Quantization:                "fp8",
		},
		"mistralai/Mistral-Small-3.2-24B-Instruct-2506": {
			Ratio:                       nativeRate(0.075),
			CompletionRatio:             2.666666666666667,
			ContextLength:               128000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "mistralai/Mistral-Small-3.2-24B-Instruct-2506 on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified.",
			Quantization:                "fp8",
		},
		"mistralai/Voxtral-Mini-3B-2507": {
			Ratio:           nativeRate(1.6666666666666667),
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				PromptTokensPerSecond: 10,
				UsdPerSecond:          1.6666666666666667e-05,
			},
			InputModalities:  []string{"audio"},
			OutputModalities: []string{"text"},
			Description:      "Voxtral Mini transcription and speech translation model.",
		},
		"mistralai/Voxtral-Small-24B-2507": {
			Ratio:           nativeRate(5),
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				PromptTokensPerSecond: 10,
				UsdPerSecond:          5e-05,
			},
			InputModalities:  []string{"audio"},
			OutputModalities: []string{"text"},
			Description:      "Voxtral Small transcription and speech translation model.",
		},
	}
}
