package openai

import (
	"context"
	"math"
	"regexp"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
)

const (
	// claudeLegacyMaxImageEdge is the long-edge resize limit of Claude models
	// before Opus 4.7 and Sonnet 5.
	claudeLegacyMaxImageEdge = 1568
	// claudeHighResMaxImageEdge and claudeHighResMaxImageTokens bound the
	// high-resolution image input introduced with Opus 4.7 and Sonnet 5.
	// Source: https://platform.claude.com/docs/en/build-with-claude/vision
	claudeHighResMaxImageEdge   = 2576
	claudeHighResMaxImageTokens = 4784
	// claudeImagePixelsPerToken is Claude's documented image token approximation.
	claudeImagePixelsPerToken = 750.0
	// gemini3DefaultImageTokens is Gemini 3's documented per-image allocation
	// when media_resolution is unspecified, which the adapters never set.
	// Source: https://ai.google.dev/gemini-api/docs/media-resolution#token-counts
	gemini3DefaultImageTokens = 1120
	// geminiImageFallbackTokens is Gemini 3's largest documented per-image allocation.
	geminiImageFallbackTokens = 2240
)

// claudeLegacyVisionPattern matches Claude model IDs that predate high-resolution
// image input: Claude 2/3 and the Claude 4.0-4.6 Opus, Sonnet, and Haiku models,
// including dated and Vertex "@" snapshot suffixes. Unknown or newer Claude IDs
// fall outside it and receive the larger high-resolution reservation.
var claudeLegacyVisionPattern = regexp.MustCompile(`^claude-(2|3|instant)|^claude-(opus|sonnet|haiku)-4(-[0-6])?($|@|-\d{8})`)

// newerGPTPattern matches OpenAI GPT model IDs newer than the documented
// families, which receive the most expansive documented patch profile.
var newerGPTPattern = regexp.MustCompile(`^gpt-(5\.([7-9]|[1-9]\d)|[7-9]|[1-9]\d)`)

// isClaudeVisionModel reports whether name belongs to a native Claude vision model family.
func isClaudeVisionModel(name string) bool {
	return strings.HasPrefix(name, "claude-") || strings.HasPrefix(name, "sonnet") || strings.HasPrefix(name, "haiku") || strings.HasPrefix(name, "opus")
}

// claudeImageLimits returns the long-edge resize limit and the per-image token
// cap (zero when uncapped) that the Claude geometry estimator applies to name.
func claudeImageLimits(name string) (maxEdge float64, maxTokens int) {
	if claudeLegacyVisionPattern.MatchString(strings.ToLower(name)) {
		return claudeLegacyMaxImageEdge, 0
	}
	return claudeHighResMaxImageEdge, claudeHighResMaxImageTokens
}

// countClaudeImageTokens approximates Claude image tokens for width x height
// after the model's long-edge resize, returning at least one token.
func countClaudeImageTokens(width, height int, name string) int {
	maxEdge, maxTokens := claudeImageLimits(name)
	w, h := float64(width), float64(height)
	if longest := math.Max(w, h); longest > maxEdge {
		scale := maxEdge / longest
		w *= scale
		h *= scale
	}
	tokens := max(int(math.Ceil((w*h)/claudeImagePixelsPerToken)), 1)
	if maxTokens > 0 {
		tokens = min(tokens, maxTokens)
	}
	return tokens
}

// isOpenAITileVisionModel reports whether name is an OpenAI tile-based vision
// model whose documented low detail costs only the base tokens. Only these
// models may receive the low-detail discount; other providers process the
// complete image regardless of the OpenAI-only hint.
func isOpenAITileVisionModel(name string) bool {
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "openai/")
	if name == "gpt-5" || strings.HasPrefix(name, "gpt-5-") || strings.HasPrefix(name, "gpt-5.1") {
		return true
	}
	for _, prefix := range []string{"gpt-4o", "chatgpt-4o", "gpt-4.1", "gpt-4.5", "gpt-4-turbo", "gpt-4-vision", "o1", "o3", "computer-use-preview"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// imageEstimationFallback returns a conservative admission allowance for one
// image of model name with the requested detail when its dimensions or detail
// cannot be measured locally. It covers the largest documented processing for
// that detail (any detail when unknown) or the supported estimator's envelope.
// It does not represent measured usage; an authoritative provider receipt determines final billing.
func imageEstimationFallback(name, detail string) int {
	if profile, ok := resolveOpenAIPatchProfile(name); ok {
		return profile.maxTokens(detail)
	}
	if isClaudeVisionModel(name) {
		maxEdge, maxTokens := claudeImageLimits(name)
		if maxTokens > 0 {
			return maxTokens
		}
		return countClaudeImageTokens(int(maxEdge), int(maxEdge), name)
	}
	if strings.HasPrefix(name, "gemini-") {
		// Older Gemini estimates retain at least their geometric reservation envelope.
		return geminiImageFallbackTokens
	}
	if strings.HasPrefix(name, "gpt-4o-mini") {
		return gpt4oMiniAdditionalCost + 8*gpt4oMiniHighDetailCost
	}
	// The supported OpenAI tile estimator fits images inside 2048 pixels and
	// scales the shorter edge to 768, yielding at most eight 512-pixel tiles.
	base, tile := getVisionBaseTile(name)
	return base + 8*tile
}

// resolveOpenAIPatchProfile returns the documented patch profile of name, or
// the most expansive documented profile for GPT models newer than the guide.
func resolveOpenAIPatchProfile(name string) (patchProfile, bool) {
	if profile, ok := openAIPatchProfile(name); ok {
		return profile, true
	}
	if newerGPTPattern.MatchString(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "openai/")) {
		return patchProfileGPT6, true
	}
	return patchProfile{}, false
}

// imageDetailForProvider removes an OpenAI-only resolution hint when the routed adapter sends an unchanged full image, returning the effective detail.
func imageDetailForProvider(ctx context.Context, detail string) string {
	if ctx == nil {
		return detail
	}
	m, _ := ctx.Value(ctxkey.Meta).(*meta.Meta)
	if c, ok := ctx.Value(gmw.CtxKeyGin).(*gin.Context); ok && c != nil {
		if value, exists := c.Get(ctxkey.Meta); exists {
			m, _ = value.(*meta.Meta)
		}
	}
	if m == nil {
		return detail
	}
	switch m.APIType {
	case apitype.Gemini, apitype.VertexAI, apitype.Anthropic, apitype.AwsClaude:
		return "high"
	}
	return detail
}

// EstimateImageTokens returns the admission estimate for one image part that
// will be forwarded to model. url is the image URL or data URL, or empty for a
// file reference the gateway cannot inspect; detail is the caller's hint.
// The routed provider's semantics decide whether detail applies, and an image
// that cannot be measured keeps a conservative nonzero allowance instead of
// being admitted for free. It returns the estimated prompt tokens for the image.
func EstimateImageTokens(ctx context.Context, url, detail, model string) int {
	detail = imageDetailForProvider(ctx, detail)
	tokens, err := countImageTokens(url, detail, model)
	if err == nil {
		return tokens
	}
	fallback := imageEstimationFallback(model, detail)
	gmw.GetLogger(ctx).Warn("using conservative image token allowance",
		zap.Error(err),
		zap.String("model", model),
		zap.Bool("data_url", strings.HasPrefix(url, "data:")),
		zap.Int("reserved_tokens", fallback),
	)
	return fallback
}
