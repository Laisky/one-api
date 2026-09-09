package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGPTImage25ReviewWireUsage exercises the provider JSON boundary rather than
// constructing already-normalized usage. Parameters: t runs the cases. Returns: none.
func TestGPTImage25ReviewWireUsage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body string
		usd  float64
	}{
		{"uncached_control", `{"input_tokens":2000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000}}`, 0.043},
		{"cached_text", `{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":0,"cached_tokens":1000}}`, 0.03125},
		{"cached_image", `{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":0,"image_tokens":1000,"cached_tokens":1000}}`, 0.032},
		{"only_images_cached", `{"input_tokens":2000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000,"cached_tokens":1000,"cached_tokens_details":{"text_tokens":0,"image_tokens":1000}}}`, 0.037},
		{"only_text_cached", `{"input_tokens":2000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000,"cached_tokens":1000,"cached_tokens_details":{"text_tokens":1000,"image_tokens":0}}}`, 0.03925},
		{"unequal_cache_buckets", `{"input_tokens":1200,"output_tokens":100,"input_tokens_details":{"text_tokens":200,"image_tokens":1000,"cached_tokens":400,"cached_tokens_details":{"text_tokens":200,"image_tokens":200}}}`, 0.01005},
		{"totals_only", `{"input_tokens":1000,"output_tokens":1000}`, 0.035},
		{"partial_modality_details", `{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":100,"image_tokens":200}}`, 0.0356},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstream openai.ImageUsage
			require.NoError(t, json.Unmarshal([]byte(tc.body), &upstream))
			usage := upstream.Convert2GeneralUsage()
			for _, name := range []string{
				"gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08",
				"gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08",
			} {
				for _, group := range []float64{1, 1.5} {
					require.InDelta(t, tc.usd*billingratio.QuotaPerUsd*group,
						computeImageUsageQuota(name, usage, group), 1e-8, name)
				}
			}
		})
	}
}

// TestGPTImage25ReviewReservationIsNotAnExtraFee verifies token settlement with
// an operator-configured reserve. Parameters: t runs the assertion. Returns: none.
func TestGPTImage25ReviewReservationIsNotAnExtraFee(t *testing.T) {
	t.Parallel()
	var upstream openai.ImageUsage
	require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":0}}`), &upstream))
	got := finalizeImageQuota(50_000, true, "gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst", upstream.Convert2GeneralUsage(), 1)
	require.Equal(t, int64(17_500), got.TotalQuota, "the reserve is replaced, never added to actual token cost")
}

// TestGPTImage25ReviewIncompleteUsageKeepsConfiguredFallback covers absent,
// partial, and ambiguous provider usage. Parameters: t runs the cases. Returns: none.
func TestGPTImage25ReviewIncompleteUsageKeepsConfiguredFallback(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{}`, `null`,
		`{"input_tokens":1000,"output_tokens":0,"input_tokens_details":{"text_tokens":1000}}`,
		`{"input_tokens":2000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000,"cached_tokens":1000}}`,
	} {
		var upstream openai.ImageUsage
		require.NoError(t, json.Unmarshal([]byte(body), &upstream))
		got := finalizeImageQuota(50_000, true, "gpt-image-2.5-flare", "gpt-image-2.5-flare", upstream.Convert2GeneralUsage(), 1)
		require.Equal(t, int64(50_000), got.TotalQuota, body)
		require.Zero(t, got.TokenQuota, "an estimate must not masquerade as measured token billing")
	}
}
