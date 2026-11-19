package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// togetherPixelsPerMegapixel is Together AI's megapixel: "MP = (Width x Height
// / 1,000,000)" (https://docs.together.ai/docs/serverless/models.md).
const togetherPixelsPerMegapixel = 1_000_000

// perMegapixelImageTariff returns the tariff of a model Together AI lists with
// the `megapixel` unit. Together states "The listed price is a true rate, so
// doubling the pixels doubles the cost" and "Cost = MP x Price per MP x (Steps
// / Default Steps)" (https://docs.together.ai/docs/serverless/overview.md), so
// the rate applies pro rata to every output megapixel. The tariff's one
// megapixel minimum is lifted for these models by ImageBillingRuleFor.
// Parameters: usdPerMegapixel is the listed price. Returns: a fresh tariff
// owned by the caller.
func perMegapixelImageTariff(usdPerMegapixel float64) *adaptor.ImageMegapixelPricing {
	return &adaptor.ImageMegapixelPricing{
		FirstMegapixelUsd:      usdPerMegapixel,
		AdditionalMegapixelUsd: usdPerMegapixel,
		PixelsPerMegapixel:     togetherPixelsPerMegapixel,
		Rounding:               adaptor.ImageMegapixelRoundingExact,
	}
}

// perImageAreaTariff returns the tariff of a model Together AI lists with the
// `image` unit. Together passes those requests to the model's provider and
// warns that "the actual cost varies with resolution"; its pricing page states
// "Displayed prices refer to the lowest resolution/duration settings"
// (https://www.together.ai/pricing). The listed price is therefore a floor:
// one image never costs less, and larger outputs scale it by their megapixels,
// matching the area-proportional size table these models used before.
// Parameters: listedUsd is the listed per-image price. Returns: a fresh tariff
// owned by the caller.
func perImageAreaTariff(listedUsd float64) *adaptor.ImageMegapixelPricing {
	return &adaptor.ImageMegapixelPricing{
		FirstMegapixelUsd:      listedUsd,
		AdditionalMegapixelUsd: listedUsd,
		PixelsPerMegapixel:     togetherPixelsPerMegapixel,
		Rounding:               adaptor.ImageMegapixelRoundingExact,
	}
}

// ImageBillingRule is how Together AI bills one image model beyond its catalog
// tariff. The zero value adds nothing to the tariff.
type ImageBillingRule struct {
	// DefaultSteps is the published default step count. Together bills steps
	// "only if you go above the default steps", proportionally; zero means no
	// default is published and steps do not change the price.
	DefaultSteps int
	// ProRataOutput reports a true per-megapixel rate: an output smaller than
	// one megapixel pays its fraction of the rate, with no minimum.
	ProRataOutput bool
}

// imageBillingRules lists the models Together AI bills by megapixel, with
// their published default steps (https://www.together.ai/pricing "Default
// steps" column; "$0.07 at 50 steps" for FLUX.2 [max] in
// https://docs.together.ai/docs/serverless/models.md). FLUX1.1 [pro] lists no
// default steps and does not take a steps parameter
// (https://docs.together.ai/docs/inference/images/parameters.md).
var imageBillingRules = map[string]ImageBillingRule{
	"black-forest-labs/FLUX.2-max":         {DefaultSteps: 50, ProRataOutput: true},
	"black-forest-labs/FLUX.1-kontext-max": {DefaultSteps: 28, ProRataOutput: true},
	"black-forest-labs/FLUX.1-kontext-pro": {DefaultSteps: 28, ProRataOutput: true},
	"black-forest-labs/FLUX.1.1-pro":       {ProRataOutput: true},
}

// ImageBillingRuleFor returns Together AI's billing rule for an image model.
// Parameters: model is the upstream model name. Returns: the rule, or the
// zero rule for a model without per-megapixel billing.
func ImageBillingRuleFor(model string) ImageBillingRule {
	return imageBillingRules[model]
}

// gemini3ProImage4KMultiplier scales the $0.134 1K/2K price to the "about
// $0.24 per image" Together quotes at 4K.
const gemini3ProImage4KMultiplier = 0.24 / 0.134

// gemini3ProImageSizeTiers returns the Gemini 3 Pro Image price tier of every
// output size Together AI documents for it ("Supported dimensions" in
// https://docs.together.ai/docs/serverless/models.md): 1K and 2K cost the
// listed $0.134 and 4K about $0.24. The earlier 1920x1080 and 3840x2160 keys
// are kept at the tier their area falls in so existing requests stay priced.
// It takes no arguments. Returns: a fresh WIDTHxHEIGHT to multiplier table.
func gemini3ProImageSizeTiers() map[string]float64 {
	tiers := map[string]float64{
		"1920x1080": 1,
		"3840x2160": gemini3ProImage4KMultiplier,
	}
	for _, size := range []string{
		// 1K
		"1024x1024", "1264x848", "848x1264", "1200x896", "896x1200", "928x1152",
		"1152x928", "768x1376", "1376x768", "1548x672", "1584x672",
		// 2K
		"2048x2048", "2528x1696", "1696x2528", "2400x1792", "1792x2400",
		"1856x2304", "2304x1856", "1536x2752", "2752x1536", "3168x1344",
	} {
		tiers[size] = 1
	}
	for _, size := range []string{
		// 4K
		"4096x4096", "5096x3392", "5056x3392", "3392x5096", "3392x5056",
		"4800x3584", "3584x4800", "3712x4608", "4608x3712", "3072x5504",
		"5504x3072", "6336x2688",
	} {
		tiers[size] = gemini3ProImage4KMultiplier
	}
	return tiers
}
