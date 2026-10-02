package billing

import (
	"context"
	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestReviewBillingSanitizesBothEndpointSources observes the real billing sink,
// including metadata supplied directly by callers, while pinning exact charges.
func TestReviewBillingSanitizesBothEndpointSources(t *testing.T) {
	original := postConsumeQuotaWithLogFn
	t.Cleanup(func() { postConsumeQuotaWithLogFn = original })
	for _, direct := range []bool{false, true} {
		raw := "https://user:fixture-secret@example.com/v1/chat?credential=fixture-secret"
		metadata := model.LogMetadata{model.LogMetadataKeyUpstreamEndpoint: raw, "preserved": true}
		var got *model.Log
		postConsumeQuotaWithLogFn = func(ctx context.Context, token int, delta, total int64, entry *model.Log, provisional ...int) {
			require.Equal(t, 123, token)
			require.Equal(t, int64(7), delta)
			require.Equal(t, int64(19), total)
			got = entry
		}
		detail := QuotaConsumeDetail{Ctx: context.Background(), TokenId: 123, UserId: 1, ChannelId: 2,
			QuotaDelta: 7, TotalQuota: 19, PromptTokens: 11, CompletionTokens: 13, ModelName: "gpt-4",
			TokenName: "fixture-token", StartTime: time.Now(), Metadata: metadata}
		if direct {
			detail.UpstreamEndpoint = raw
		}
		PostConsumeQuotaDetailed(detail)
		require.NotNil(t, got)
		require.Equal(t, "https://example.com/v1/chat", got.Metadata[model.LogMetadataKeyUpstreamEndpoint])
		require.Equal(t, true, got.Metadata["preserved"])
		require.Equal(t, 11, got.PromptTokens)
		require.Equal(t, 13, got.CompletionTokens)
		require.Equal(t, raw, metadata[model.LogMetadataKeyUpstreamEndpoint])
	}
}
