package controller

import (
	"math"

	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// gptImageTokenBucketPricing stores USD per 1M token prices for GPT Image models.
type gptImageTokenBucketPricing struct {
	inputTextUSD        float64
	cachedInputTextUSD  float64
	inputImageUSD       float64
	cachedInputImageUSD float64
	outputImageUSD      float64
}

var gptImageTokenBucketPrices = map[string]gptImageTokenBucketPricing{
	// https://platform.openai.com/docs/models/gpt-image-1
	"gpt-image-1": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       10.0,
		cachedInputImageUSD: 2.5,
		outputImageUSD:      40.0,
	},
	// https://platform.openai.com/docs/models/gpt-image-1-mini
	"gpt-image-1-mini": {
		inputTextUSD:        2.0,
		cachedInputTextUSD:  0.20,
		inputImageUSD:       2.5,
		cachedInputImageUSD: 0.25,
		outputImageUSD:      8.0,
	},
	// https://platform.openai.com/docs/models/chatgpt-image-latest
	"chatgpt-image-latest": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      32.0,
	},
	// https://platform.openai.com/docs/models/gpt-image-1.5
	"gpt-image-1.5": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      32.0,
	},
	"gpt-image-1.5-2025-12-16": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      32.0,
	},
	// https://platform.openai.com/docs/models/gpt-image-2
	"gpt-image-2": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
	"gpt-image-2-2026-04-21": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
	// GPT Image 2.5 token rates verified 2026-09-09 (not per-image estimates).
	// https://developers.openai.com/api/docs/models/gpt-image-2.5-sunburst
	// https://developers.openai.com/api/docs/models/gpt-image-2.5-flare
	"gpt-image-2.5-sunburst": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
	"gpt-image-2.5-sunburst-2026-09-08": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
	"gpt-image-2.5-flare": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
	"gpt-image-2.5-flare-2026-09-08": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
}

// computeGptImageTokenQuota calculates quota for GPT image family models using five billing buckets:
// input text, cached input text, input image, cached input image, and output image tokens.
// Prices are expressed in USD per 1M tokens and multiplied by the groupRatio (quota multiplier) before returning quota units.
func computeGptImageTokenQuota(modelName string, usage *relaymodel.Usage, groupRatio float64) float64 {
	if usage == nil {
		return 0
	}
	pricing, ok := gptImageTokenBucketPrices[modelName]
	if !ok {
		return 0
	}
	if isGPTImage25Model(modelName) {
		return computeGPTImage25TokenQuota(usage, pricing, groupRatio)
	}

	var textIn, imageIn, cachedIn int
	if usage.PromptTokensDetails != nil {
		textIn = usage.PromptTokensDetails.TextTokens
		imageIn = usage.PromptTokensDetails.ImageTokens
		cachedIn = usage.PromptTokensDetails.CachedTokens
	}
	if textIn < 0 {
		textIn = 0
	}
	if imageIn < 0 {
		imageIn = 0
	}
	if cachedIn < 0 {
		cachedIn = 0
	}
	totalIn := textIn + imageIn
	if cachedIn > totalIn {
		cachedIn = totalIn
	}
	cachedText := 0
	cachedImage := 0
	if cachedIn > 0 && totalIn > 0 {
		cachedText = min(max(int(math.Round(float64(cachedIn)*(float64(textIn)/float64(totalIn)))), 0), cachedIn)
		cachedImage = cachedIn - cachedText
	}
	normalText := max(textIn-cachedText, 0)
	normalImage := max(imageIn-cachedImage, 0)
	outTokens := max(usage.CompletionTokens, 0)

	quota := 0.0
	quota += float64(normalText) * pricing.inputTextUSD * billingratio.MilliTokensUsd
	quota += float64(cachedText) * pricing.cachedInputTextUSD * billingratio.MilliTokensUsd
	quota += float64(normalImage) * pricing.inputImageUSD * billingratio.MilliTokensUsd
	quota += float64(cachedImage) * pricing.cachedInputImageUSD * billingratio.MilliTokensUsd
	quota += float64(outTokens) * pricing.outputImageUSD * billingratio.MilliTokensUsd

	if groupRatio > 0 {
		quota *= groupRatio
	}
	return quota
}

// computeImageUsageQuota routes to the correct usage-based cost function per model.
// Returns 0 when usage is missing or the model has no token pricing rule.
func computeImageUsageQuota(modelName string, usage *relaymodel.Usage, groupRatio float64) float64 {
	if usage == nil {
		return 0
	}
	// Basic reliability check: some providers may omit usage entirely
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && (usage.PromptTokensDetails == nil) {
		return 0
	}
	// The bucket table is the allowlist; keep dispatch in sync with every priced
	// alias and snapshot without maintaining a second model-name list.
	return computeGptImageTokenQuota(modelName, usage, groupRatio)
}

// imageQuotaSummary tracks the breakdown of image billing across fixed per-image components and token-based usage.
type imageQuotaSummary struct {
	BaseQuota       int64
	TokenQuota      int64
	TokenQuotaFloat float64
	TotalQuota      int64
}

