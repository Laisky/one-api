package openai

import (
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
)

// OpenAI patch-based vision models cover an image with 32px patches after
// model- and detail-specific resizing, then multiply the patch count.
// Source: https://developers.openai.com/api/docs/guides/images-vision#patch-based-image-tokenization
const (
	openAIImagePatchSize = 32
	// openAIImageRejectPatches is the documented per-image patch limit above
	// which the API rejects the request instead of resizing it.
	openAIImageRejectPatches = 30000
	// openAIImageUnboundedEdge is the pixel-dimension limit of the original
	// detail level on models that preserve native image dimensions.
	openAIImageUnboundedEdge = 65535
)

// patchSizing is one documented detail level: images are fitted within
// maxEdge pixels on each side, then scaled to at most budget patches when
// budget is positive.
type patchSizing struct {
	maxEdge int
	budget  int
}

// patchProfile describes the documented image tokenization of one OpenAI
// patch-based model family. auto names the detail level that "auto" uses.
type patchProfile struct {
	multiplier float64
	low        patchSizing
	high       patchSizing
	original   patchSizing
	auto       string
}

var (
	// gpt-6-astra: low fits 512x512; high is 2,500 patches with no practical
	// edge limit; original keeps native dimensions; auto behaves like original.
	patchProfileGPT6 = patchProfile{multiplier: 1.2,
		low: patchSizing{maxEdge: 512}, high: patchSizing{maxEdge: openAIImageUnboundedEdge, budget: 2500},
		original: patchSizing{maxEdge: openAIImageUnboundedEdge}, auto: "original"}
	// gpt-5.6 sol/terra/luna: like gpt-6-astra, but high fits 2048x2048.
	patchProfileGPT56 = patchProfile{multiplier: 1.2,
		low: patchSizing{maxEdge: 512}, high: patchSizing{maxEdge: 2048, budget: 2500},
		original: patchSizing{maxEdge: openAIImageUnboundedEdge}, auto: "original"}
	// gpt-5.5: original is limited to 6000px and 10,000 patches; auto behaves like original.
	patchProfileGPT55 = patchProfile{multiplier: 1.2,
		low: patchSizing{maxEdge: 512}, high: patchSizing{maxEdge: 2048, budget: 2500},
		original: patchSizing{maxEdge: 6000, budget: 10000}, auto: "original"}
	// gpt-5.4 family: low uses a 6,144-patch budget and can exceed high; auto behaves like high.
	patchProfileGPT54 = patchProfile{multiplier: 1.2,
		low: patchSizing{maxEdge: 2048, budget: 6144}, high: patchSizing{maxEdge: 2048, budget: 2500},
		original: patchSizing{maxEdge: 6000, budget: 10000}, auto: "high"}
)

// uniformPatchProfile returns a profile whose detail levels all share the
// documented 2048px / 6,144-patch sizing, as gpt-5.2 and gpt-4.1-mini do.
func uniformPatchProfile(multiplier float64) patchProfile {
	sizing := patchSizing{maxEdge: 2048, budget: 6144}
	return patchProfile{multiplier: multiplier, low: sizing, high: sizing, original: sizing, auto: "high"}
}

// openAIPatchProfile returns the documented patch tokenization for model and
// whether model is a patch-based OpenAI vision model. Deprecated patch models
// that the current guide no longer sizes use its general 2048px / 6,144-patch
// sizing, which bounds their historical, smaller budget.
func openAIPatchProfile(model string) (patchProfile, bool) {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "openai/")
	switch {
	case strings.HasPrefix(name, "gpt-6"):
		return patchProfileGPT6, true
	case strings.HasPrefix(name, "gpt-5.6"):
		return patchProfileGPT56, true
	case strings.HasPrefix(name, "gpt-5.5"), name == "chat-latest":
		// chat-latest is the rolling GPT-5.5 Instant alias.
		return patchProfileGPT55, true
	case strings.HasPrefix(name, "gpt-5.4"):
		return patchProfileGPT54, true
	case strings.HasPrefix(name, "gpt-5.2"), strings.HasPrefix(name, "gpt-5.3"):
		return uniformPatchProfile(1.2), true
	case strings.HasPrefix(name, "gpt-5-mini"):
		return uniformPatchProfile(1.2), true
	case strings.HasPrefix(name, "gpt-5-nano"):
		return uniformPatchProfile(1.5), true
	case strings.HasPrefix(name, "gpt-4.1-mini"):
		return uniformPatchProfile(1.62), true
	case strings.HasPrefix(name, "gpt-4.1-nano"):
		return uniformPatchProfile(2.46), true
	case strings.HasPrefix(name, "o4-mini"):
		return uniformPatchProfile(1.72), true
	}
	return patchProfile{}, false
}

