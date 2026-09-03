package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestThinkingBindingControls_ConvertedRequestRoundTrip verifies converted Anthropic
// requests retain explicit block-binding controls and enable the required beta flag.
// It accepts the test context and reports all validation failures through that context.
func TestThinkingBindingControls_ConvertedRequestRoundTrip(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request := &model.GeneralOpenAIRequest{
		Model:     "claude-fable-5-1",
		MaxTokens: 1024,
		Messages:  []model.Message{{Role: "user", Content: "hello"}},
		Thinking: &model.Thinking{
			Type: "adaptive",
			BlockBinding: &model.ThinkingBlockBinding{
				PrefixMismatchBehavior: "drop_block",
			},
		},
	}

	out, err := (&Adaptor{}).ConvertRequest(c, 0, request)
	require.NoError(t, err)
	converted, ok := out.(*Request)
	require.True(t, ok, "expected *anthropic.Request, got %T", out)
	require.NotNil(t, converted.Thinking)
	require.NotNil(t, converted.Thinking.BlockBinding)
	require.Equal(t, "drop_block", converted.Thinking.BlockBinding.PrefixMismatchBehavior)

	raw, err := json.Marshal(converted)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"block_binding":{"prefix_mismatch_behavior":"drop_block"}`)
	require.True(t, c.GetBool(ctxkey.ClaudeThinkingBindingControlsEnabled))
}

// TestThinkingBindingControls_HeaderActivation verifies the Anthropic beta header
// is added only for explicit binding controls and is deduplicated case-insensitively.
// It accepts the test context and reports all validation failures through that context.
func TestThinkingBindingControls_HeaderActivation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		thinking    *model.Thinking
		inboundBeta string
		wantHeader  bool
	}{
		{name: "absent", wantHeader: false},
		{
			name: "error",
			thinking: &model.Thinking{
				Type:         "adaptive",
				BlockBinding: &model.ThinkingBlockBinding{PrefixMismatchBehavior: "error"},
			},
			wantHeader: true,
		},
		{
			name: "drop block deduplicates inbound beta",
			thinking: &model.Thinking{
				Type:         "adaptive",
				BlockBinding: &model.ThinkingBlockBinding{PrefixMismatchBehavior: "drop_block"},
			},
			inboundBeta: strings.ToUpper(AnthropicBetaThinkingBindingControls),
			wantHeader:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			if tt.inboundBeta != "" {
				c.Request.Header.Set("anthropic-beta", tt.inboundBeta)
			}

			request := &model.ClaudeRequest{
				Model:     "claude-fable-5-1",
				MaxTokens: 1024,
				Messages:  []model.ClaudeMessage{{Role: "user", Content: "hello"}},
				Thinking:  tt.thinking,
			}
			a := &Adaptor{}
			_, err := a.ConvertClaudeRequest(c, request)
			require.NoError(t, err)

			upstreamReq, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
			require.NoError(t, err)
			require.NoError(t, a.SetupRequestHeader(c, upstreamReq, &metalib.Meta{
				APIKey:          "test-key",
				ActualModelName: "claude-fable-5-1",
			}))

			tokens := strings.Split(upstreamReq.Header.Get("anthropic-beta"), ",")
			count := 0
			for _, token := range tokens {
				if token == AnthropicBetaThinkingBindingControls {
					count++
				}
			}
			if tt.wantHeader {
				require.Equal(t, 1, count)
			} else {
				require.Zero(t, count)
			}
		})
	}
}

// TestPreservedThinkingResponseMetadataRoundTrip verifies typed response handling
// retains empty thinking text, redacted data, and future transformation metadata.
// It accepts the test context and reports all validation failures through that context.
func TestPreservedThinkingResponseMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-fable-5-1",
		"content":[
			{"type":"thinking","thinking":"","signature":"sig=="},
			{"type":"redacted_thinking","data":"opaque=="}
		],
		"stop_reason":"end_turn",
		"stop_sequence":null,
		"usage":{"input_tokens":10,"output_tokens":2},
		"input_transformations":[{
			"type":"thinking_dropped",
			"path":"messages.1.content.0",
			"reason":"prefix_binding_mismatch",
			"future_metadata":{"kept":true}
		}]
	}`)

	var response Response
	require.NoError(t, json.Unmarshal(raw, &response))
	require.Len(t, response.Content, 2)
	require.NotNil(t, response.Content[0].Thinking)
	require.Equal(t, "", *response.Content[0].Thinking)
	require.NotNil(t, response.Content[1].Data)
	require.Equal(t, "opaque==", *response.Content[1].Data)
	require.NotNil(t, response.InputTransformations)
	require.Len(t, *response.InputTransformations, 1)
	require.Equal(t, "thinking_dropped", (*response.InputTransformations)[0]["type"])
	require.Contains(t, (*response.InputTransformations)[0], "future_metadata")

	reencoded, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(reencoded), `"thinking":""`)
	require.Contains(t, string(reencoded), `"data":"opaque=="`)
	require.Contains(t, string(reencoded), `"future_metadata":{"kept":true}`)

	var empty Response
	require.NoError(t, json.Unmarshal([]byte(`{"input_transformations":[]}`), &empty))
	require.NotNil(t, empty.InputTransformations)
	reencoded, err = json.Marshal(empty)
	require.NoError(t, err)
	require.Contains(t, string(reencoded), `"input_transformations":[]`)
}

// TestNativeStream_PreservesThinkingInputTransformations verifies native streaming
// forwards preserved-thinking transformation metadata from the message_start event.
// It accepts the test context and reports all validation failures through that context.
func TestNativeStream_PreservesThinkingInputTransformations(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-fable-5-1","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0},"input_transformations":[{"type":"thinking_dropped","path":"messages.1.content.0","reason":"prefix_binding_mismatch","future_metadata":{"kept":true}}]}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":2}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	body := recorder.Body.String()
	require.Contains(t, body, `"input_transformations":[{"type":"thinking_dropped","path":"messages.1.content.0","reason":"prefix_binding_mismatch","future_metadata":{"kept":true}}]`)
}
