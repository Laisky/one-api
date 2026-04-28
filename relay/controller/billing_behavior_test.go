package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// cloneUsage returns a deep-enough copy of usage for billing tests so helper
// functions can mutate or read it independently.
func cloneUsage(usage *relaymodel.Usage) *relaymodel.Usage {
	if usage == nil {
		return nil
	}
	clone := *usage
	if usage.PromptTokensDetails != nil {
		details := *usage.PromptTokensDetails
		clone.PromptTokensDetails = &details
	}
	return &clone
}

// TestPostConsumeQuotaParityAcrossAPIs verifies Chat, Response API, and Claude
// post-billing paths all resolve to the same quota as the shared calculator.
func TestPostConsumeQuotaParityAcrossAPIs(t *testing.T) {
	testCases := []struct {
		name            string
		modelName       string
		usage           *relaymodel.Usage
		modelRatio      float64
		groupRatio      float64
		completionRatio float64
	}{
		{
			name:            "basic_text_usage",
			modelName:       "gpt-4o-mini",
			usage:           &relaymodel.Usage{PromptTokens: 18, CompletionTokens: 35, TotalTokens: 53},
			modelRatio:      1.25,
			groupRatio:      1,
			completionRatio: 2,
		},
		{
			name:      "usage_with_tool_cost",
			modelName: "gpt-4o-mini",
			usage: &relaymodel.Usage{
				PromptTokens:     25,
				CompletionTokens: 40,
				TotalTokens:      65,
				ToolsCost:        9,
			},
			modelRatio:      1.4,
			groupRatio:      0.8,
			completionRatio: 1.5,
		},
		{
			name:      "claude_cached_prompt_usage",
			modelName: "claude-test-model",
			usage: &relaymodel.Usage{
				PromptTokens:        120,
				CompletionTokens:    45,
				TotalTokens:         165,
				CacheWrite5mTokens:  10,
				CacheWrite1hTokens:  5,
				PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 70},
			},
			modelRatio:      1.8,
			groupRatio:      1.1,
			completionRatio: 2.2,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			completionOverrides := map[string]float64{tc.modelName: tc.completionRatio}
			expected := quotautil.Compute(quotautil.ComputeInput{
				Usage:                  cloneUsage(tc.usage),
				ModelName:              tc.modelName,
				ModelRatio:             tc.modelRatio,
				GroupRatio:             tc.groupRatio,
				ChannelCompletionRatio: completionOverrides,
			}).TotalQuota

			meta := &metalib.Meta{StartTime: time.Now(), APIType: -1, ChannelType: -1}

			chatQuota := postConsumeQuota(
				context.Background(),
				cloneUsage(tc.usage),
				meta,
				&relaymodel.GeneralOpenAIRequest{Model: tc.modelName},
				0,
				0,
				0,
				tc.modelRatio,
				nil,
				tc.groupRatio,
				false,
				nil,
				completionOverrides,
			)

			responseQuota := postConsumeResponseAPIQuota(
				context.Background(),
				cloneUsage(tc.usage),
				meta,
				&openai.ResponseAPIRequest{Model: tc.modelName},
				0,
				tc.modelRatio,
				nil,
				tc.groupRatio,
				nil,
				completionOverrides,
			)

			claudeQuota := postConsumeClaudeMessagesQuotaWithTraceID(
				context.Background(),
				"",
				"",
				cloneUsage(tc.usage),
				meta,
				&ClaudeMessagesRequest{Model: tc.modelName},
				0,
				0,
				0,
				tc.modelRatio,
				nil,
				tc.groupRatio,
				nil,
				completionOverrides,
			)

			require.Equal(t, expected, chatQuota)
			require.Equal(t, expected, responseQuota)
			require.Equal(t, expected, claudeQuota)
		})
	}
}

// TestUpdateMCPRequestCostEstimateDoesNotDoubleCountTools verifies provisional
// MCP request-cost snapshots reuse the shared total quota without adding tool cost twice.
func TestUpdateMCPRequestCostEstimateDoesNotDoubleCountTools(t *testing.T) {
	ensureResponseFallbackFixtures(t)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/mcp", nil)
	requestID := fmt.Sprintf("test-mcp-request-cost-%d", time.Now().UnixNano())
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.RequestId, requestID)

	usage := &relaymodel.Usage{
		PromptTokens:     12,
		CompletionTokens: 8,
		TotalTokens:      20,
		ToolsCost:        11,
	}
	completionOverrides := map[string]float64{"gpt-4o-mini": 2}

	updateMCPRequestCostEstimate(c, &metalib.Meta{UserId: fallbackUserID}, usage, "gpt-4o-mini", 1.5, nil, 1, nil, completionOverrides, nil)

	recorded, err := model.GetCostByRequestId(requestID)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.UserRequestCost{}).Error)
	})

	expected := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  cloneUsage(usage),
		ModelName:              "gpt-4o-mini",
		ModelRatio:             1.5,
		GroupRatio:             1,
		ChannelCompletionRatio: completionOverrides,
	}).TotalQuota

	require.Equal(t, expected, recorded.Quota)
	baseOnly := int64((float64(usage.PromptTokens) + float64(usage.CompletionTokens)*2) * 1.5)
	require.Equal(t, baseOnly+usage.ToolsCost, recorded.Quota)
}
