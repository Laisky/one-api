package openai

import (
	"math"
	"strconv"
	"strings"
)

// GPT Image output-token calculator, restated from OpenAI's published cost
// calculator (verified 2026-11-23):
// https://developers.openai.com/api/docs/guides/image-generation#calculating-costs
//
// For a quality base B and an output of W x H pixels:
//
//	d      = round(B / (long_edge / short_edge))   (float64; an exact .5 rounds to even)
//	tokens = ceil(B * d * (2,000,000 + W*H) / 4,000,000)
//
// GPT Image 2.6 then multiplies by a per-quality factor and rounds half up.
// The calculator excludes text/image input tokens and streamed partial images,
// treats Sunburst and Flare identically, and reproduces OpenAI's published
// GPT Image 2 per-image prices. It does not cover "auto" quality or size.
const (
	// gptImageMaxPixels is the documented output pixel budget (4K, inclusive).
	gptImageMaxPixels = 8_294_400
	// gptImageMaxEdge is the documented maximum output edge in pixels.
	gptImageMaxEdge = 3840
	// gptImageMaxAspect is the documented maximum long:short edge ratio.
	gptImageMaxAspect = 3
	// gptImageWorstCaseEdge is the square edge that fills the pixel budget;
	// 2880x2880 yields the most output tokens of every valid size at every
	// quality (pinned by TestGPTImageCalculatorWorstCaseSize).
	gptImageWorstCaseEdge = 2880
)

// gptImageCalculatorBases are the per-quality B values of the calculator.
// The gpt-image-2 row is not used for reservations (gpt-image-2 has a published
// per-image tariff); it pins the formula to OpenAI's published price table.
var gptImageCalculatorBases = map[string]map[string]int{
	"gpt-image-2":   {"low": 16, "medium": 48, "high": 96},
	"gpt-image-2.5": {"low": 16, "medium": 24, "high": 48, "xhigh": 64, "max": 96},
	"gpt-image-2.6": {"low": 16, "medium": 24, "high": 48, "xhigh": 64, "max": 96},
}

// gptImage26QualityFactors are the GPT Image 2.6 per-quality multipliers.
var gptImage26QualityFactors = map[string]float64{"low": 4.2, "medium": 3.3, "high": 1, "xhigh": 1, "max": 1}

// gptImageCalculatorFamily maps a published model ID to its calculator family.
// Parameters: model is the provider model ID. Returns: the family key and
// whether the calculator covers the model.
func gptImageCalculatorFamily(model string) (string, bool) {
	switch model {
	case "gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08":
		return "gpt-image-2.5", true
	case "gpt-image-2.6-sunburst", "gpt-image-2.6-flare":
		return "gpt-image-2.6", true
	default:
		return "", false
	}
}

// EstimateGPTImageOutputTokens returns OpenAI's calculator estimate of the
// image-output tokens one generated image consumes, resolving anything the
// calculator does not price to its most expensive case: an "auto", empty,
// unparseable, or out-of-bounds size becomes 2880x2880, and an "auto" or
// unknown quality becomes the most expensive quality for that size. Parameters:
// model is the provider model ID, size a normalized "WxH" or "auto", and
// quality a normalized quality key. Returns: the per-image token estimate and
// whether the calculator covers the model.
func EstimateGPTImageOutputTokens(model, size, quality string) (int, bool) {
	family, ok := gptImageCalculatorFamily(model)
	if !ok {
		return 0, false
	}
	width, height, ok := parseGPTImageCalculatorSize(size)
	if !ok {
		width, height = gptImageWorstCaseEdge, gptImageWorstCaseEdge
	}
	if _, known := gptImageCalculatorBases[family][quality]; known {
		return gptImageCalculatorTokens(family, quality, width, height), true
	}
	worst := 0
	for candidate := range gptImageCalculatorBases[family] {
		worst = max(worst, gptImageCalculatorTokens(family, candidate, width, height))
	}
	return worst, true
}

// parseGPTImageCalculatorSize parses a "WxH" size within the documented output
// bounds. Parameters: size is a normalized size key. Returns: the width and
// height, and false for "auto", malformed sizes, or sizes outside the
// documented edge, aspect, or pixel-budget limits.
func parseGPTImageCalculatorSize(size string) (int, int, bool) {
	rawWidth, rawHeight, found := strings.Cut(strings.ToLower(strings.TrimSpace(size)), "x")
	if !found {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(rawWidth)
	height, heightErr := strconv.Atoi(rawHeight)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	long, short := max(width, height), min(width, height)
	if long > gptImageMaxEdge || long > gptImageMaxAspect*short || width*height > gptImageMaxPixels {
		return 0, 0, false
	}
	return width, height, true
}

// gptImageCalculatorTokens applies the calculator formula with the same
// float64 operation order as OpenAI's implementation. Parameters: family and
// quality select B (and the 2.6 factor); width and height are positive pixel
// edges. Returns: the per-image output-token estimate.
func gptImageCalculatorTokens(family, quality string, width, height int) int {
	base := gptImageCalculatorBases[family][quality]
	long, short := max(width, height), min(width, height)
	scaled := float64(base) / (float64(long) / float64(short))
	floor := math.Floor(scaled)
	d := math.Round(scaled)
	if scaled-floor == 0.5 {
		d = floor + math.Mod(floor, 2)
	}
	tokens := math.Ceil(float64(base) * d * (2e6 + float64(width*height)) / 4e6)
	if family == "gpt-image-2.6" {
		tokens = math.Round(tokens * gptImage26QualityFactors[quality])
	}
	return int(tokens)
}
