package tooling

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

func TestConservativeDeepSeekToolFloorAndControls(t *testing.T) {
	for _, tc := range []struct {
		name, base   string
		channel      int
		expectedCost int64
	}{
		{"deepseek_paid", "https://api.deepseek.com", channeltype.DeepSeek, 400},
		{"third_party_same_model", "https://provider.example", channeltype.OpenAI, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channel}
			require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{
				"web_search": {QuotaPerCall: 17},
				"web_fetch":  {QuotaPerCall: 23},
			}}))
			meta := &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, ActualModelName: "deepseek-v4-pro"}
			quotes, err := BuiltinToolQuotes(context.Background(), meta.ActualModelName, meta, channel, nil, map[string]struct{}{"web_search": {}, "web_fetch": {}})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(nil)
			RecordConservativeDeepSeekTools(c, meta, quotes, map[string]int64{"web_search": 2, "web_fetch": 4})
			require.Equal(t, tc.expectedCost, ConservativeDeepSeekToolQuota(c))
			if tc.channel == channeltype.DeepSeek {
				RecordConservativeDeepSeekTools(c, meta, quotes, map[string]int64{"web_search": 20})
				require.EqualValues(t, 570, ConservativeDeepSeekToolQuota(c))
				again := map[string]int{"web_search": 3, "web_fetch": 11}
				require.True(t, conservativeDeepSeekCounts(c, again))
				require.Equal(t, 20, again["web_search"])
				require.Equal(t, 11, again["web_fetch"])
			}
		})
	}
}

func TestConservativeDeepSeekUnsolicitedAndZeroPrice(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	counts := map[string]int{"web_search": 3, "web_fetch": 2, "file_search": 1}
	require.False(t, conservativeDeepSeekCounts(c, counts))
	require.Equal(t, map[string]int{"file_search": 1}, counts)
	zero := map[string]int64{"web_search": 0}
	meta := &metalib.Meta{ChannelType: channeltype.DeepSeek, BaseURL: "https://api.deepseek.com"}
	RecordConservativeDeepSeekTools(c, meta, zero, nil)
	require.Zero(t, ConservativeDeepSeekToolQuota(c))
}
