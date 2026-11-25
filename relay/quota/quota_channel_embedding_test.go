package quota_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// TestComputeChannelEmbeddingOverride checks that a channel's own embedding
// block prices a multimodal embedding receipt outside any time window, as it
// already did inside one: modality token ratios and per-unit prices apply
// instead of the plain input ratio. Parameters: t runs the cases. Returns: none.
func TestComputeChannelEmbeddingOverride(t *testing.T) {
	t.Parallel()

	const modelName = "channel-embedding-model"
	for _, tc := range []struct {
		name      string
		embedding *model.EmbeddingPricingLocal
		details   *relaymodel.UsagePromptTokensDetails
		want      int64
	}{
		{
			// 10 image tokens at the channel's image ratio 2, not ratio 1.
			name:      "modality_token_ratio",
			embedding: &model.EmbeddingPricingLocal{TextTokenRatio: 1, ImageTokenRatio: 2},
			details:   &relaymodel.UsagePromptTokensDetails{ImageTokens: 10},
			want:      20,
		},
		{
			// Three images at $0.001 each (500 quota per image) on top of the
			// 10 text tokens: an unpriced image must not be free.
			name:      "per_image_price",
			embedding: &model.EmbeddingPricingLocal{TextTokenRatio: 1, UsdPerImage: 0.001},
			details:   &relaymodel.UsagePromptTokensDetails{TextTokens: 10, ImageCount: 3},
			want:      10 + 3*500,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := quotautil.Compute(quotautil.ComputeInput{
				Usage:      &relaymodel.Usage{PromptTokens: 10, PromptTokensDetails: tc.details},
				ModelName:  modelName,
				ModelRatio: 1,
				GroupRatio: 1,
				ChannelModelConfigs: map[string]model.ModelConfigLocal{
					modelName: {Ratio: 1, Embedding: tc.embedding},
				},
				RequestTime: time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
			})
			require.Equal(t, tc.want, result.TotalQuota)
		})
	}
}
