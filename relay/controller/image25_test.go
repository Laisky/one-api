package controller

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestGPTImage25TokenBuckets verifies normalized usage against the official token rates.
// Parameters: t is the test runner. Returns: none; missing dispatch or incorrect rates fail the test.
func TestGPTImage25TokenBuckets(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"gpt-image-2.5-sunburst",
		"gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-flare-2026-09-08",
	} {
		for _, tc := range []struct {
			label                    string
			text, image, cached, out int
			usd                      float64
		}{
			{label: "text", text: 1000, usd: 0.005},
			{label: "image", image: 1000, usd: 0.008},
			{label: "output", out: 1000, usd: 0.03},
			{label: "cached_text", text: 1000, cached: 1000, usd: 0.00125},
			{label: "cached_image", image: 1000, cached: 1000, usd: 0.002},
			{label: "mixed", text: 100, image: 340, out: 196, usd: 0.0091},
		} {
			t.Run(name+"/"+tc.label, func(t *testing.T) {
				usage := &relaymodel.Usage{
					PromptTokens:     tc.text + tc.image,
					CompletionTokens: tc.out,
					PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
						TextTokens: tc.text, ImageTokens: tc.image, CachedTokens: tc.cached,
					},
				}
				for _, group := range []float64{1, 1.5} {
					got := computeImageUsageQuota(name, usage, group)
					require.InDelta(t, tc.usd*billingratio.QuotaPerUsd*group, got, 1e-9)
				}
			})
		}
	}
	require.Zero(t, computeImageUsageQuota("gpt-image-2.5-sunburst", nil, 1))
	require.Zero(t, computeImageUsageQuota("gpt-image-2.5-sunburst", &relaymodel.Usage{}, 1))
	require.Zero(t, computeImageUsageQuota("gpt-image-unknown", &relaymodel.Usage{CompletionTokens: 1000}, 1))
}

// TestGPTImage25UsageReconciliation verifies parsing and token-only final billing together.
// Parameters: t is the test runner. Returns: none; a duplicate render fee fails the test.
func TestGPTImage25UsageReconciliation(t *testing.T) {
	t.Parallel()
	var upstream openai.ImageUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"input_tokens":440,"output_tokens":196,"total_tokens":636,
		"input_tokens_details":{"text_tokens":100,"image_tokens":340}
	}`), &upstream))
	usage := upstream.Convert2GeneralUsage()
	for _, name := range []string{
		"gpt-image-2.5-sunburst",
		"gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-flare-2026-09-08",
	} {
		cfg := openai.ModelRatios[name]
		require.NotNil(t, cfg.Image)
		perImage := cfg.Image.PricePerImageUsd > 0
		require.False(t, perImage)
		base := calculateImageBaseQuota(cfg.Image.PricePerImageUsd, cfg.Ratio, 1, 1, 1)
		require.Positive(t, base)
		summary := finalizeImageQuota(base, perImage, name, name, usage, 1)
		want := int64(math.Ceil(0.0091 * billingratio.QuotaPerUsd))
		require.Equal(t, want, summary.TotalQuota, name)
		require.Equal(t, want, summary.TokenQuota, name)
		require.InDelta(t, 0.0091*billingratio.QuotaPerUsd, summary.TokenQuotaFloat, 1e-9)
		require.NotEqual(t, base+want, summary.TotalQuota, "usage replaces the estimate instead of adding a render fee")
	}
}

// TestGPTImage25RequestDefaults verifies defaults and forwarding of new quality levels.
// Parameters: t is the test runner. Returns: none; stale Image 2 constraints fail the test.
func TestGPTImage25RequestDefaults(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"gpt-image-2.5-sunburst",
		"gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-flare-2026-09-08",
	} {
		cfg := openai.ModelRatios[name]
		require.NotNil(t, cfg.Image)
		req := &relaymodel.ImageRequest{Model: name, Prompt: "A lighthouse", N: 1}
		applyImageDefaults(req, cfg.Image)
		require.Equal(t, "auto", req.Size)
		require.Equal(t, "auto", req.Quality)
		require.Nil(t, validateImageRequest(req, nil, cfg.Image))
		for _, quality := range []string{"low", "medium", "high", "xhigh", "max", "auto"} {
			req.Quality = quality
			req.Size = "1536x864"
			require.Nil(t, validateImageRequest(req, nil, cfg.Image))
			multiplier, err := getImageCostRatio(req, cfg.Image)
			require.NoError(t, err)
			require.Equal(t, 1.0, multiplier, "size and quality costs come from actual token usage")
			upstream := buildOpenAIImageRequest(req)
			require.Equal(t, name, upstream.Model)
			require.Equal(t, quality, upstream.Quality)
			require.Equal(t, "1536x864", upstream.Size)
			require.Equal(t, 1, upstream.N)
		}
	}
}
