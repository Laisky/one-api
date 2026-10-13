package stepfun

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers the published Step 5 Preview API, without claiming promised
// October weights are already released. Prices use the domestic CNY tariff.
// Sources rechecked 2026-10-12:
// https://platform.stepfun.com/docs/zh/guides/models/step-5-preview
// https://platform.stepfun.com/docs/zh/guides/pricing/details
//
// The announced cache-price change is account-dependent, not a universal time
// window. Keep the current/grandfathered cache price; operators using an account
// without qualifying prior use must configure the new rate after the cutoff.
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
		Description:                 "Step 5 Preview on StepFun: 1M context, 64K output, text/image/video input, tools and JSON Schema. Domestic CNY/M input/output remains 7/20. Cache input is 0.35 before 2026-10-14 00:30 Asia/Shanghai (2026-10-13 16:30 UTC). Accounts with prior pay-as-you-go Step 5 usage keep 0.35 afterward; other accounts must configure 0.70 cached-input pricing from that cutoff. These defaults preserve the current/grandfathered tariff and do not infer account eligibility. API preview; no published-weight assumption.",
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
