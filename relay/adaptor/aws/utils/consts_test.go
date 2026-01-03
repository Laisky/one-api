package utils

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

func TestGetRegionPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		region   string
		expected string
	}{
		{"US East 1", "us-east-1", "us"},
		{"US West 2", "us-west-2", "us"},
		{"Canada Central", "ca-central-1", "us"},
		{"EU West 1", "eu-west-1", "eu"},
		{"EU Central", "eu-central-1", "eu"},
		{"Asia Pacific Southeast", "ap-southeast-1", "apac"},
		{"Asia Pacific Northeast", "ap-northeast-1", "jp"},
		{"US Government East", "us-gov-east-1", "us-gov"},
		{"US Government West", "us-gov-west-1", "us-gov"},
		{"South America", "sa-east-1", "us"},
		{"Unknown region", "unknown-region-1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getRegionPrefix(tt.region)
			require.Equalf(t, tt.expected, result, "getRegionPrefix(%s)", tt.region)
		})
	}
}

func TestConvertModelID2CrossRegionProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name     string
		model    string
		region   string
		expected string
	}{
		{
			name:     "US region with supported model",
			model:    "anthropic.claude-3-haiku-20240307-v1:0",
			region:   "us-east-1",
			expected: "us.anthropic.claude-3-haiku-20240307-v1:0",
		},
		{
			name:     "EU region with supported model",
			model:    "anthropic.claude-3-sonnet-20240229-v1:0",
			region:   "eu-west-1",
			expected: "eu.anthropic.claude-3-sonnet-20240229-v1:0",
		},
		{
			name:     "APAC region with supported model",
			model:    "anthropic.claude-3-5-sonnet-20240620-v1:0",
			region:   "ap-southeast-1",
			expected: "apac.anthropic.claude-3-5-sonnet-20240620-v1:0",
		},
		{
			name:     "Japan region prefers JP profile when available",
			model:    "anthropic.claude-haiku-4-5-20251001-v1:0",
			region:   "ap-northeast-1",
			expected: "global.anthropic.claude-haiku-4-5-20251001-v1:0",
		},
		{
			name:     "Global profile when source region is allowed",
			model:    "anthropic.claude-sonnet-4-20250514-v1:0",
			region:   "us-west-2",
			expected: "global.anthropic.claude-sonnet-4-20250514-v1:0",
		},
		{
			name:     "Global profile falls back to geography when source region unsupported",
			model:    "anthropic.claude-sonnet-4-20250514-v1:0",
			region:   "eu-central-1",
			expected: "eu.anthropic.claude-sonnet-4-20250514-v1:0",
		},
		{
			name:     "Australian region prefers AU prefix when available",
			model:    "anthropic.claude-sonnet-4-5-20250929-v1:0",
			region:   "ap-southeast-2",
			expected: "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
		},
		{
			name:     "Unsupported model returns original",
			model:    "unsupported.model-v1:0",
			region:   "us-east-1",
			expected: "unsupported.model-v1:0",
		},
		{
			name:     "Unsupported region returns original",
			model:    "anthropic.claude-3-haiku-20240307-v1:0",
			region:   "unknown-region-1",
			expected: "anthropic.claude-3-haiku-20240307-v1:0",
		},
		// Claude Opus 4.5 - requires global inference profile
		{
			name:     "claude-opus-4-5 in us-east-1 uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "us-east-1",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in us-west-2 uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "us-west-2",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in eu-west-1 uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "eu-west-1",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in ap-northeast-1 (Tokyo) uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "ap-northeast-1",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in ap-southeast-1 (Singapore) uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "ap-southeast-1",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in ap-southeast-2 (Sydney) uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "ap-southeast-2",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in sa-east-1 (Sao Paulo) uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "sa-east-1",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
		{
			name:     "claude-opus-4-5 in ca-central-1 (Canada) uses global profile",
			model:    "anthropic.claude-opus-4-5-20251101-v1:0",
			region:   "ca-central-1",
			expected: "global.anthropic.claude-opus-4-5-20251101-v1:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ConvertModelID2CrossRegionProfile(ctx, tt.model, tt.region)
			require.Equalf(t, tt.expected, result, "ConvertModelID2CrossRegionProfile(%s, %s)", tt.model, tt.region)
		})
	}
}

func TestUpdateRegionHealthMetrics(t *testing.T) {
	t.Parallel()
	region := "us-east-1"

	// Test successful operation
	UpdateRegionHealthMetrics(region, true, 100*time.Millisecond, nil)
	health := GetRegionHealth(region)

	require.True(t, health.IsHealthy, "region should be healthy after successful operation")
	require.Zero(t, health.ErrorCount, "error count should remain zero after success")
	require.Equal(t, 100*time.Millisecond, health.AvgLatency, "average latency should match successful operation")

	// Test failed operation
	testErr := errors.New("test error")
	UpdateRegionHealthMetrics(region, false, 0, testErr)
	health = GetRegionHealth(region)

	require.Equal(t, 1, health.ErrorCount, "error count should increment after failure")
	require.NotNil(t, health.LastError, "last error should be recorded")

	// Test multiple failures to trigger unhealthy status
	for range 3 {
		UpdateRegionHealthMetrics(region, false, 0, testErr)
	}
	health = GetRegionHealth(region)

	require.False(t, health.IsHealthy, "region should transition to unhealthy after repeated failures")
}

