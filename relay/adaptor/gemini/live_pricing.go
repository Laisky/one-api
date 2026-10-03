package gemini

import "github.com/Laisky/one-api/relay/realtime"

// UsesGeminiRealtimePricing identifies the native receipt modality contract.
// Parameters: none. Returns: true without inferring the protocol from a model ID.
func (a *Adaptor) UsesGeminiRealtimePricing() bool { return true }

// RequiresExplicitRealtimePricing distinguishes documented Developer defaults
// from operator-configured models. Parameters: model is the mapped ID. Returns:
// true when the operator must supply prices; this never checks upstream access.
func (a *Adaptor) RequiresExplicitRealtimePricing(model string) bool {
	return !realtime.IsGeminiLiveModel(model)
}