// sizing returns the sizing for detail. Empty detail means auto. It returns
// an error for values outside the documented low/high/original/auto set.
func (p patchProfile) sizing(detail string) (patchSizing, error) {
	if detail == "" || detail == "auto" {
		detail = p.auto
	}
	switch detail {
	case "low":
		return p.low, nil
	case "high":
		return p.high, nil
	case "original":
		return p.original, nil
	default:
		return patchSizing{}, errors.Errorf("unsupported image detail %q", detail)
	}
}

// maxTokens returns the largest documented token count for detail, or for
// any detail level when detail is unknown. It never returns zero.
func (p patchProfile) maxTokens(detail string) int {
	if sizing, err := p.sizing(detail); err == nil {
		return patchTokensFromCount(sizing.maxPatches(), p.multiplier)
	}
	largest := 0
	for _, sizing := range []patchSizing{p.low, p.high, p.original} {
		largest = max(largest, sizing.maxPatches())
	}
	return patchTokensFromCount(largest, p.multiplier)
}

// maxPatches returns the largest patch count an image can produce under s,
// bounded by the API's rejection limit.
func (s patchSizing) maxPatches() int {
	perSide := (s.maxEdge + openAIImagePatchSize - 1) / openAIImagePatchSize
	limit := openAIImageRejectPatches
	if s.budget > 0 {
		limit = min(limit, s.budget)
	}
	if perSide > 0 && perSide <= limit/perSide {
		limit = min(limit, perSide*perSide)
	}
	return limit
}

// countPatchImageTokens estimates the tokens of one image of width x height
// under profile p and detail, following the documented resize-then-patch
// algorithm. It returns an error for unsupported detail or invalid dimensions.
func countPatchImageTokens(width, height int, detail string, p patchProfile) (int, error) {
	sizing, err := p.sizing(detail)
	if err != nil {
		return 0, errors.Wrap(err, "resolve patch image sizing")
	}
	if width <= 0 || height <= 0 {
		return 0, errors.Errorf("invalid image dimensions %dx%d", width, height)
	}
	w, h := float64(width), float64(height)
	// Fit within the pixel-dimension limit without enlarging. Rounding up keeps
	// the estimate on the conservative side of the provider's integer rounding.
	if edge := float64(sizing.maxEdge); w > edge || h > edge {
		scale := math.Min(edge/w, edge/h)
		w = math.Min(edge, math.Ceil(w*scale))
		h = math.Min(edge, math.Ceil(h*scale))
	}
	patches := coveringPatches(w, h)
	if sizing.budget > 0 && patches > sizing.budget {
		shrink := math.Sqrt(float64(openAIImagePatchSize*openAIImagePatchSize*sizing.budget) / (w * h))
		scaledW := w * shrink / openAIImagePatchSize
		scaledH := h * shrink / openAIImagePatchSize
		adjusted := shrink * math.Min(math.Floor(scaledW)/scaledW, math.Floor(scaledH)/scaledH)
		patches = min(coveringPatches(math.Max(1, math.Floor(w*adjusted)), math.Max(1, math.Floor(h*adjusted))), sizing.budget)
	}
	// Above the rejection limit the provider refuses the image; never quote more.
	patches = min(patches, openAIImageRejectPatches)
	return patchTokensFromCount(patches, p.multiplier), nil
}

// coveringPatches returns the number of 32px patches that cover a w x h image.
func coveringPatches(w, h float64) int {
	return int(math.Ceil(w/openAIImagePatchSize) * math.Ceil(h/openAIImagePatchSize))
}

// patchTokensFromCount applies the model multiplier and rounds up, returning at least one token.
func patchTokensFromCount(patches int, multiplier float64) int {
	return max(int(math.Ceil(float64(patches)*multiplier)), 1)
}
