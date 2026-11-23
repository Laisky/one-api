package openai

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGPTImageCalculatorMatchesPublishedGPTImage2Prices pins the restated
// calculator formula to OpenAI's independently published GPT Image 2 per-image
// prices (USD, rounded to 3 decimals, at USD 30 per million output tokens).
// Parameters: t is the test handle. Returns: none.
func TestGPTImageCalculatorMatchesPublishedGPTImage2Prices(t *testing.T) {
	published := map[string]map[string]float64{
		"low":    {"1024x1024": 0.006, "1024x1536": 0.005, "1536x1024": 0.005},
		"medium": {"1024x1024": 0.053, "1024x1536": 0.041, "1536x1024": 0.041},
		"high":   {"1024x1024": 0.211, "1024x1536": 0.165, "1536x1024": 0.165},
	}
	for quality, sizes := range published {
		for size, usd := range sizes {
			width, height, ok := parseGPTImageCalculatorSize(size)
			require.True(t, ok, size)
			tokens := gptImageCalculatorTokens("gpt-image-2", quality, width, height)
			got := math.Round(float64(tokens)*30/1e6*1000) / 1000
			require.InDelta(t, usd, got, 1e-12, "%s %s -> %d tokens", quality, size, tokens)
		}
	}
}

// TestEstimateGPTImageOutputTokens checks explicit cells, the 2.6 quality
// factors, and the worst-case resolution of inputs the calculator does not
// price. Parameters: t is the test handle. Returns: none.
func TestEstimateGPTImageOutputTokens(t *testing.T) {
	for _, tc := range []struct {
		model, size, quality string
		tokens               int
	}{
		{"gpt-image-2.5-flare", "1024x1024", "low", 196},
		{"gpt-image-2.5-sunburst", "1024x1024", "medium", 439},
		{"gpt-image-2.6-flare", "1024x1024", "low", 823},        // round(196 * 4.2)
		{"gpt-image-2.6-sunburst", "1024x1024", "medium", 1449}, // round(439 * 3.3)
		{"gpt-image-2.6-flare", "1024x1024", "high", 1756},
		{"gpt-image-2.5-flare-2026-09-08", "1536x1024", "xhigh", 2459},
		{"gpt-image-2.5-flare", "1024x1536", "xhigh", 2459},
		{"gpt-image-2.5-sunburst-2026-09-08", "1024x1024", "max", 7024},
		{"gpt-image-2.6-sunburst", "3840x2160", "max", 13342},
		{"gpt-image-2.5-sunburst", "2880x2880", "max", 23719},
		// 64 * 720 / 2048 = 22.5 exactly: half-to-even gives d = 22, so
		// ceil(64 * 22 * 3,474,560 / 4e6) = 1,224 (half-up would give 1,279).
		{"gpt-image-2.5-flare", "2048x720", "xhigh", 1224},
		{"gpt-image-2.6-flare", "720x2048", "xhigh", 1224},
		// Unpriced inputs resolve to the most expensive case.
		{"gpt-image-2.5-sunburst", "auto", "auto", 23719},
		{"gpt-image-2.6-flare", "", "", 23719},
		{"gpt-image-2.5-flare", "1024x1024", "auto", 7024},
		{"gpt-image-2.5-flare", "1024x1024", "standard", 7024},
		{"gpt-image-2.6-flare", "auto", "low", 2768},
		{"gpt-image-2.5-flare", "4096x4096", "high", 5930},
		{"gpt-image-2.5-flare", "3840x1024", "high", 5930}, // aspect above 3:1
		{"gpt-image-2.5-flare", "big", "high", 5930},
		{"gpt-image-2.5-flare", "0x1024", "high", 5930},
	} {
		got, ok := EstimateGPTImageOutputTokens(tc.model, tc.size, tc.quality)
		require.True(t, ok, tc.model)
		require.Equal(t, tc.tokens, got, "%s %s %s", tc.model, tc.size, tc.quality)
	}
	for _, model := range []string{"gpt-image-2", "gpt-image-1.5", "dall-e-3", "gpt-image-2.6-sunburst-2026-11-04"} {
		_, ok := EstimateGPTImageOutputTokens(model, "1024x1024", "high")
		require.False(t, ok, "%s is outside the calculator-backed reservation policy", model)
	}
}

// TestGPTImageCalculatorWorstCaseSize proves the hardcoded 2880x2880
// worst case by enumerating every documented valid output size (edges in
// multiples of 16, at most 3840, aspect at most 3:1, 655,360 to 8,294,400
// pixels) for every family and quality. Parameters: t is the test handle.
// Returns: none.
func TestGPTImageCalculatorWorstCaseSize(t *testing.T) {
	_, _, ok := parseGPTImageCalculatorSize(fmt.Sprintf("%dx%d", gptImageWorstCaseEdge, gptImageWorstCaseEdge))
	require.True(t, ok, "the worst case must itself be a valid size")
	for family, bases := range gptImageCalculatorBases {
		for quality := range bases {
			worst := gptImageCalculatorTokens(family, quality, gptImageWorstCaseEdge, gptImageWorstCaseEdge)
			for width := 16; width <= gptImageMaxEdge; width += 16 {
				for height := 16; height <= gptImageMaxEdge; height += 16 {
					pixels := width * height
					if pixels < 655_360 || pixels > gptImageMaxPixels || max(width, height) > gptImageMaxAspect*min(width, height) {
						continue
					}
					got := gptImageCalculatorTokens(family, quality, width, height)
					require.LessOrEqual(t, got, worst, "%s %s %dx%d", family, quality, width, height)
				}
			}
		}
	}
}
