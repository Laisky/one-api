package model

// UsageCachedTokensDetails preserves the provider's cached input modalities.
// A nil parent pointer means no split was supplied; an explicit zero remains
// authoritative. Counts are included in TextTokens/ImageTokens, not added to them.
type UsageCachedTokensDetails struct {
	AudioTokens int `json:"audio_tokens,omitempty"`
	TextTokens  int `json:"text_tokens,omitempty"`
	ImageTokens int `json:"image_tokens,omitempty"`
}
