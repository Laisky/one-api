package vertexai

// UsesGeminiRealtimePricing identifies Vertex's Gemini-native usage contract.
// Parameters: none. Returns: true; pricing still comes from the Vertex channel.
func (a *Adaptor) UsesGeminiRealtimePricing() bool { return true }

// RequiresExplicitRealtimePricing prevents importing Developer API prices into
// Vertex sessions. Parameters: model is the configured ID. Returns: true until
// a separately verified Vertex Live price contract is bundled. Administrators
// can configure any model's complete rates, including explicitly free pricing.
func (a *Adaptor) RequiresExplicitRealtimePricing(model string) bool { return true }
