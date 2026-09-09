package quota_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewConfiguredImageCacheDoesNotRequireFamilyName checks that the existing
// channel image-price contract remains usable under a custom model name.
func TestReviewConfiguredImageCacheDoesNotRequireFamilyName(t *testing.T) {
	name := "custom-image-realtime"
	ledger := realtime.NewLedger()
	require.NoError(t, ledger.Observe([]byte(`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":110,"output_tokens":10,"input_token_details":{"text_tokens":10,"image_tokens":100,"cached_tokens":40,"cached_tokens_details":{"image_tokens":40}}}}}`)))
	got := quota.Compute(quota.ComputeInput{ModelName: name, ModelRatio: 2, GroupRatio: 1,
		ChannelModelConfigs: map[string]modelcfg.ModelConfigLocal{
			name: {Ratio: 2, CompletionRatio: 4, Image: &modelcfg.ImagePricingLocal{PromptRatio: 1.25}},
		}, Usage: &model.Usage{Realtime: ledger}})
	require.Empty(t, got.BillingIssues, "a resolved image cache rate must not be rejected by the family-name gate")
	require.Equal(t, int64(260), got.TotalQuota, "20 text + 150 image misses + 10 image cache + 80 output")
	require.Equal(t, 40, got.CachedPromptTokens)
}
