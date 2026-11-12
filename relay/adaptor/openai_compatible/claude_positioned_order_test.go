package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// claudePositionedConversion exercises the actual buffered bridge and decodes its wire response.
func claudePositionedConversion(t *testing.T, message string) model.ClaudeResponse {
	t.Helper()
	c, _ := newGinTestContext()
	body := `{"model":"gemini-3.8-flash","choices":[{"message":` + message + `,"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`
	converted, failure := ConvertOpenAIResponseToClaudeResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, failure)
	raw, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	require.NoError(t, converted.Body.Close())
	var response model.ClaudeResponse
	require.NoError(t, json.Unmarshal(raw, &response))
	require.Equal(t, "tool_use", response.StopReason)
	require.Equal(t, 5, response.Usage.InputTokens)
	require.Equal(t, 2, response.Usage.OutputTokens)
	return response
}

// TestClaudePositionedCallBeforeText preserves the native call/text order rather than regrouping by kind.
func TestClaudePositionedCallBeforeText(t *testing.T) {
	response := claudePositionedConversion(t, `{"role":"assistant","content":[{"type":"text","text":"after","extra_content":{"google":{"thought_signature":"text-signature"},"oneapi":{"gemini_part_index":1}}}],"tool_calls":[{"id":"call_one","type":"function","function":{"name":"lookup","arguments":"{\"x\":1}"},"extra_content":{"google":{"thought_signature":"call-signature"},"oneapi":{"gemini_part_index":0}}}]}`)
	require.Len(t, response.Content, 2)
	require.Equal(t, "tool_use", response.Content[0].Type)
	require.Equal(t, "lookup", response.Content[0].Name)
	require.JSONEq(t, `{"x":1}`, string(response.Content[0].Input))
	require.Equal(t, "call-signature", model.ToolThoughtSignature(model.Tool{ExtraContent: response.Content[0].ExtraContent}))
	require.Equal(t, "text", response.Content[1].Type)
	require.Equal(t, "after", response.Content[1].Text)
	require.Equal(t, "text-signature", model.ToolThoughtSignature(model.Tool{ExtraContent: response.Content[1].ExtraContent}))
}

// TestClaudePositionedMixedOrder retains empty signed boundaries between independently signed calls.
func TestClaudePositionedMixedOrder(t *testing.T) {
	response := claudePositionedConversion(t, `{"role":"assistant","content":[{"type":"text","text":"before","extra_content":{"oneapi":{"gemini_part_index":0}}},{"type":"text","text":"","extra_content":{"google":{"thought_signature":"empty-signature"},"oneapi":{"gemini_part_index":2}}},{"type":"text","text":"after","extra_content":{"oneapi":{"gemini_part_index":4}}}],"tool_calls":[{"id":"call_one","type":"function","function":{"name":"one","arguments":"{}"},"extra_content":{"google":{"thought_signature":"one-signature"},"oneapi":{"gemini_part_index":1}}},{"id":"call_two","type":"function","function":{"name":"two","arguments":"{}"},"extra_content":{"google":{"thought_signature":"two-signature"},"oneapi":{"gemini_part_index":3}}}]}`)
	require.Len(t, response.Content, 5)
	var kinds []string
	for _, block := range response.Content {
		kinds = append(kinds, block.Type)
	}
	require.Equal(t, []string{"text", "tool_use", "text", "tool_use", "text"}, kinds)
	require.Equal(t, "before", response.Content[0].Text)
	require.Equal(t, "after", response.Content[4].Text)
	require.Equal(t, "one", response.Content[1].Name)
	require.Equal(t, "two", response.Content[3].Name)
	require.NotEmpty(t, response.Content[1].ID)
	require.NotEqual(t, response.Content[1].ID, response.Content[3].ID)
	require.Equal(t, "", response.Content[2].Text)
	require.Equal(t, "empty-signature", model.ToolThoughtSignature(model.Tool{ExtraContent: response.Content[2].ExtraContent}))
	for i, block := range response.Content {
		position, present := model.GeminiPartIndex(block.ExtraContent)
		require.True(t, present)
		require.Equal(t, i, position)
	}
}

// TestClaudePositionedUnsignedAndInvalidControls retains the established projection unless a complete permutation exists.
func TestClaudePositionedUnsignedAndInvalidControls(t *testing.T) {
	for _, tc := range []struct{ name, contentMeta, toolMeta string }{
		{"unsigned", "", ""},
		{"signature_without_positions", `,"extra_content":{"google":{"thought_signature":"text"}}`, `,"extra_content":{"google":{"thought_signature":"call"}}`},
		{"duplicate", `,"extra_content":{"oneapi":{"gemini_part_index":0}}`, `,"extra_content":{"oneapi":{"gemini_part_index":0}}`},
		{"missing", `,"extra_content":{"oneapi":{"gemini_part_index":1}}`, ""},
		{"negative", `,"extra_content":{"oneapi":{"gemini_part_index":-1}}`, `,"extra_content":{"oneapi":{"gemini_part_index":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := `{"role":"assistant","content":[{"type":"text","text":"visible"` + tc.contentMeta + `}],"tool_calls":[{"id":"call_one","type":"function","function":{"name":"lookup","arguments":"{}"}` + tc.toolMeta + `}]}`
			// The ordinary unsigned response uses the existing scalar content contract.
			if tc.name == "unsigned" {
				message = `{"role":"assistant","content":"visible","tool_calls":[{"id":"call_one","type":"function","function":{"name":"lookup","arguments":"{}"}}]}`
			}
			response := claudePositionedConversion(t, message)
			require.Len(t, response.Content, 2)
			require.Equal(t, "text", response.Content[0].Type)
			require.Equal(t, "visible", response.Content[0].Text)
			require.Equal(t, "tool_use", response.Content[1].Type)
			require.Equal(t, "lookup", response.Content[1].Name)
			if tc.name == "unsigned" {
				require.Nil(t, response.Content[0].ExtraContent)
				require.Nil(t, response.Content[1].ExtraContent)
			}
		})
	}
}