func TestConvertModelID2CrossRegionProfileWithFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	model := "anthropic.claude-3-haiku-20240307-v1:0"
	region := "us-east-1"

	// Test with nil client (should return cross-region profile for best effort)
	result := ConvertModelID2CrossRegionProfileWithFallback(ctx, model, region, nil)
	expected := "us.anthropic.claude-3-haiku-20240307-v1:0"
	require.Equal(t, expected, result, "fallback conversion should return cross-region profile when available")

	// Test that static conversion works independently
	staticResult := ConvertModelID2CrossRegionProfile(ctx, model, region)
	require.Equal(t, expected, staticResult, "static conversion should match fallback conversion")
}

func TestRegionMapping(t *testing.T) {
	t.Parallel()
	// Test that all regions in RegionMapping have valid prefixes
	for region, prefixes := range RegionMapping {
		require.NotEmptyf(t, prefixes, "RegionMapping entry for %s should define at least one prefix", region)

		actualPrefix := getRegionPrefix(region)
		require.Equalf(t, prefixes[0], actualPrefix, "primary prefix mismatch for region %s", region)

		for _, prefix := range prefixes {
			require.NotEmptyf(t, prefix, "RegionMapping entry for %s contains an empty prefix", region)
		}
	}
}

func TestCrossRegionInferencesValidation(t *testing.T) {
	t.Parallel()
	// Test that all cross-region inference models have valid prefixes
	validPrefixes := map[string]bool{
		"us":     true,
		"us-gov": true,
		"eu":     true,
		"apac":   true,
		"global": true,
		"ca":     true,
		"jp":     true,
		"au":     true,
	}

	for _, modelID := range CrossRegionInferences {
		parts := strings.SplitN(modelID, ".", 2)
		require.Lenf(t, parts, 2, "invalid cross-region model ID format: %s", modelID)

		prefix := parts[0]
		require.Truef(t, validPrefixes[prefix], "invalid prefix %s in model ID: %s", prefix, modelID)
	}
}

// TestGlobalProfileSourceRegionsConsistency verifies that all models in GlobalProfileSourceRegions
// have corresponding entries in CrossRegionInferences with the global prefix.
func TestGlobalProfileSourceRegionsConsistency(t *testing.T) {
	t.Parallel()
	for modelID := range GlobalProfileSourceRegions {
		globalProfileID := "global." + modelID
		require.Containsf(t, CrossRegionInferences, globalProfileID,
			"model %s in GlobalProfileSourceRegions should have corresponding global profile %s in CrossRegionInferences",
			modelID, globalProfileID)
	}
}

// TestClaudeOpus45RequiresGlobalProfile specifically tests that claude-opus-4-5 is configured
// correctly to use the global inference profile, as AWS requires this for on-demand invocation.
func TestClaudeOpus45RequiresGlobalProfile(t *testing.T) {
	t.Parallel()
	modelID := "anthropic.claude-opus-4-5-20251101-v1:0"
	globalProfileID := "global." + modelID

	// Verify model is in GlobalProfileSourceRegions
	allowedRegions, exists := GlobalProfileSourceRegions[modelID]
	require.Truef(t, exists, "claude-opus-4-5 (%s) must be in GlobalProfileSourceRegions for AWS Bedrock on-demand invocation", modelID)

	// Verify global profile exists in CrossRegionInferences
	require.Containsf(t, CrossRegionInferences, globalProfileID,
		"claude-opus-4-5 global profile (%s) must be in CrossRegionInferences", globalProfileID)

	// Verify key regions are in the allowed list
	keyRegions := []string{
		"us-east-1",
		"us-west-2",
		"eu-west-1",
		"ap-northeast-1", // Tokyo - the user's region from the issue
		"ap-southeast-1",
	}
	for _, region := range keyRegions {
		require.Containsf(t, allowedRegions, region,
			"region %s should be in GlobalProfileSourceRegions for claude-opus-4-5", region)
	}

	// Test actual conversion works for each key region
	ctx := context.Background()
	for _, region := range keyRegions {
		result := ConvertModelID2CrossRegionProfile(ctx, modelID, region)
		require.Equalf(t, globalProfileID, result,
			"claude-opus-4-5 in region %s should convert to global profile %s", region, globalProfileID)
	}
}

func BenchmarkConvertModelID2CrossRegionProfile(b *testing.B) {
	ctx := context.Background()
	model := "anthropic.claude-3-haiku-20240307-v1:0"
	region := "us-east-1"

	for b.Loop() {
		ConvertModelID2CrossRegionProfile(ctx, model, region)
	}
}

func BenchmarkGetRegionPrefix(b *testing.B) {
	region := "us-east-1"

	for b.Loop() {
		getRegionPrefix(region)
	}
}
