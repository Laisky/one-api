package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// xiaomimimoModels returns independently owned xiaomimimo defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func xiaomimimoModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"XiaomiMiMo/MiMo-V2.5": {
			Ratio:                       nativeRate(0.4),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(0.08),
			ContextLength:               262144,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "MiMo V2.5 general reasoning model.",
		},
		"XiaomiMiMo/MiMo-V2.5-Pro": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             3,
			CachedInputRatio:            nativeRate(0.2),
			ContextLength:               1048576,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "MiMo V2.5 Pro long-context reasoning model.",
		},
		// DeepInfra lists all three MiMo V2.5 speech models at $0.00 per 1M input
		// characters, "Free for a limited time" (model pages and
		// https://api.deepinfra.com/models/list: cents_per_input_chars 0,
		// discount_ends_at null), verified 2026-10-05. No end date is published, so
		// ValidUntil stays unset rather than inventing one; replace this explicit
		// promotional zero with the paid character tariff when the promotion ends.
		"XiaomiMiMo/MiMo-V2.5-tts": {
			PricingProvenance: &adaptor.PricingProvenance{State: adaptor.TariffStatePromotionalFree, Unit: adaptor.TariffUnitCharacters, Source: "https://deepinfra.com/XiaomiMiMo/MiMo-V2.5-tts", VerifiedAt: "2026-10-05"},
			Audio:             &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 0},
			Ratio:             0,
			CompletionRatio:   1,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"audio"},
			Description:       "MiMo V2.5 text-to-speech model.",
		},
		"XiaomiMiMo/MiMo-V2.5-tts-voiceclone": {
			PricingProvenance: &adaptor.PricingProvenance{State: adaptor.TariffStatePromotionalFree, Unit: adaptor.TariffUnitCharacters, Source: "https://deepinfra.com/XiaomiMiMo/MiMo-V2.5-tts-voiceclone", VerifiedAt: "2026-10-05"},
			Audio:             &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 0},
			Ratio:             0,
			CompletionRatio:   1,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"audio"},
			Description:       "MiMo V2.5 voice-cloning speech model.",
		},
		"XiaomiMiMo/MiMo-V2.5-tts-voicedesign": {
			PricingProvenance: &adaptor.PricingProvenance{State: adaptor.TariffStatePromotionalFree, Unit: adaptor.TariffUnitCharacters, Source: "https://deepinfra.com/XiaomiMiMo/MiMo-V2.5-tts-voicedesign", VerifiedAt: "2026-10-05"},
			Audio:             &adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 0},
			Ratio:             0,
			CompletionRatio:   1,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"audio"},
			Description:       "MiMo V2.5 voice-design speech model.",
		},
		"XiaomiMiMo/MiMo-V2.6-Flash": {
			Ratio:             nativeRate(0.14),
			CompletionRatio:   (0.28 * 1.0) / 0.14,
			CachedInputRatio:  nativeRate(0.0028),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image", "video", "audio"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp8",
			Description:       "XiaomiMiMo/MiMo-V2.6-Flash on DeepInfra; public host catalogue verified 2026-10-12. Prices and limits are host-specific, not copied from another provider. Unpublished output limits, cache prices, sampling controls and reasoning-effort levels remain unspecified.",
		},
		"XiaomiMiMo/MiMo-V2.6-Pro": {
			Ratio:             nativeRate(0.387),
			CompletionRatio:   (0.87 * 1.0) / 0.43,
			CachedInputRatio:  nativeRate(0.00323999883),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image", "video", "audio"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp8",
			Description:       "XiaomiMiMo/MiMo-V2.6-Pro on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified. Includes the published 10% discount; no expiry is invented.",
		},
	}
}
