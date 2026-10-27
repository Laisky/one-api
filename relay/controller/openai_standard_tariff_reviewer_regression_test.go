package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestGPT61TerraReviewerRejectsPresentNonStringTTL reproduces the review
// finding against the actual outgoing Responses normalization boundary.
// A present null TTL must not silently become a default cache duration.
func TestGPT61TerraReviewerRejectsPresentNonStringTTL(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"top_level_null", "{\"model\":\"gpt-6.1-terra\",\"input\":\"hi\",\"prompt_cache_options\":{\"ttl\":null}}"},
		{"input_null", "{\"model\":\"gpt-6.1-terra\",\"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"hello\",\"prompt_cache_breakpoint\":{\"ttl\":null}}]}]}"},
		{"tools_null", "{\"model\":\"gpt-6.1-terra\",\"input\":\"hi\",\"tools\":[{\"type\":\"function\",\"name\":\"lookup\",\"description\":\"A small lookup tool\",\"parameters\":{\"type\":\"object\",\"properties\":{}},\"prompt_cache_breakpoint\":{\"ttl\":null}}]}"},
		{"prompt_null", "{\"model\":\"gpt-6.1-terra\",\"prompt\":{\"id\":\"pmpt_cache_test\",\"prompt_cache_breakpoint\":{\"ttl\":null}}}"},
		{"tools_numeric", "{\"model\":\"gpt-6.1-terra\",\"input\":\"hi\",\"tools\":[{\"type\":\"function\",\"name\":\"lookup\",\"description\":\"A small lookup tool\",\"parameters\":{\"type\":\"object\",\"properties\":{}},\"prompt_cache_breakpoint\":{\"ttl\":3600}}]}"},
		{"prompt_paid", "{\"model\":\"gpt-6.1-terra\",\"prompt\":{\"id\":\"pmpt_cache_test\",\"prompt_cache_breakpoint\":{\"ttl\":\"12h\"}}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.body)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			_, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
			require.Error(t, err, "unsupported or malformed cache TTL reached provider dispatch")
		})
	}
}

// TestGPT61TerraReviewerPreservesNonControlData distinguishes structural
// provider cache controls from user-supplied tool schemas and prompt variables.
// Valid default/30m cache controls must remain admissible.
func TestGPT61TerraReviewerPreservesNonControlData(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"tool_parameters_are_schema", "{\"model\":\"gpt-6.1-terra\",\"input\":\"hello\",\"tools\":[{\"type\":\"function\",\"name\":\"lookup\",\"description\":\"lookup\",\"parameters\":{\"type\":\"object\",\"properties\":{\"subject\":{\"type\":\"string\",\"prompt_cache_breakpoint\":{\"ttl\":\"12h\"}}}}}]}"},
		{"prompt_variables_are_data", "{\"model\":\"gpt-6.1-terra\",\"prompt\":{\"id\":\"pmpt_cache_test\",\"variables\":{\"subject\":{\"prompt_cache_breakpoint\":{\"ttl\":\"12h\"}}}}}"},
		{"input_default_ttl", "{\"model\":\"gpt-6.1-terra\",\"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"hi\",\"prompt_cache_breakpoint\":{}}]}]}"},
		{"input_empty_ttl", "{\"model\":\"gpt-6.1-terra\",\"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"hi\",\"prompt_cache_breakpoint\":{\"ttl\":\"\"}}]}]}"},
		{"input_30m_casefold", "{\"model\":\"gpt-6.1-terra\",\"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"hi\",\"prompt_cache_breakpoint\":{\"ttl\":\" 30M \"}}]}]}"},
		{"tools_30m", "{\"model\":\"gpt-6.1-terra\",\"input\":\"hi\",\"tools\":[{\"type\":\"function\",\"name\":\"lookup\",\"description\":\"A small lookup tool\",\"parameters\":{\"type\":\"object\",\"properties\":{}},\"prompt_cache_breakpoint\":{\"ttl\":\"30m\"}}]}"},
		{"prompt_30m", "{\"model\":\"gpt-6.1-terra\",\"prompt\":{\"id\":\"pmpt_cache_test\",\"prompt_cache_breakpoint\":{\"ttl\":\"30m\"}}}"},
		{"ordinary_text_literal", "{\"model\":\"gpt-6.1-terra\",\"input\":\"literal prompt_cache_breakpoint TTL 12h must not be interpreted\"}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.body)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			_, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
			require.NoError(t, err, "valid data or audited 30m control was rejected")
		})
	}
}

// TestGPT61TerraReviewerProviderIsolation protects the existing
// non-native-provider request policy from native Terra-only billing checks.
func TestGPT61TerraReviewerProviderIsolation(t *testing.T) {
	raw := []byte("{\"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"hi\",\"prompt_cache_breakpoint\":{\"ttl\":null}}]}]}")
	for _, channel := range []int{channeltype.Azure, channeltype.OpenAICompatible, channeltype.Groq} {
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &root))
		changed, err := normalizeOpenAITerraStandardPricing(root, "gpt-6.1-terra", channel)
		require.NoError(t, err)
		require.False(t, changed)
	}
}
