package quota

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestGPT61TerraUnpricedCacheWriteReceipt verifies that unsupported paid
// retention is flagged as unpriced, never silently settled as Standard.
func TestGPT61TerraUnpricedCacheWriteReceipt(t *testing.T) {
	receipt := model.ParseResponseUsage([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":8000,"output_tokens":10,"input_tokens_details":{"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":0,"ttl_12h_tokens":7000}}}}}`), 0, nil)
	require.NotNil(t, receipt)
	result := Compute(ComputeInput{Usage: receipt, ModelName: "gpt-6.1-terra", ModelRatio: 1, GroupRatio: 1})
	require.True(t, result.UnpricedUsage, "a paid 12h receipt without a configured tariff must retain its reservation")
	require.Positive(t, result.TotalQuota)
}

// TestGPT61TerraUnverifiedActualServiceTier checks that provider-reported
// non-Standard usage cannot be considered fully priced by the Standard contract.
func TestGPT61TerraUnverifiedActualServiceTier(t *testing.T) {
	input := ComputeInput{
		Usage:     &model.Usage{PromptTokens: 200, CompletionTokens: 50, ServiceTier: "fast"},
		ModelName: "gpt-6.1-terra", ModelRatio: 1, GroupRatio: 1,
	}
	require.True(t, Compute(input).UnpricedUsage)
	input.Usage.ServiceTier = "default"
	require.False(t, Compute(input).UnpricedUsage, "supported Standard receipts must retain historical pricing")
}

// TestServiceTierUnverifiedReceipt checks that a receipt whose actual tier
// could not be read (Usage.ServiceTierUnverified) is not priced as Standard by
// a tier-restricted tariff, while models without a tier restriction keep their
// exact price. Parameter t reports failures; nothing returns.
func TestServiceTierUnverifiedReceipt(t *testing.T) {
	for _, tc := range []struct {
		modelName string
		unpriced  bool
	}{
		{modelName: "gpt-6.1-terra", unpriced: true},
		{modelName: "gpt-6-cyber", unpriced: true},
		{modelName: "gpt-4o-mini", unpriced: false},
	} {
		t.Run(tc.modelName, func(t *testing.T) {
			input := ComputeInput{
				Usage:     &model.Usage{PromptTokens: 200, CompletionTokens: 50, ServiceTier: "default", ServiceTierUnverified: true},
				ModelName: tc.modelName, ModelRatio: 1, GroupRatio: 1,
			}
			result := Compute(input)
			require.Equal(t, tc.unpriced, result.UnpricedUsage)
			require.Positive(t, result.TotalQuota, "the priced part is still computed for the estimate floor")
			input.Usage.ServiceTierUnverified = false
			require.False(t, Compute(input).UnpricedUsage, "a verified Standard receipt keeps its exact price")
		})
	}
}
