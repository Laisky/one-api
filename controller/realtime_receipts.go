package controller

import (
	glog "github.com/Laisky/go-utils/v6/log"

	"github.com/Laisky/one-api/model"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// retainRealtimeEstimate selects the legacy no-usage fallback. Explicit OpenAI
// ledgers instead settle observed receipts, including free idle sessions and
// duration-only transcription with zero input/output token counts.
func retainRealtimeEstimate(usage *rmodel.Usage) bool {
	return usage == nil || (usage.Realtime == nil && usage.PromptTokens == 0 && usage.CompletionTokens == 0)
}

// computeRealtimeSessionQuota prices a session once. Non-OpenAI sessions keep the
// legacy audio surcharge; receipt-based sessions must never add that surcharge
// because quota.Compute already prices audio and cache modalities independently.
func computeRealtimeSessionQuota(input quota.ComputeInput, lg glog.Logger) quota.ComputeResult {
	if input.Usage != nil && input.Usage.Realtime == nil {
		applyRealtimeAudioSurcharge(input.Usage, input.ModelName, input.ModelRatio, input.GroupRatio,
			input.ChannelModelRatio, input.ChannelModelConfigs, input.PricingAdaptor, lg, input.RequestTime)
	}
	result := quota.Compute(input)
	if result.PromptTokens+result.CompletionTokens == 0 && (input.Usage == nil || input.Usage.Realtime == nil) {
		result.TotalQuota = 0
	}
	return result
}

// realtimeReceiptMetadata returns audit evidence and an explicit reconciliation
// status. It retains only model names, numeric usage and accounting diagnostics,
// never transcripts, audio, authentication tokens or upstream credentials.
func realtimeReceiptMetadata(usage *rmodel.Usage, result quota.ComputeResult) model.LogMetadata {
	if usage == nil || usage.Realtime == nil {
		return nil
	}
	metadata := model.LogMetadata{"realtime_usage": usage.Realtime,
		"realtime_billing_complete": len(result.BillingIssues) == 0}
	if len(result.BillingIssues) > 0 {
		metadata["realtime_billing_issues"] = result.BillingIssues
	}
	return metadata
}
