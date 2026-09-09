package controller

import (
	"encoding/json"

	glog "github.com/Laisky/go-utils/v6/log"

	"github.com/Laisky/one-api/model"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// retainRealtimeEstimate distinguishes missing evidence from explicit idle/zero
// usage. Cache-allocation uncertainty retains measured receipts, not a fabricated
// empty session. Corrected terminal receipts clear their keyed usage gaps.
func retainRealtimeEstimate(usage *rmodel.Usage) bool {
	if usage == nil {
		return true
	}
	if ledger := usage.Realtime; ledger != nil {
		return ledger.HasUsageGap() || (len(ledger.Records) == 0 && len(ledger.Issues) > 0)
	}
	return usage.PromptTokens == 0 && usage.CompletionTokens == 0
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

// prepareRealtimeReceiptSettlement returns the final charge and immutable log
// metadata. Missing work retains the greater of the reservation and known cost,
// explicitly as an estimate; it neither erases known usage nor charges idle time.
// A wholly unpriceable receipt also keeps the reservation. Free groups stay free.
func prepareRealtimeReceiptSettlement(input quota.ComputeInput, preConsumed int64, lg glog.Logger) (quota.ComputeResult, model.LogMetadata) {
	result := computeRealtimeSessionQuota(input, lg)
	metadata := realtimeReceiptMetadata(input.Usage, result)
	estimated := retainRealtimeEstimate(input.Usage) || (result.UnpricedUsage && result.TotalQuota == 0)
	if estimated && input.GroupRatio != 0 {
		if metadata == nil {
			metadata = model.LogMetadata{}
		}
		metadata[model.LogMetadataKeyEstimatedCharge] = true
		metadata["realtime_billing_complete"] = false
		metadata["realtime_observed_quota"] = result.TotalQuota
		result.TotalQuota = max(result.TotalQuota, preConsumed)
	}
	return result, metadata
}

// realtimeReceiptMetadata returns bounded audit evidence and reconciliation
// status. Live receipts are all priced, but persistent logs contain counters,
// a digest and an explicitly labeled sample rather than an unbounded ledger.
func realtimeReceiptMetadata(usage *rmodel.Usage, result quota.ComputeResult) model.LogMetadata {
	if usage == nil || usage.Realtime == nil {
		return nil
	}
	audit := usage.Realtime.Audit()
	metadata := model.LogMetadata{"realtime_usage": audit,
		"realtime_billing_complete": len(result.BillingIssues) == 0 && !usage.Realtime.HasUsageGap()}
	if len(result.BillingIssues) > 0 {
		metadata["realtime_billing_issues"] = append([]string(nil), result.BillingIssues...)
	}
	lowerBound := result.UnpricedUsage
	for _, record := range usage.Realtime.Records {
		lowerBound = lowerBound || record.Tokens.CachedUnallocated > 0
	}
	if lowerBound {
		metadata["realtime_pricing_lower_bound"] = true
	}
	// Enforce an encoded-byte budget as well as collection limits. Escaping a
	// control character can expand one byte to six; leave ample TEXT headroom
	// for the rest of the billing pipeline's metadata. Counters/digest survive.
	if data, err := json.Marshal(metadata); err != nil || len(data) > 24*1024 {
		audit.SampleRecords = nil
		audit.SampleTruncated = audit.ReceiptCount > 0
		audit.Issues = nil
		metadata["realtime_usage"] = audit
		metadata["realtime_audit_detail_truncated"] = true
		metadata["realtime_billing_issues"] = []string{"audit detail exceeded the metadata budget; inspect billing diagnostics"}
	}
	return metadata
}
