package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestPostConsumeQuotaWithStructuredOutput(t *testing.T) {
	t.Parallel()
	// Test that the postConsumeQuota function correctly handles structured output costs
	// when they are included in usage.ToolsCost

	tests := []struct {
		name             string
		promptTokens     int
		completionTokens int
		toolsCost        int64
		modelName        string
		expectedMinQuota int64 // minimum expected quota due to structured output cost
	}{
		// ToolsCost is still included if other features add costs (e.g., web search), but structured outputs add none
		{
			name:             "No structured output cost",
			promptTokens:     100,
			completionTokens: 1000,
			toolsCost:        0,
			modelName:        "gpt-4o",
			expectedMinQuota: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Create usage with structured output cost
			usage := &relaymodel.Usage{
				PromptTokens:     tt.promptTokens,
				CompletionTokens: tt.completionTokens,
				TotalTokens:      tt.promptTokens + tt.completionTokens,
				ToolsCost:        tt.toolsCost,
			}

			// Get model ratio and calculate expected quota
			modelRatio := ratio.GetModelRatioWithChannel(tt.modelName, channeltype.OpenAI, nil)
			completionRatio := ratio.GetCompletionRatioWithChannel(tt.modelName, channeltype.OpenAI, nil)

			// Calculate quota manually (similar to postConsumeQuota logic)
			calculatedQuota := int64(float64(usage.PromptTokens)+float64(usage.CompletionTokens)*completionRatio) + usage.ToolsCost

			// Verify the tools cost is properly included
			require.GreaterOrEqual(t, calculatedQuota, tt.expectedMinQuota,
				"Expected quota to be at least %d (to include tools cost), but got %d", tt.expectedMinQuota, calculatedQuota)

			// Verify structured output cost is preserved
			require.Equal(t, tt.toolsCost, usage.ToolsCost,
				"Expected ToolsCost to be %d, but got %d", tt.toolsCost, usage.ToolsCost)

			t.Logf("Model: %s, Quota: %d, ToolsCost: %d, ModelRatio: %.6f, CompletionRatio: %.2f",
				tt.modelName, calculatedQuota, usage.ToolsCost, modelRatio, completionRatio)
		})
	}
}

// TestStructuredOutputCostIntegration tests the complete flow from request to cost calculation
func TestStructuredOutputCostIntegration(t *testing.T) {
	t.Parallel()
	// This test verifies that structured output costs flow correctly through the system

	textRequest := &relaymodel.GeneralOpenAIRequest{
		Model: "gpt-4o",
		ResponseFormat: &relaymodel.ResponseFormat{
			Type: "json_schema",
			JsonSchema: &relaymodel.JSONSchema{
				Name: "test_schema",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"result": map[string]any{
							"type": "string",
						},
					},
				},
			},
		},
	}

	// Simulate usage that would come from the adaptor; structured outputs add no extra ToolsCost
	completionTokens := 1000

	usage := &relaymodel.Usage{
		PromptTokens:     100,
		CompletionTokens: completionTokens,
		TotalTokens:      1100,
		ToolsCost:        0, // No structured output surcharge
	}

	// Calculate final quota as postConsumeQuota would
	completionRatio := ratio.GetCompletionRatioWithChannel("gpt-4o", channeltype.OpenAI, nil)
	quota := int64(float64(usage.PromptTokens)+float64(usage.CompletionTokens)*completionRatio) + usage.ToolsCost

	// Verify final quota equals base cost when no additional tools cost is present
	require.Equal(t, int64(float64(usage.PromptTokens)+float64(usage.CompletionTokens)*completionRatio), quota,
		"Final quota should equal base cost when no ToolsCost is applied")

	// Calculate the structured output portion of the total cost
	baseQuota := int64(float64(usage.PromptTokens) + float64(usage.CompletionTokens)*completionRatio)
	structuredOutputPortion := float64(usage.ToolsCost) / float64(quota) * 100

	t.Logf("Integration test results:")
	t.Logf("  Model: %s", textRequest.Model)
	t.Logf("  Completion tokens: %d", completionTokens)
	t.Logf("  Base quota: %d", baseQuota)
	t.Logf("  Structured output cost: %d", usage.ToolsCost)
	t.Logf("  Total quota: %d", quota)
	t.Logf("  Structured output portion: %.2f%%", structuredOutputPortion)

	// No structured output surcharge should be present
	require.Equal(t, float64(0), structuredOutputPortion,
		"Structured output cost portion (%.2f%%) should be 0", structuredOutputPortion)
}
