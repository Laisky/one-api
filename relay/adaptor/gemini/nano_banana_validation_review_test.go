package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNanoBananaHistoricalToolAdmission exercises conversion rather than only
// testing the validator: unsupported history must never reach generateContent.
func TestNanoBananaHistoricalToolAdmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request model.GeneralOpenAIRequest
		wantErr string
	}{
		{
			name: "assistant_tool_calls",
			request: model.GeneralOpenAIRequest{Messages: []model.Message{
				{Role: "assistant", ToolCalls: []model.Tool{{Type: "function", Function: &model.Function{Name: "lookup", Arguments: "{}"}}}},
			}},
			wantErr: "function calling",
		},
		{
			name: "tool_response_history",
			request: model.GeneralOpenAIRequest{Messages: []model.Message{
				{Role: "tool", ToolCallId: "call_123", Content: "tool result"},
			}},
			wantErr: "function calling",
		},
		{
			name: "legacy_function_call",
			request: model.GeneralOpenAIRequest{
				FunctionCall: map[string]any{"name": "lookup"},
				Messages:     []model.Message{{Role: "user", Content: "draw"}},
			},
			wantErr: "function calling",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
			tc.request.Model = "channel-alias"
			got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &tc.request)
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, got)
		})
	}

	// Existing numeric models still accept historical tool conversations.
	legacy := model.GeneralOpenAIRequest{Model: "gemini-2.5-flash", Messages: []model.Message{
		{Role: "assistant", ToolCalls: []model.Tool{{Type: "function", Function: &model.Function{Name: "lookup", Arguments: "{}"}}}},
	}}
	got, err := (&Adaptor{}).ConvertRequest(nil, relaymode.ChatCompletions, &legacy)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestNanoBananaMalformedAudioAdmissionRequires4xx verifies rejected
// unsupported audio cannot crash parsing before the provider call.
func TestNanoBananaMalformedAudioAdmissionRequires4xx(t *testing.T) {
	for _, malformed := range []string{
		"{\"type\":\"input_audio\",\"input_audio\":{\"format\":\"wav\"}}",
		"{\"type\":\"input_audio\",\"input_audio\":{\"data\":\"YQ==\",\"format\":42}}",
	} {
		var req model.GeneralOpenAIRequest
		payload := "{\"model\":\"gemini-nano-banana-2.1\",\"messages\":[{\"role\":\"user\",\"content\":[" + malformed + "]}]}"
		require.NoError(t, json.Unmarshal([]byte(payload), &req))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		var got any
		var err error
		require.NotPanics(t, func() {
			got, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &req)
		})
		require.ErrorContains(t, err, "input content type")
		require.Nil(t, got)
	}
}
