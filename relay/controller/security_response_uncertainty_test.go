package controller

import (
	"context"
	"fmt"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestSecurityResponseUnknownUsageRetainsHold checks the existing settlement
// owner treats missing/unsupported stream evidence as uncertain, not measured zero.
func TestSecurityResponseUnknownUsageRetainsHold(t *testing.T) {
	for _, estimated := range []bool{false, true} {
		t.Run(fmt.Sprint(estimated), func(t *testing.T) {
			const balance = int64(1000)
			xaiVideoSetup(t, balance, false)
			require.NoError(t, model.PreConsumeTokenQuota(context.Background(), fallbackTokenID, 100))
			usage := &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}
			if estimated {
				usage.BillingEstimateReason = "response_stream_incomplete_or_missing_receipt"
			}
			meta := &metalib.Meta{UserId: fallbackUserID, TokenId: fallbackTokenID, ChannelId: fallbackChannelID, ActualModelName: "gpt-4o", OriginModelName: "gpt-4o"}
			quota := postConsumeResponseAPIQuota(context.Background(), usage, meta, &openai.ResponseAPIRequest{Model: "gpt-4o"}, 100, 1, nil, 1, map[string]model.ModelConfigLocal{"gpt-4o": {Ratio: 1, CompletionRatio: 1}}, nil)
			expected := int64(5)
			if estimated {
				expected = 100
			}
			require.Equal(t, expected, quota)
			require.Equal(t, balance-expected, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-expected, token.RemainQuota)
		})
	}
}
