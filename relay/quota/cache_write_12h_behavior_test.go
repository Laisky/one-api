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
