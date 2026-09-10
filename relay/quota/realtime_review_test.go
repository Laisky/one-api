package quota_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewMixedCacheRetainsMeasuredUsage checks the production collector-to-quota
// path. Missing cache allocation is uncertain pricing, not a free response.
func TestReviewMixedCacheRetainsMeasuredUsage(t *testing.T) {
	for _, tc := range []struct {
		name, details string
		want          int64
		incomplete    bool
	}{
		{"ambiguous_audio", `"text_tokens":100,"audio_tokens":100,"cached_tokens":100`, 300, true},
		{"explicit_audio", `"text_tokens":100,"audio_tokens":100,"cached_tokens":100,"cached_tokens_details":{"audio_tokens":100}`, 300, false},
		{"ambiguous_image", `"text_tokens":100,"image_tokens":100,"cached_tokens":100`, 305, true},
		{"pure_audio", `"audio_tokens":200,"cached_tokens":100`, 1700, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := realtime.NewLedger()
			frame := []byte(`{"type":"response.done","response":{"id":"r1","usage":{"input_tokens":200,"output_tokens":10,"input_token_details":{` + tc.details + `}}}}`)
			_ = ledger.Observe(frame)
			ledger.Finish()
			input := quota.ComputeInput{ModelName: "gpt-realtime", ModelRatio: 2, GroupRatio: 1,
				PricingAdaptor: &openai.Adaptor{}, Usage: &model.Usage{Realtime: ledger}}
			got := quota.Compute(input)
			require.Equal(t, 200, got.PromptTokens, "the receipt must survive uncertain cache allocation")
			require.Equal(t, 10, got.CompletionTokens)
			require.Equal(t, 100, got.CachedPromptTokens)
			require.Equal(t, tc.want, got.TotalQuota, "ambiguous cases use the minimum cost consistent with reported totals")
			require.Equal(t, tc.incomplete, len(ledger.Issues) > 0)
			_ = ledger.Observe(frame)
			require.Equal(t, got.TotalQuota, quota.Compute(input).TotalQuota, "replaying an uncertain receipt must not charge twice")
		})
	}
}

// TestReviewMissingMediaPriceKeepsKnownBuckets proves that an unknown image rate
// cannot erase separately priced text/audio or another valid receipt.
func TestReviewMissingMediaPriceKeepsKnownBuckets(t *testing.T) {
	name := "custom-realtime-alias"
	input := quota.ComputeInput{ModelName: name, ModelRatio: 2, GroupRatio: 1,
		ChannelModelConfigs: map[string]modelcfg.ModelConfigLocal{
			name: {Ratio: 2, CompletionRatio: 4, Audio: &modelcfg.AudioPricingLocal{PromptRatio: 8, CompletionRatio: 2}},
		},
		Usage: &model.Usage{Realtime: &realtime.Ledger{InputTokens: 300, OutputTokens: 10,
			Records: []realtime.Record{{Tokens: realtime.Tokens{Input: 300, Text: 100, Audio: 100, Image: 100, Output: 10, OutputText: 10}}}}},
	}
	got := quota.Compute(input)
	require.NotEmpty(t, got.BillingIssues, "unknown image price must remain visible")
	require.Equal(t, int64(1880), got.TotalQuota, "200 text + 1600 audio + 80 output; unknown image is not claimed as exact")
	input.Usage.Realtime.Records = append(input.Usage.Realtime.Records, realtime.Record{Tokens: realtime.Tokens{Input: 10, Text: 10}})
	input.Usage.Realtime.InputTokens += 10
	got = quota.Compute(input)
	require.NotEmpty(t, got.BillingIssues)
	require.Equal(t, int64(1900), got.TotalQuota)
}