// calculateImageBaseQuota derives the upfront quota reservation for an image request.
// When per-image billing is enabled, the quota scales with the billed image count and tier multiplier.
// For token-only models, the base quota falls back to the model ratio estimation.
func calculateImageBaseQuota(imagePriceUsd, ratio, imageCostRatio, groupRatio float64, count int) int64 {
	if count <= 0 {
		return 0
	}
	if imagePriceUsd > 0 {
		perImageQuota := math.Ceil(imagePriceUsd * billingratio.QuotaPerUsd * imageCostRatio * groupRatio)
		if perImageQuota <= 0 {
			return 0
		}
		return int64(perImageQuota) * int64(count)
	}
	if ratio <= 0 {
		return 0
	}
	perImageQuota := math.Ceil(ratio * imageCostRatio)
	if perImageQuota <= 0 {
		return 0
	}
	return int64(perImageQuota) * int64(count)
}

// finalizeImageQuota merges token usage data with the reserved base quota to produce the final billed amount.
// GPT Image 2.5 replaces its reserve; older models retain additive render billing.
// Parameters: baseQuota is reserved quota, perImageBilling selects the legacy path,
// imageModel/actualModel identify pricing, usage contains provider counts, and groupRatio
// scales prices; configs optionally provides request-start channel pricing. Returns: the final charge and its token/reservation breakdown.
func finalizeImageQuota(baseQuota int64, perImageBilling bool, imageModel string, actualModel string, usage *relaymodel.Usage, groupRatio float64, configs ...adaptor.ModelConfig) imageQuotaSummary {
	summary := imageQuotaSummary{
		BaseQuota:  baseQuota,
		TotalQuota: baseQuota,
	}
	if usage == nil {
		return summary
	}

	tokenQuotaFloat := computeImageUsageQuota(imageModel, usage, groupRatio)
	if isGPTImage25Model(imageModel) && len(configs) > 0 {
		tokenQuotaFloat = computeGPTImage25ConfiguredQuota(usage, configs[0], groupRatio)
	}
	if tokenQuotaFloat < 0 {
		tokenQuotaFloat = 0
	}
	tokenQuota := int64(math.Ceil(tokenQuotaFloat))
	if tokenQuota < 0 {
		tokenQuota = 0
	}
	summary.TokenQuotaFloat = tokenQuotaFloat
	summary.TokenQuota = tokenQuota

	if isGPTImage25Model(imageModel) {
		// A successful render needs output usage. Missing/partial/ambiguous
		// usage keeps the explicit operator fallback, never a one-token charge.
		if usage.CompletionTokens > 0 && tokenQuota > 0 {
			summary.TotalQuota = tokenQuota
		} else {
			summary.TokenQuota = 0
			summary.TokenQuotaFloat = 0
		}
		return summary
	}

	if perImageBilling {
		if tokenQuota > 0 {
			summary.TotalQuota += tokenQuota
		}
		return summary
	}

	if tokenQuota > 0 {
		summary.TotalQuota = tokenQuota
		return summary
	}

	fallbackFloat := computeLegacyImageTokenQuota(actualModel, usage, groupRatio)
	if fallbackFloat > 0 {
		fallbackQuota := int64(math.Ceil(fallbackFloat))
		if fallbackQuota < 0 {
			fallbackQuota = 0
		}
		summary.TokenQuotaFloat = fallbackFloat
		summary.TokenQuota = fallbackQuota
		summary.TotalQuota = baseQuota + fallbackQuota
	}

	return summary
}

// computeLegacyImageTokenQuota handles legacy token billing paths for image models lacking detailed bucket pricing.
func computeLegacyImageTokenQuota(modelName string, usage *relaymodel.Usage, groupRatio float64) float64 {
	if usage == nil || usage.PromptTokensDetails == nil {
		return 0
	}
	switch modelName {
	case "gpt-image-1", "gpt-image-1-mini":
		textTokens := usage.PromptTokensDetails.TextTokens
		if textTokens < 0 {
			textTokens = 0
		}
		imageTokens := usage.PromptTokensDetails.ImageTokens
		if imageTokens < 0 {
			imageTokens = 0
		}
		quota := float64(textTokens)*5*billingratio.MilliTokensUsd + float64(imageTokens)*10*billingratio.MilliTokensUsd
		if groupRatio > 0 {
			quota *= groupRatio
		}
		return quota
	case "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16", "gpt-image-2", "gpt-image-2-2026-04-21":
		textTokens := usage.PromptTokensDetails.TextTokens
		if textTokens < 0 {
			textTokens = 0
		}
		imageTokens := usage.PromptTokensDetails.ImageTokens
		if imageTokens < 0 {
			imageTokens = 0
		}
		quota := float64(textTokens)*5*billingratio.MilliTokensUsd + float64(imageTokens)*8*billingratio.MilliTokensUsd
		if groupRatio > 0 {
			quota *= groupRatio
		}
		return quota
	default:
		return 0
	}
}
