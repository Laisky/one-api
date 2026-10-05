package openai

import (
	"context"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"strings"
)

// isClaudeVisionModel reports whether name belongs to a native Claude vision model family.
func isClaudeVisionModel(name string) bool {
	return strings.HasPrefix(name, "claude-") || strings.HasPrefix(name, "sonnet") || strings.HasPrefix(name, "haiku") || strings.HasPrefix(name, "opus")
}

// imageEstimationFallback returns a conservative admission allowance within the supported model estimator's image envelope.
// It does not represent measured usage; an authoritative provider receipt determines final billing.
func imageEstimationFallback(name string) int {
	if isClaudeVisionModel(name) {
		// The legacy Claude geometry estimator caps each edge at 1568 pixels.
		return 3279 // ceil(1568 * 1568 / 750).
	}
	if strings.HasPrefix(name, "gemini-") {
		// Gemini 3's largest documented per-image resolution allowance. Older model
		// estimates retain at least their existing geometric reservation envelope.
		// https://ai.google.dev/gemini-api/docs/media-resolution#token-counts
		return 2240
	}
	if strings.HasPrefix(name, "gpt-4o-mini") {
		return gpt4oMiniAdditionalCost + 8*gpt4oMiniHighDetailCost
	}
	// The supported OpenAI tile estimator fits images inside 2048 pixels and
	// scales the shorter edge to 768, yielding at most eight 512-pixel tiles.
	base, tile := getVisionBaseTile(name)
	return base + 8*tile
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
