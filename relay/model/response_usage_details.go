package model

// mergeResponsePromptDetails preserves earlier cumulative dimensions when a
// later provider receipt contains only a subset of prompt usage details.
func mergeResponsePromptDetails(previous, next *UsagePromptTokensDetails) *UsagePromptTokensDetails {
	if previous == nil {
		previous = &UsagePromptTokensDetails{}
	}
	merged := *previous
	merged.CachedTokens = max(0, previous.CachedTokens, next.CachedTokens)
	merged.AudioTokens = max(0, previous.AudioTokens, next.AudioTokens)
	merged.TextTokens = max(0, previous.TextTokens, next.TextTokens)
	merged.ImageTokens = max(0, previous.ImageTokens, next.ImageTokens)
	merged.VideoTokens = max(0, previous.VideoTokens, next.VideoTokens)
	merged.DocumentTokens = max(0, previous.DocumentTokens, next.DocumentTokens)
	merged.ImageCount = max(0, previous.ImageCount, next.ImageCount)
	merged.AudioSeconds = max(0, previous.AudioSeconds, next.AudioSeconds)
	merged.VideoFrames = max(0, previous.VideoFrames, next.VideoFrames)
	merged.DocumentPages = max(0, previous.DocumentPages, next.DocumentPages)
	if next.CachedTokensDetails != nil {
		merged.CachedTokensDetails = next.CachedTokensDetails
	}
	return &merged
}

// mergeResponseCompletionDetails retains cumulative reasoning, audio, and
// prediction dimensions across partial or repeated completion usage receipts.
func mergeResponseCompletionDetails(previous, next *UsageCompletionTokensDetails) *UsageCompletionTokensDetails {
	if previous == nil {
		previous = &UsageCompletionTokensDetails{}
	}
	merged := *previous
	merged.ReasoningTokens = max(0, previous.ReasoningTokens, next.ReasoningTokens)
	merged.AudioTokens = max(0, previous.AudioTokens, next.AudioTokens)
	merged.AcceptedPredictionTokens = max(0, previous.AcceptedPredictionTokens, next.AcceptedPredictionTokens)
	merged.RejectedPredictionTokens = max(0, previous.RejectedPredictionTokens, next.RejectedPredictionTokens)
	merged.TextTokens = max(0, previous.TextTokens, next.TextTokens)
	merged.CachedTokens = max(0, previous.CachedTokens, next.CachedTokens)
	return &merged
}
