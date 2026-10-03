package openai_compatible

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestConvertClaudeRequest_DoesNotLeakAnthropicThinkingBinding verifies model
// switching to an OpenAI-compatible upstream translates visible reasoning context
// without forwarding Anthropic-only binding controls or signatures. It accepts the
// test context and reports all validation failures through that context.
func TestConvertClaudeRequest_DoesNotLeakAnthropicThinkingBinding(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.ClaudeRequest{
		Model:     "gpt-compatible-model",
		MaxTokens: 4096,
		Thinking: &relaymodel.Thinking{
			Type: "adaptive",
			BlockBinding: &relaymodel.ThinkingBlockBinding{
				PrefixMismatchBehavior: "drop_block",
			},
		},
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: []any{
				map[string]any{
					"type":      "thinking",
					"thinking":  "portable reasoning context",
					"signature": "anthropic-signature",
				},
				map[string]any{"type": "text", "text": "first answer"},
			}},
			{Role: "user", Content: "follow up"},
		},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	convertedAny, err := ConvertClaudeRequest(context, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)

	// The generic conversion does not forward an Anthropic top-level thinking object.
	require.Nil(t, converted.Thinking)

	var assistant *relaymodel.Message
	for i := range converted.Messages {
		if converted.Messages[i].Role == "assistant" {
			assistant = &converted.Messages[i]
			break
		}
	}
	require.NotNil(t, assistant)
	require.NotNil(t, assistant.Thinking)
	require.Equal(t, "portable reasoning context", *assistant.Thinking)

	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"block_binding"`)
	require.NotContains(t, string(encoded), `"anthropic-signature"`)

	// Conversion must not mutate the caller's reusable request object.
	require.NotNil(t, request.Thinking.BlockBinding)
}
