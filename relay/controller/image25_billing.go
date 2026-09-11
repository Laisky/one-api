package controller

import (
	"math"

	"github.com/Laisky/errors/v2"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// isGPTImage25Model identifies the four verified token-priced image IDs.
// Parameters: name is the mapped provider model. Returns: whether this policy applies.
func isGPTImage25Model(name string) bool {
	switch name {
	case "gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08":
		return true
	default:
		return false
	}
}

// imagePostConsumeDelta computes the remaining debit or reservation refund.
// Parameters: name selects the billing policy; used and reserved are quota units.
// Returns: a signed delta for GPT Image 2.5, preserving legacy settlement elsewhere.
func imagePostConsumeDelta(name string, used, reserved int64) int64 {
	delta := used - reserved
	if delta < 0 && !isGPTImage25Model(name) {
		return 0
	}
	return delta
}

// computeGPTImage25TokenQuota prices normalized provider usage without inventing
// a proportional cache split. Parameters: usage carries token counts, prices are
// USD per million tokens, and group is the quota multiplier. Returns: quota units,
// or zero when the provider's buckets are inconsistent or ambiguous.
func computeGPTImage25TokenQuota(usage *relaymodel.Usage, prices gptImageTokenBucketPricing, group float64) float64 {
	if usage == nil || usage.PromptTokens < 0 || usage.CompletionTokens < 0 || group <= 0 || math.IsNaN(group) || math.IsInf(group, 0) {
		return 0
	}
	var textIn, imageIn, cached int
	var cacheDetails *relaymodel.UsageCachedTokensDetails
	if d := usage.PromptTokensDetails; d != nil {
		textIn, imageIn, cached = d.TextTokens, d.ImageTokens, d.CachedTokens
		cacheDetails = d.CachedTokensDetails
	}
	if textIn < 0 || imageIn < 0 || cached < 0 || textIn > int(^uint(0)>>1)-imageIn {
		return 0
	}
	total := textIn + imageIn
	if usage.PromptTokens > 0 {
		if total > usage.PromptTokens {
			return 0
		}
		// Compatibility fallback: bill unclassified input at the text rate rather
		// than silently dropping it. This is not a claim about its actual modality.
		textIn += usage.PromptTokens - total
		total = usage.PromptTokens
	}
	if cached > total {
		return 0
	}
	cachedText, cachedImage := 0, 0
	if cacheDetails != nil {
		cachedText, cachedImage = cacheDetails.TextTokens, cacheDetails.ImageTokens
		if cachedText < 0 || cachedText > textIn || cachedImage < 0 || cachedImage > imageIn {
			return 0
		}
		// A supplied aggregate must agree with the modality-specific buckets.
		// Some compatible endpoints omit the aggregate but supply the split.
		if cached > 0 && cachedText+cachedImage != cached {
			return 0
		}
	} else if cached > 0 {
		switch {
		case imageIn == 0:
			cachedText = cached
		case textIn == 0:
			cachedImage = cached
		default:
			// The split is unknowable: use the explicitly configured fallback
			// during settlement instead of claiming a guessed token charge.
			return 0
		}
	}
	quota := (float64(textIn-cachedText)*prices.inputTextUSD +
		float64(cachedText)*prices.cachedInputTextUSD +
		float64(imageIn-cachedImage)*prices.inputImageUSD +
		float64(cachedImage)*prices.cachedInputImageUSD +
		float64(usage.CompletionTokens)*prices.outputImageUSD) * billingratio.MilliTokensUsd * group
	if math.IsNaN(quota) || math.IsInf(quota, 0) || quota >= float64(uint64(1)<<63) {
		return 0
	}
	return quota
}

// validateGPTImage25Billing checks admission before quota lookup or upstream I/O.
// Parameters: name is the mapped model; fallbackUSD, tier, group, and count define
// the operator's reservation/fallback tariff. Returns: a configuration error when
// no positive, finite, representable reservation exists; nil for other models.
func validateGPTImage25Billing(name string, fallbackUSD, tier, group float64, count int) error {
	if !isGPTImage25Model(name) {
		return nil
	}
	if fallbackUSD <= 0 || tier <= 0 || group <= 0 || count <= 0 {
		return errors.New("GPT Image 2.5 requires a positive image.price_per_image_usd reservation/fallback tariff in channel model_configs; this is operator policy, not an official per-image price")
	}
	perImage := math.Ceil(fallbackUSD * billingratio.QuotaPerUsd * tier * group)
	if math.IsNaN(perImage) || math.IsInf(perImage, 0) || perImage <= 0 || perImage >= float64(uint64(1)<<63)/float64(count) {
		return errors.New("GPT Image 2.5 reservation/fallback tariff must produce finite, representable quota")
	}
	return nil
}
