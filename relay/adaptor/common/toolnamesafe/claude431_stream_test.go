package toolnamesafe_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestClaude431ReviewToolStreamCompletion checks tool-name restoration independently of stream finality.
// Complete streams finish once; missing block, receipt, or message terminators never become success.
func TestClaude431ReviewToolStreamCompletion(t *testing.T) {
	t.Parallel()
	for _, remove := range []string{"", "content_block_stop", "message_stop", "message_delta"} {
		name := remove
		if name == "" {
			name = "complete"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, w := newRoundTripCtx(t, channeltype.Anthropic, "https://provider.invalid", "claude-sonnet-5-5")
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			request := &model.GeneralOpenAIRequest{
				Model: "claude-sonnet-5-5", MaxTokens: 256, Stream: true,
				Messages: []model.Message{{Role: "user", Content: "use the tool"}},
				Tools:    []model.Tool{{Type: "function", Function: &model.Function{Name: originalToolName, Parameters: map[string]any{"type": "object"}}}},
			}
			_, err := (&anthropic.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			require.NoError(t, err)
			response := buildAnthropicStreamResponse(t, sanitizedToolName)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			var events []string
			for _, event := range strings.Split(strings.TrimSpace(string(body)), "\n\n") {
				if remove != "" && strings.Contains(event, `"type":"`+remove+`"`) {
					continue
				}
				events = append(events, event)
			}
			response.Body = io.NopCloser(strings.NewReader(strings.Join(events, "\n\n") + "\n\n"))
			failure, usage := anthropic.StreamHandler(c, response)
			require.NotNil(t, usage)
			require.Equal(t, 3, usage.PromptTokens)
			out := w.Body.String()
			require.Contains(t, out, `"name":"`+originalToolName+`"`)
			require.NotContains(t, out, `"name":"`+sanitizedToolName+`"`)
			require.NotContains(t, out, `"finish_reason":""`)
			if remove == "" {
				require.Nil(t, failure)
				require.Equal(t, 7, usage.CompletionTokens)
				require.Equal(t, 10, usage.TotalTokens)
				require.Equal(t, 1, strings.Count(out, `"finish_reason":"tool_calls"`))
				require.Equal(t, 1, strings.Count(out, "data: [DONE]"))
			} else {
				require.NotNil(t, failure)
				require.NotContains(t, out, `"finish_reason":"tool_calls"`)
				require.NotContains(t, out, "data: [DONE]")
			}
		})
	}
}
