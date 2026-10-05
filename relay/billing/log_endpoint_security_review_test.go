package billing

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestReviewBillingSanitizesBothEndpointSources observes the real billing sink,
// independently covering both sources and direct-field precedence while pinning
// exact charges and preserving caller-owned metadata.
func TestReviewBillingSanitizesBothEndpointSources(t *testing.T) {
	const metadataURL = "https://metadata-user:fixture-metadata-secret@metadata.example/v1/chat?credential=fixture-metadata-secret#metadata-fragment"
	const directURL = "https://direct-user:fixture-direct-secret@direct.example/v1/responses?credential=fixture-direct-secret#direct-fragment"
	for _, tc := range []struct {
		name             string
		metadataEndpoint string
		directEndpoint   string
		wantEndpoint     string
	}{
		{"metadata-only", metadataURL, "", "https://metadata.example/v1/chat"},
		{"direct-only", "", directURL, "https://direct.example/v1/responses"},
		{"direct-overrides-metadata", metadataURL, directURL, "https://direct.example/v1/responses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := model.LogMetadata{"preserved": true}
			if tc.metadataEndpoint != "" {
				metadata[model.LogMetadataKeyUpstreamEndpoint] = tc.metadataEndpoint
			}
			before := maps.Clone(metadata)
			original := postConsumeQuotaWithLogFn
			t.Cleanup(func() { postConsumeQuotaWithLogFn = original })
			var got *model.Log
			calls := 0
			postConsumeQuotaWithLogFn = func(ctx context.Context, token int, delta, total int64, entry *model.Log, provisional ...int) {
				calls++
				require.Equal(t, 123, token)
				require.Equal(t, int64(7), delta)
				require.Equal(t, int64(19), total)
				got = entry
			}
			detail := QuotaConsumeDetail{Ctx: context.Background(), TokenId: 123, UserId: 1, ChannelId: 2,
				QuotaDelta: 7, TotalQuota: 19, PromptTokens: 11, CompletionTokens: 13, ModelName: "gpt-4",
				TokenName: "fixture-token", StartTime: time.Now(), Metadata: metadata,
				UpstreamEndpoint: tc.directEndpoint}
			PostConsumeQuotaDetailed(detail)
			require.Equal(t, 1, calls, "the real billing sink must be invoked exactly once")
			require.NotNil(t, got)
			require.Equal(t, tc.wantEndpoint, got.Metadata[model.LogMetadataKeyUpstreamEndpoint])
			require.Equal(t, true, got.Metadata["preserved"])
			require.Equal(t, 11, got.PromptTokens)
			require.Equal(t, 13, got.CompletionTokens)
			require.Equal(t, before, metadata, "billing must not mutate caller-owned metadata")
		})
	}
}
