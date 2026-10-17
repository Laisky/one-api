package controller

import (
	"math"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClientCompatibilityMCPMergePreservesUncertainty requires later uncertain
// work to retain its established reason and strongest existing quote floor.
// Floors can cover overlapping quotes, so this test does not invent their sum.
func TestClientCompatibilityMCPMergePreservesUncertainty(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                          string
		beforeFloor, afterFloor, want int64
	}{
		{name: "later_estimate", afterFloor: 1200, want: 1200},
		{name: "earlier_floor_is_stronger", beforeFloor: 2000, afterFloor: 1200, want: 2000},
		{name: "later_floor_is_stronger", beforeFloor: 1200, afterFloor: 2000, want: 2000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := &relaymodel.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18, BillingEstimateQuotaFloor: tc.beforeFloor}
			if tc.beforeFloor > 0 {
				first.BillingEstimateReason = "earlier_round_estimated"
			}
			later := &relaymodel.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8, BillingEstimateReason: "later_round_estimated", BillingEstimateQuotaFloor: tc.afterFloor}
			merged, mergeErr := mergeUsage(first, later)
			require.NoError(t, mergeErr)
			require.NotEmpty(t, merged.BillingEstimateReason)
			require.Equal(t, tc.want, merged.BillingEstimateQuotaFloor)
		})
	}
}

// TestClientCompatibilityMCPLaterEstimateRetainsReservation exercises actual
// settlement after a measured round is followed by an uncertain accepted round.
// Dropping that uncertainty would refund a reservation without a complete receipt.
func TestClientCompatibilityMCPLaterEstimateRetainsReservation(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	previousRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(previousRedis) })
	start := seedRetryDoubleChargeUser(t)
	c := setupClaudeRetryContext(t, httptest.NewRecorder(), "")
	requestID := "req_client_later_estimate"
	responseNamespaceCleanLedger(t, requestID)
	c.Set(ctxkey.RequestId, requestID)
	m := meta.GetByContext(c)
	m.APIType = apitype.Anthropic
	m.ActualModelName = "claude-client-uncertainty-fixture"
	m.OriginModelName = m.ActualModelName
	local := map[string]model.ModelConfigLocal{m.ActualModelName: {Ratio: 1, CompletionRatio: 2}}
	require.NoError(t, c.MustGet(ctxkey.ChannelModel).(*model.Channel).SetModelPriceConfigs(local))
	request := &ClaudeMessagesRequest{Model: m.ActualModelName, MaxTokens: 100}
	reservation, admissionErr := preConsumeClaudeMessagesQuota(c, request, 1000, 1, 2, m)
	require.Nil(t, admissionErr)
	require.EqualValues(t, 1200, reservation)
	markPreConsumed(c, reservation)
	usage, mergeErr := mergeUsage(&relaymodel.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, &relaymodel.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8, BillingEstimateReason: "later_round_estimated", BillingEstimateQuotaFloor: reservation})
	require.NoError(t, mergeErr)
	markBillingReconciled(c)
	charged := postConsumeClaudeMessagesQuotaWithTraceID(detachForBilling(c), requestID, "", usage, m, request, 1, reservation, 0, 1, nil, 1, local, nil)
	drainCriticalTasks(t)
	require.Equal(t, reservation, charged)
	require.Equal(t, charged, start-reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, charged, start-token.RemainQuota)
}

// TestClientCompatibilityMCPMergeRejectsOverflow requires invalid cross-round
// sums to be rejected, rather than wrapping or inventing a saturated charge.
// The baseline returned a wrapped receipt; the checked API must now report an
// explicit error and preserve the earlier valid accumulator atomically.
func TestClientCompatibilityMCPMergeRejectsOverflow(t *testing.T) {
	t.Parallel()
	large := math.MaxInt/2 + 1
	for _, tc := range []struct {
		name  string
		usage func() *relaymodel.Usage
	}{
		{name: "input", usage: func() *relaymodel.Usage { return &relaymodel.Usage{PromptTokens: large} }},
		{name: "output", usage: func() *relaymodel.Usage { return &relaymodel.Usage{CompletionTokens: large} }},
		{name: "total", usage: func() *relaymodel.Usage { return &relaymodel.Usage{TotalTokens: large} }},
		{name: "cache_read", usage: func() *relaymodel.Usage {
			return &relaymodel.Usage{PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: large}}
		}},
		{name: "cache_write", usage: func() *relaymodel.Usage { return &relaymodel.Usage{CacheWriteTokens: large} }},
		{name: "cache_write_5m", usage: func() *relaymodel.Usage { return &relaymodel.Usage{CacheWrite5mTokens: large} }},
		{name: "cache_write_1h", usage: func() *relaymodel.Usage { return &relaymodel.Usage{CacheWrite1hTokens: large} }},
		{name: "tools", usage: func() *relaymodel.Usage { return &relaymodel.Usage{ToolsCost: math.MaxInt64/2 + 1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, next := tc.usage(), tc.usage()
			before := *first
			merged, mergeErr := mergeUsage(first, next)
			require.Error(t, mergeErr)
			require.Nil(t, merged, "unrepresentable cross-round usage must be rejected before pricing")
			require.Equal(t, before, *first, "failed merge must not partially mutate the accumulator")
		})
	}
}
