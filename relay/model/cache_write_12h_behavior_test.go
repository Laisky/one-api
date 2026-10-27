package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageTerraTwelveHourReceiptIsNotOrdinaryWrite reproduces an
// actual provider receipt whose 12h writes must not be charged at the 30m rate.
func TestResponseUsageTerraTwelveHourReceiptIsNotOrdinaryWrite(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":8000,"output_tokens":10,"input_tokens_details":{"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":0,"ttl_12h_tokens":7000}}}}}`)
	usage := ParseResponseUsage(body, 0, nil)
	require.NotNil(t, usage)
	require.Zero(t, usage.CacheWrite5mTokens, "12h receipts must not be mislabeled as ordinary 30m writes")
}

// TestResponseUsageTerraMixedCacheWriteLifetimes prevents aggregate writes
// from being added to the individually priced 30m and 12h sub-buckets.
func TestResponseUsageTerraMixedCacheWriteLifetimes(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":9000,"output_tokens":10,"cache_write_tokens":7000,"input_tokens_details":{"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":2000,"ttl_12h_tokens":5000}}}}}`)
	usage := ParseResponseUsage(body, 0, nil)
	require.NotNil(t, usage)
	require.Equal(t, 2000, usage.CacheWrite5mTokens)
}

// TestResponseUsageTerraNestedServiceTierPreserved requires authoritative
// nested Responses service_tier to reach settlement and audit.
func TestResponseUsageTerraNestedServiceTierPreserved(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"service_tier":"fast","usage":{"input_tokens":200,"output_tokens":50}}}`)
	usage := ParseResponseUsage(body, 0, nil)
	require.NotNil(t, usage)
	require.Equal(t, "fast", usage.ServiceTier)
}

// TestResponseUsageTerraLegacyCacheWritePreservation guards existing providers
// whose top-level write tokens and legacy 5m bucket are genuinely additive.
func TestResponseUsageTerraLegacyCacheWritePreservation(t *testing.T) {
	usage := &Usage{CacheWriteTokens: 300, CacheWrite5mTokens: 20}
	usage.NormalizeCacheWriteTokens()
	require.Equal(t, 320, usage.CacheWrite5mTokens)
	require.Zero(t, usage.CacheWriteTokens)
}
