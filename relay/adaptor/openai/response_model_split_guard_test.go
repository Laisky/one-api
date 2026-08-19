package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

func TestResponseAPIInput_MarshalUnmarshal(t *testing.T) {
	t.Run("unmarshal string", func(t *testing.T) {
		var input ResponseAPIInput
		err := json.Unmarshal([]byte(`"hello"`), &input)
		require.NoError(t, err)
		require.Len(t, input, 1)
		require.Equal(t, "hello", input[0])
	})

	t.Run("unmarshal array", func(t *testing.T) {
		var input ResponseAPIInput
		err := json.Unmarshal([]byte(`["a", {"role":"user"}]`), &input)
		require.NoError(t, err)
		require.Len(t, input, 2)
		require.Equal(t, "a", input[0])
		_, ok := input[1].(map[string]any)
		require.True(t, ok)
	})

	t.Run("marshal single string becomes string", func(t *testing.T) {
		input := ResponseAPIInput{"hello"}
		b, err := json.Marshal(input)
		require.NoError(t, err)
		require.Equal(t, `"hello"`, string(b))
	})

	t.Run("marshal multi becomes array", func(t *testing.T) {
		input := ResponseAPIInput{"a", "b"}
		b, err := json.Marshal(input)
		require.NoError(t, err)
		require.Equal(t, `["a","b"]`, string(b))
	})
}

func TestResponseAPITool_JSONRoundTrip_Function(t *testing.T) {
	original := ResponseAPITool{
		Type:        "function",
		Name:        "lookup",
		Description: "Lookup city",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{"type": "string"},
			},
		},
		Function: &model.Function{
			Name:        "lookup",
			Description: "Lookup city",
			Parameters: map[string]any{
				"type": "object",
			},
			Arguments: "SHOULD_NOT_SERIALIZE",
		},
	}

	b, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))
	require.Equal(t, "function", decoded["type"])
	require.Equal(t, "lookup", decoded["name"])
	require.Equal(t, "Lookup city", decoded["description"])
	require.NotNil(t, decoded["function"], "expected function block")

	var roundTripped ResponseAPITool
	require.NoError(t, json.Unmarshal(b, &roundTripped))
	require.Equal(t, "function", roundTripped.Type)
	require.NotNil(t, roundTripped.Function)
	require.Equal(t, "lookup", roundTripped.Function.Name)
	require.Equal(t, "lookup", roundTripped.Name, "legacy name should sync")
	require.Equal(t, "Lookup city", roundTripped.Description, "legacy description should sync")
}

func TestToolCallIDConversion_RoundTrip(t *testing.T) {
	fcID, callID := convertToolCallIDToResponseAPI("abc")
	require.Equal(t, "fc_abc", fcID)
	require.Equal(t, "call_abc", callID)
	require.Equal(t, "abc", convertResponseAPIIDToToolCall(fcID, callID))

	fcID, callID = convertToolCallIDToResponseAPI("fc_abc")
	require.Equal(t, "fc_abc", fcID)
	require.Equal(t, "call_abc", callID)
	require.Equal(t, "abc", convertResponseAPIIDToToolCall(fcID, ""))

	fcID, callID = convertToolCallIDToResponseAPI("call_abc")
	require.Equal(t, "fc_abc", fcID)
	require.Equal(t, "call_abc", callID)
	require.Equal(t, "abc", convertResponseAPIIDToToolCall("", callID))
}

func TestResponseAPIInputTokensDetails_WebSearchInvocationCount(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int
	}{
		{"nil", nil, 0},
		{"int", 3, 3},
		{"string", " 2 ", 2},
		{"float", 2.2, 2},
		{"array sums", []any{1, 2}, 3},
		{"array fallback len", []any{"x", "y"}, 2},
		{"map request_count", map[string]any{"request_count": "4"}, 4},
		{"map nested", map[string]any{"web": map[string]any{"count": 5}}, 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &ResponseAPIInputTokensDetails{WebSearch: tc.raw}
			require.Equal(t, tc.want, d.WebSearchInvocationCount())
		})
	}
}

func TestResponseAPIInputTokensDetails_MarshalPreservesAdditional(t *testing.T) {
	var details ResponseAPIInputTokensDetails
	err := json.Unmarshal([]byte(`{"cached_tokens":1,"web_search":{"count":2},"future_field":"keep_me"}`), &details)
	require.NoError(t, err)

	b, err := json.Marshal(details)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))
	require.Equal(t, float64(1), decoded["cached_tokens"])
	require.NotNil(t, decoded["web_search"])
	require.Equal(t, "keep_me", decoded["future_field"])
}

// TestResponseAPITokenDetails_MarshalRequiredZeroValues verifies that present usage detail objects
// retain the zero-valued fields required by strict Responses API clients. It accepts no parameters
// beyond the test context and returns no value.
func TestResponseAPITokenDetails_MarshalRequiredZeroValues(t *testing.T) {
	inputData, err := json.Marshal(ResponseAPIInputTokensDetails{})
	require.NoError(t, err)
	require.JSONEq(t, `{"cached_tokens":0}`, string(inputData))

	outputData, err := json.Marshal(ResponseAPIOutputTokensDetails{})
	require.NoError(t, err)
	require.JSONEq(t, `{"reasoning_tokens":0}`, string(outputData))
}

func TestResponseAPIUsage_ToModelUsageCacheWriteTokens(t *testing.T) {
	var usage ResponseAPIUsage
	err := json.Unmarshal([]byte(`{
		"input_tokens":100,
		"output_tokens":20,
		"total_tokens":120,
		"cache_write_tokens":30,
		"input_tokens_details":{"cached_tokens":40}
	}`), &usage)
	require.NoError(t, err)

	converted := usage.ToModelUsage()
	require.NotNil(t, converted)
	require.Equal(t, 100, converted.PromptTokens)
	require.Equal(t, 20, converted.CompletionTokens)
	require.Equal(t, 120, converted.TotalTokens)
	require.Equal(t, 30, converted.CacheWrite5mTokens)
	require.Zero(t, converted.CacheWrite1hTokens)
	require.NotNil(t, converted.PromptTokensDetails)
	require.Equal(t, 40, converted.PromptTokensDetails.CachedTokens)
}

func TestUsageNormalizeCacheWriteTokens(t *testing.T) {
	usage := &model.Usage{
		CacheWriteTokens:   30,
		CacheWrite5mTokens: 2,
	}

	usage.NormalizeCacheWriteTokens()

	require.Zero(t, usage.CacheWriteTokens)
	require.Equal(t, 32, usage.CacheWrite5mTokens)
}
