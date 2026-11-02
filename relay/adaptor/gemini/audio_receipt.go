package gemini

import (
	"github.com/Laisky/one-api/relay/model"
	"strings"
)

// NativeUsageReceiptKey carries server-only usage across the Claude protocol bridge.
const NativeUsageReceiptKey = "oneapi.native_google_usage_receipt"

// applyNativeGoogleAudioReceipt preserves measured modality counts without
// changing the public prompt total. Invalid or incomplete cache evidence remains
// explicitly uncertain rather than receiving a fabricated cheaper text rate.
func applyNativeGoogleAudioReceipt(usage *model.Usage, prompt, cache []PromptTokensDetails) {
	if usage == nil {
		return
	}
	audio, valid := nativeModalityCount(prompt, "AUDIO", usage.PromptTokens)
	if !valid {
		usage.BillingEstimateReason = "gemini_invalid_input_modality_receipt"
		return
	}
	if audio == 0 {
		return
	}
	cached := 0
	if usage.PromptTokensDetails != nil {
		cached = usage.PromptTokensDetails.CachedTokens
	}
	cacheAudio, cacheValid := nativeModalityCount(cache, "AUDIO", cached)
	known := cached == 0 || (cache != nil && cacheValid)
	if cacheAudio > audio {
		known = false
		cacheAudio = 0
	}
	if usage.PromptTokensDetails == nil {
		usage.PromptTokensDetails = &model.UsagePromptTokensDetails{}
	}
	usage.PromptTokensDetails.AudioTokens = audio
	usage.NativeGoogleAudio = &model.NativeGoogleAudioUsage{Tokens: audio, CachedTokens: cacheAudio, CachedKnown: known}
}

// nativeModalityCount validates bounded aggregate counts before selecting a modality.
func nativeModalityCount(details []PromptTokensDetails, modality string, limit int) (int, bool) {
	remaining, selected := limit, 0
	for _, entry := range details {
		if entry.TokenCount < 0 || entry.TokenCount > remaining {
			return 0, false
		}
		remaining -= entry.TokenCount
		if strings.EqualFold(entry.Modality, modality) {
			selected += entry.TokenCount
		}
	}
	return selected, true
}
