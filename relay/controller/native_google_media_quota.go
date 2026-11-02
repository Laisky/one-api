package controller

import (
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// withMediaPromptQuota retains the existing text/output allowance and adds any
// modality surcharge measured by the normal pricing resolver. It never gives
// a cache discount during admission or changes the provider usage receipt.
func withMediaPromptQuota(quote int64, input quotautil.ComputeInput) int64 {
	if input.Usage == nil || input.Usage.PromptTokensDetails == nil || input.Usage.PromptTokensDetails.AudioTokens <= 0 {
		return quote
	}
	actual := quotautil.Compute(input).TotalQuota
	textOnly := *input.Usage
	textOnly.PromptTokensDetails = nil
	textOnly.NativeGoogleAudio = nil
	flatInput := input
	flatInput.Usage = &textOnly
	flat := quotautil.Compute(flatInput).TotalQuota
	if actual <= flat {
		return quote
	}
	return saturatingQuotaAdd(quote, actual-flat)
}
