package realtime

// IsGeminiLiveModel identifies the researched 3.8 family with bundled Developer
// API prices and model-specific setup rules. Parameters: model is the mapped
// upstream ID. Returns: true for those exact IDs. This is not a Live admission
// allowlist: other administrator-configured native models use explicit pricing.
func IsGeminiLiveModel(model string) bool {
	return model == "gemini-3.8-live" || model == "gemini-3.8-live-extended-thinking"
}
