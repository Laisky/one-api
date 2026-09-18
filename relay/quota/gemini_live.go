package quota

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/realtime"
)

// resolveGeminiLiveVisualRates completes Live modality pricing after the shared
// channel/provider resolver. Parameters: input and record are the billing
// request, modelName is the upstream ID, rates and resolved contain text/audio
// prices, and missing tracks unresolved buckets. Returns: absolute rates and any
// error. Image/video input costs $1/M versus $0.75/M text; an explicit channel
// image PromptRatio overrides this shared visual-input multiplier. No generated
// image or per-second video fee is used. Source, verified 2026-09-18:
// https://ai.google.dev/gemini-api/docs/pricing.
func resolveGeminiLiveVisualRates(input ComputeInput, record realtime.Record, modelName string,
	rates realtime.Rates, resolved ComputeResult, missing []string) (realtime.Rates, ComputeResult, error) {
	visualMultiplier := 1.0 / 0.75
	if image, ok := pricing.ResolveImagePricing(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime); ok && image.PromptRatio > 0 {
		visualMultiplier = image.PromptRatio
	}
	rates.Image, rates.Video = rates.Text*visualMultiplier, rates.Text*visualMultiplier
	// Live does not publish cache pricing. Never borrow GPT's 90% discount.
	t := record.Tokens
	if t.CachedText > 0 || t.CachedAudio > 0 || t.CachedImage > 0 || t.CachedVideo > 0 || t.CachedUnallocated > 0 {
		rates.CachedText, rates.CachedAudio, rates.CachedImage, rates.CachedVideo = 0, 0, 0, 0
		missing = append(missing, "Gemini Live cache")
	}
	if len(missing) > 0 {
		return rates, resolved, errors.Wrap(ErrRealtimePriceUnavailable, strings.Join(missing, ", "))
	}
	return rates, resolved, nil
}
