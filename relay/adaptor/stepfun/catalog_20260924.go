package stepfun

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers the published Step 5 Preview API, without claiming promised
// October weights are already released. Prices use the domestic CNY tariff.
// Sources checked 2026-09-24:
// https://platform.stepfun.com/docs/zh/guides/models/step-5-preview
// https://platform.stepfun.com/docs/zh/guides/pricing/details.md
func init() {
	ModelRatios["step-5-preview"] = adaptor.ModelConfig{
		Ratio:            7 * ratio.MilliTokensRmb,
		CompletionRatio:  20.0 / 7,
		CachedInputRatio: 0.35 * ratio.MilliTokensRmb,
		ContextLength:    1_000_000, MaxOutputTokens: 64_000,
		InputModalities:             []string{"text", "image", "video"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: []string{"max_tokens", "reasoning_effort"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		Description:                 "Step 5 Preview on StepFun: 1M context, 64K output, text/image/video input, tools and JSON Schema. Domestic tariff CNY 7 input / 0.35 cached input / 20 output per million tokens. API preview; no published-weight or account-access assumption.",
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
