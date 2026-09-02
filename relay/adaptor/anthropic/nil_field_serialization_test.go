package anthropic

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertClaudeRequestNeverSendsEmptyRequiredFields pins two required
// Anthropic fields that cannot carry omitempty and so must never be left at
// their zero value.
//
//   - Tool.InputSchema is a struct with a required `type`; a Claude tool that
//     omits input_schema, or sends a non-object, produced
//     `"input_schema":{"type":""}`. The OpenAI->Anthropic converter in the same
//     file already defaulted this to "object"; this path did not.
//   - Message.Content is a slice with no omitempty; the non-string/non-array arm
//     leaves it nil on a literal `null` or an object, producing `"content": null`.
//     Every other adaptor guards this with len(); Anthropic was the outlier.
func TestConvertClaudeRequestNeverSendsEmptyRequiredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name    string
		request model.ClaudeRequest
	}{
		{
			name: "tool without input_schema",
			request: model.ClaudeRequest{
				Model:     "claude-opus-4-6",
				MaxTokens: 100,
				Messages:  []model.ClaudeMessage{{Role: "user", Content: "hi"}},
				Tools:     []model.ClaudeTool{{Name: "lookup", Description: "d"}},
			},
		},
		{
			name: "tool whose input_schema is not an object",
			request: model.ClaudeRequest{
				Model:     "claude-opus-4-6",
				MaxTokens: 100,
				Messages:  []model.ClaudeMessage{{Role: "user", Content: "hi"}},
				Tools:     []model.ClaudeTool{{Name: "lookup", InputSchema: "not-an-object"}},
			},
		},
		{
			// A literal JSON null decodes to a nil `any`, reaches the default arm,
			// and leaves contentBlocks nil without an error.
			name: "message content sent as null",
			request: model.ClaudeRequest{
				Model:     "claude-opus-4-6",
				MaxTokens: 100,
				Messages:  []model.ClaudeMessage{{Role: "user", Content: nil}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

			converted, err := ConvertClaudeRequest(c, tc.request)
			require.NoError(t, err)

			body, err := json.Marshal(converted)
			require.NoError(t, err)

			require.NotContains(t, string(body), `"content":null`,
				"Anthropic rejects a null content array: %s", body)
			require.NotContains(t, string(body), `"type":""`,
				"input_schema.type must default to object: %s", body)
		})
	}
}
