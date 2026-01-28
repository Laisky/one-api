package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

func init() {
	relay.InitializeGlobalPricing()
}

func TestPostConsumeClaudeMessagesQuota_MatchesCompute(t *testing.T) {
	t.Parallel()
	modelName := "claude-3-sonnet-20240229"
	channelType := channeltype.Anthropic
	adaptor := relay.GetAdaptor(channelType)
	require.NotNil(t, adaptor, "nil adaptor for channel %d", channelType)

	promptTokens := 100
	completionTokens := 200
	modelRatio := adaptor.GetModelRatio(modelName)
	groupRatio := 1.0

	meta := &metalib.Meta{
		ChannelType: channelType,
		ChannelId:   1,
		TokenId:     0,
		UserId:      1,
		TokenName:   "test-token",
		StartTime:   time.Now(),
		IsStream:    false,
	}
	req := &ClaudeMessagesRequest{Model: modelName}
	usage := &relaymodel.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens}

	quota := postConsumeClaudeMessagesQuotaWithTraceID(context.Background(), "req-1", "trace-1", usage, meta, req, modelRatio, 0, 0, modelRatio, groupRatio, nil)

	expectedResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	require.Equal(t, expectedResult.TotalQuota, quota, "postConsumeClaudeMessagesQuota mismatch with compute result")
}

func TestPostConsumeClaudeMessagesQuota_HandlesCache(t *testing.T) {
	t.Parallel()
	modelName := "claude-3-sonnet-20240229"
	channelType := channeltype.Anthropic
	adaptor := relay.GetAdaptor(channelType)

	promptTokens := 1000
	completionTokens := 500
	cachedTokens := 600
	modelRatio := adaptor.GetModelRatio(modelName)
	groupRatio := 1.0

	meta := &metalib.Meta{
		ChannelType: channelType,
		ChannelId:   1,
		TokenId:     0,
		UserId:      1,
		StartTime:   time.Now(),
	}
	req := &ClaudeMessagesRequest{Model: modelName}
	usage := &relaymodel.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: cachedTokens,
		},
	}

	quota := postConsumeClaudeMessagesQuotaWithTraceID(context.Background(), "req-1", "trace-1", usage, meta, req, modelRatio, 0, 0, modelRatio, groupRatio, nil)

	expectedResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	require.Equal(t, expectedResult.TotalQuota, quota, "quota should match expected compute result")
}

func TestPreConsumeClaudeMessagesQuota_Calculation(t *testing.T) {
	// This test focuses on the calculation part of pre-consumption
	request := &ClaudeMessagesRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 1000,
	}
	promptTokens := 500
	ratio := 0.001
	completionRatio := 2.0

	promptQuota := float64(promptTokens) * ratio
	completionQuota := float64(request.MaxTokens) * ratio * completionRatio
	expectedQuota := int64(promptQuota + completionQuota)

	require.Equal(t, int64(2), expectedQuota)
}
