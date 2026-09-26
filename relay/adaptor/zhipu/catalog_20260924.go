package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init installs BigModel's own published CNY prices, audited 2026-09-24 at
// https://docs.bigmodel.cn/cn/guide/start/pricing. Z.AI is a different USD
// catalog and is not repriced here. It takes no arguments and returns nothing.
func init() {
	ModelRatios["glm-5.3-flashx"] = adaptor.ModelConfig{
		Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 3.5,
		CachedInputRatio: 0.57 * ratio.MilliTokensRmb,
		ContextLength:    1000000, MaxOutputTokens: 131072,
		InputModalities: textImageVideoFileInput(), OutputModalities: textOutput(),
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning", "web_search"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "stop", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"low", "high", "max"}, DefaultReasoningEffort: "max",
		Description: "GLM-5.3-FlashX on BigModel: 1M context, 128K output, always-on reasoning. CNY 2 input / 0.57 cached input / 7 output per million tokens; not Z.AI's USD tariff or Flash's expired launch promotion.",
	}
	// TTS bills Unicode source characters, not UTF-8 bytes or generated tokens.
	tts := ModelRatios["glm-tts"].Clone()
	tts.Ratio = 200 * ratio.MilliTokensRmb
	tts.CompletionRatio = 0
	tts.Audio = &adaptor.AudioPricingConfig{
		InputUnit: "characters", InputPriceUsd: 2.0 / float64(ratio.ExchangeRateRmb), InputPriceQuantity: 10000,
	}
	tts.Description = "GLM-TTS: CNY 2 per 10,000 source characters; generated audio is not separately charged. Uses the published character billing unit."
	ModelRatios["glm-tts"] = tts
	asr := ModelRatios["glm-asr-2512"].Clone()
	asr.Ratio = 16 * ratio.MilliTokensRmb
	asr.CompletionRatio = 0
	asr.Description = "GLM-ASR-2512: CNY 16 per million input audio tokens; text output is free. Duration-based token estimation remains the existing relay fallback, not an exact provider per-second tariff."
	ModelRatios["glm-asr-2512"] = asr
	clone := ModelRatios["glm-tts-clone"].Clone()
	usdPerClone := 6.0 / float64(ratio.ExchangeRateRmb)
	clone.Ratio = usdPerClone * ratio.QuotaPerUsd
	clone.PerCall = &adaptor.PerCallPricingConfig{UsdPerThousandCalls: usdPerClone * 1000}
	clone.Description = "GLM-TTS-Clone: voice cloning via /api/paas/v4/voice/clone, CNY 6 per clone. Converted without truncating the RMB-to-quota factor."
	ModelRatios["glm-tts-clone"] = clone
	for id, cny := range map[string]float64{"search_std": 0.01, "search_pro": 0.03, "search_pro_sogou": 0.05, "search_pro_quark": 0.05} {
		ZhipuToolingDefaults.Pricing[id] = adaptor.ToolPricingConfig{UsdPerCall: cny / float64(ratio.ExchangeRateRmb)}
	}
}
