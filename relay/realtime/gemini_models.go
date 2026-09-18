package realtime

// IsGeminiLiveModel identifies the Live models whose wire and billing contracts
// this gateway implements. Parameters: model is the mapped upstream ID. Returns:
// true for an explicitly researched model, never a guessed family prefix.
func IsGeminiLiveModel(model string) bool {
	return model == "gemini-3.8-live" || model == "gemini-3.8-live-extended-thinking"
}
