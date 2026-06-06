package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUsageTopLevelCachedTokensUnmarshal verifies that a top-level
// usage.cached_tokens field (as emitted by StepFun) is captured into the
// dedicated CachedTokens field rather than being silently dropped.
func TestUsageTopLevelCachedTokensUnmarshal(t *testing.T) {
	t.Parallel()

	// StepFun-shaped usage: cached_tokens lives at the top level alongside
	// prompt_tokens, with no prompt_tokens_details block.
	payload := []byte(`{"cached_tokens":512,"prompt_tokens":591,"completion_tokens":120,"total_tokens":711}`)

	var usage Usage
	require.NoError(t, json.Unmarshal(payload, &usage))

	require.Equal(t, 512, usage.CachedTokens)
	require.Equal(t, 591, usage.PromptTokens)
	require.Nil(t, usage.PromptTokensDetails)
}

// TestUsageNormalizeCachedTokensStepFun verifies that a StepFun-shaped usage
// snapshot (top-level cached_tokens only) has the value promoted into the
// nested PromptTokensDetails.CachedTokens field after normalization.
func TestUsageNormalizeCachedTokensStepFun(t *testing.T) {
	t.Parallel()

	usage := Usage{
		PromptTokens:     591,
		CompletionTokens: 120,
		TotalTokens:      711,
		CachedTokens:     512,
	}

	usage.NormalizeCachedTokens()

	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 512, usage.PromptTokensDetails.CachedTokens)
	require.Zero(t, usage.CachedTokens)
}

// TestUsageNormalizeCachedTokensOpenAIUnchanged verifies that an OpenAI-shaped
// usage snapshot (nested prompt_tokens_details.cached_tokens, no top-level
// cached_tokens) is left untouched by normalization.
func TestUsageNormalizeCachedTokensOpenAIUnchanged(t *testing.T) {
	t.Parallel()

	usage := Usage{
		PromptTokens:     591,
		CompletionTokens: 120,
		TotalTokens:      711,
		PromptTokensDetails: &UsagePromptTokensDetails{
			CachedTokens: 256,
		},
	}

	usage.NormalizeCachedTokens()

	require.NotNil(t, usage.PromptTokensDetails)
	// Nested value must be preserved exactly; top-level remained zero so it
	// must not overwrite the upstream-reported nested count.
	require.Equal(t, 256, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 0, usage.CachedTokens)
}

// TestUsageNormalizeCachedTokensPrefersNested verifies that when both a
// top-level and a nested cached-token count are present, the nested value is
// preserved (providers that already populate the nested field are authoritative).
func TestUsageNormalizeCachedTokensPrefersNested(t *testing.T) {
	t.Parallel()

	usage := Usage{
		PromptTokens: 591,
		CachedTokens: 512,
		PromptTokensDetails: &UsagePromptTokensDetails{
			CachedTokens: 256,
		},
	}

	usage.NormalizeCachedTokens()

	require.Equal(t, 256, usage.PromptTokensDetails.CachedTokens)
	require.Zero(t, usage.CachedTokens)
}

// TestErrorTypeJSONRoundTrip verifies that ErrorType values marshal and
// unmarshal as their underlying string representations.
func TestErrorTypeJSONRoundTrip(t *testing.T) {
	t.Parallel()
	t.Helper()

	original := Error{
		Message: "something went wrong",
		Type:    ErrorTypeOneAPI,
		Code:    "example_code",
	}

	payload, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Error
	require.NoError(t, json.Unmarshal(payload, &decoded))

	require.Equal(t, original.Type, decoded.Type)
	require.Equal(t, original.Message, decoded.Message)
	require.Equal(t, original.Code, decoded.Code)
}
