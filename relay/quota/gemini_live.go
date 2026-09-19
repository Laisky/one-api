package quota

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/realtime"
)

// resolveGeminiLiveVisualRates completes native Live modality prices.
// Parameters: input and record identify billing, modelName is the configured ID,
// rates and resolved hold text/audio prices, and missing tracks unresolved rates.
// Returns: absolute rates and an error for unpriced usage. Documented Developer
// models default to $1/M visual input versus $0.75/M text; Vertex and unlisted
// models require explicit channel rates rather than inheriting that assumption.
func resolveGeminiLiveVisualRates(input ComputeInput, record realtime.Record, modelName string,
	rates realtime.Rates, resolved ComputeResult, missing []string) (realtime.Rates, ComputeResult, error) {
	visualMultiplier := 1.0 / 0.75
	if requiresExplicitRealtimePricing(input.PricingAdaptor, modelName) {
		visualMultiplier = 0
	}
	if image, ok := pricing.ResolveImagePricing(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime); ok && image.PromptRatio > 0 {
		visualMultiplier = image.PromptRatio
	}
	rates.Image, rates.Video = rates.Text*visualMultiplier, rates.Text*visualMultiplier
	t := record.Tokens
	if (t.Image > 0 || t.Video > 0) && visualMultiplier == 0 {
		missing = append(missing, "Gemini Live image/video")
	}
	// Never borrow OpenAI's cache discount for a Gemini receipt.
	if t.CachedText > 0 || t.CachedAudio > 0 || t.CachedImage > 0 || t.CachedVideo > 0 || t.CachedUnallocated > 0 {
		rates.CachedText, rates.CachedAudio, rates.CachedImage, rates.CachedVideo = 0, 0, 0, 0
		missing = append(missing, "Gemini Live cache")
	}
	if len(missing) > 0 {
		return rates, resolved, errors.Wrap(ErrRealtimePriceUnavailable, strings.Join(missing, ", "))
	}
	return rates, resolved, nil
}
