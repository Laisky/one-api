package model

// NativeGoogleAudioUsage records the audio portion of a native Google prompt.
// CachedKnown prevents a missing cache breakdown from receiving a guessed discount.
// These values are set only by the native countTokens/usageMetadata decoders.
type NativeGoogleAudioUsage struct {
	Tokens       int
	CachedTokens int
	CachedKnown  bool
}
