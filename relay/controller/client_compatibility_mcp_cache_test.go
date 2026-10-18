package controller

import (
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

// clientCompatibilityCacheRounds returns independent synthetic upstream receipts.
// The separate cache buckets make dropping later rounds observable in billed cost.
func clientCompatibilityCacheRounds() (*relaymodel.Usage, *relaymodel.Usage) {
	return &relaymodel.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18, ToolsCost: 2, PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 13}, CacheWrite5mTokens: 17, CacheWrite1hTokens: 19},
		&relaymodel.Usage{PromptTokens: 23, CompletionTokens: 29, TotalTokens: 52, ToolsCost: 3, PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 31}, CacheWrite5mTokens: 37, CacheWrite1hTokens: 41}
}

// TestClientCompatibilityMCPRoundCacheBilling settles two merged upstream rounds
// into the actual synthetic ledger and requires every cache bucket exactly once.
func TestClientCompatibilityMCPRoundCacheBilling(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	previousRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(previousRedis) })
	start := seedRetryDoubleChargeUser(t)
	c := setupClaudeRetryContext(t, httptest.NewRecorder(), "")
	requestID := "req_client_cache_rounds"
	responseNamespaceCleanLedger(t, requestID)
	c.Set(ctxkey.RequestId, requestID)
	m := meta.GetByContext(c)
	m.APIType = apitype.Anthropic
	m.ActualModelName = "claude-client-cache-fixture"
	m.OriginModelName = m.ActualModelName
	c.Set(ctxkey.RequestModel, m.ActualModelName)
	local := map[string]model.ModelConfigLocal{m.ActualModelName: {Ratio: 1, CompletionRatio: 2, CachedInputRatio: 3, CacheWrite5mRatio: 4, CacheWrite1hRatio: 5}}
	require.NoError(t, c.MustGet(ctxkey.ChannelModel).(*model.Channel).SetModelPriceConfigs(local))
	request := &ClaudeMessagesRequest{Model: m.ActualModelName, MaxTokens: 100}
	reservation, admissionErr := preConsumeClaudeMessagesQuota(c, request, 1000, 1, 2, m)
	require.Nil(t, admissionErr)
	require.Positive(t, reservation)
	markPreConsumed(c, reservation)
	first, second := clientCompatibilityCacheRounds()
	usage, mergeErr := mergeUsage(nil, first)
	require.NoError(t, mergeErr)
	usage, mergeErr = mergeUsage(usage, second)
	require.NoError(t, mergeErr)
	markBillingReconciled(c)
	charged := postConsumeClaudeMessagesQuotaWithTraceID(detachForBilling(c), requestID, "", usage, m, request, 1, reservation, 0, 1, nil, 1, local, nil)
	drainCriticalTasks(t)
	// 34 input + 36*2 output + 44*3 reads + 54*4 five-minute writes + 60*5 one-hour writes + 5 tools.
	require.EqualValues(t, 759, charged)
	require.Equal(t, charged, start-reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, charged, start-token.RemainQuota)
	require.Equal(t, 34, usage.PromptTokens)
	require.Equal(t, 36, usage.CompletionTokens)
	require.Equal(t, 70, usage.TotalTokens)
	require.Equal(t, 44, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 54, usage.CacheWrite5mTokens)
	require.Equal(t, 60, usage.CacheWrite1hTokens)
	require.EqualValues(t, 5, usage.ToolsCost)
}

// TestClientCompatibilityMCPRoundCacheIsolation checks nil receipts and ownership.
// Merging later receipts must not mutate earlier provider detail snapshots.
func TestClientCompatibilityMCPRoundCacheIsolation(t *testing.T) {
	t.Parallel()
	first, second := clientCompatibilityCacheRounds()
	merged, mergeErr := mergeUsage(nil, first)
	require.NoError(t, mergeErr)
	unchanged, mergeErr := mergeUsage(merged, nil)
	require.NoError(t, mergeErr)
	require.Same(t, merged, unchanged)
	merged, mergeErr = mergeUsage(merged, second)
	require.NoError(t, mergeErr)
	require.Equal(t, 13, first.PromptTokensDetails.CachedTokens)
	require.Equal(t, 31, second.PromptTokensDetails.CachedTokens)
	require.Equal(t, 44, merged.PromptTokensDetails.CachedTokens)
	merged.PromptTokensDetails.CachedTokens++
	require.Equal(t, 13, first.PromptTokensDetails.CachedTokens)
	require.Equal(t, 31, second.PromptTokensDetails.CachedTokens)
}
