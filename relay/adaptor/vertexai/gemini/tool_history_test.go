package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// vertexContinuationRequest returns parallel calls with deliberately reversed result messages.
func vertexContinuationRequest() *model.GeneralOpenAIRequest {
	return &model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{
		{Role: "user", Content: "check both cities"},
		{Role: "assistant", ToolCalls: []model.Tool{
			{Id: "call_a", Type: "function", Function: &model.Function{Name: "weather", Arguments: `{"city":"A"}`}},
			{Id: "call_b", Type: "function", Function: &model.Function{Name: "weather", Arguments: `{"city":"B"}`}},
		}},
		{Role: "tool", ToolCallId: "call_b", Content: `{"temperature":20}`},
		{Role: "tool", ToolCallId: "call_a", Content: "sunny"},
	}}
}

// TestVertexContinuationNativeResponses checks results are correlated by ID and emitted in call order.
func TestVertexContinuationNativeResponses(t *testing.T) {
	request := vertexContinuationRequest()
	before, err := json.Marshal(request)
	require.NoError(t, err)
	var wire struct {
		Contents []struct {
			Role  string                       `json:"role"`
			Parts []map[string]json.RawMessage `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
	require.Len(t, wire.Contents, 3)
	require.Equal(t, "model", wire.Contents[1].Role)
	require.Equal(t, "user", wire.Contents[2].Role)
	require.Len(t, wire.Contents[2].Parts, 2)
	require.JSONEq(t, `{"name":"weather","response":{"output":"sunny"}}`, string(wire.Contents[2].Parts[0]["functionResponse"]))
	require.JSONEq(t, `{"name":"weather","response":{"temperature":20}}`, string(wire.Contents[2].Parts[1]["functionResponse"]))
	for _, part := range wire.Contents[2].Parts {
		require.NotContains(t, part, "text")
	}
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestVertexContinuationRejectsInvalidHistory checks ambiguous or incomplete histories fail before dispatch.
func TestVertexContinuationRejectsInvalidHistory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.GeneralOpenAIRequest)
	}{
		{"unknown_result", func(r *model.GeneralOpenAIRequest) { r.Messages[2].ToolCallId = "unknown" }},
		{"missing_result", func(r *model.GeneralOpenAIRequest) { r.Messages = r.Messages[:3] }},
		{"duplicate_result", func(r *model.GeneralOpenAIRequest) { r.Messages[3].ToolCallId = "call_b" }},
		{"duplicate_call", func(r *model.GeneralOpenAIRequest) { r.Messages[1].ToolCalls[1].Id = "call_a" }},
		{"empty_call_id", func(r *model.GeneralOpenAIRequest) { r.Messages[1].ToolCalls[0].Id = "" }},
		{"missing_function", func(r *model.GeneralOpenAIRequest) { r.Messages[1].ToolCalls[0].Function = nil }},
		{"invalid_arguments", func(r *model.GeneralOpenAIRequest) { r.Messages[1].ToolCalls[0].Function.Arguments = "{" }},
		{"scalar_arguments", func(r *model.GeneralOpenAIRequest) { r.Messages[1].ToolCalls[0].Function.Arguments = "[]" }},
		{"orphan_result", func(r *model.GeneralOpenAIRequest) { r.Messages = r.Messages[2:] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := vertexContinuationRequest()
			tc.change(request)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			require.ErrorContains(t, err, "validation failed")
			require.Nil(t, converted)
		})
	}
}

// TestVertexContinuationOutputShapes preserves JSON objects and wraps scalar, array, null and text results.
func TestVertexContinuationOutputShapes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content any
		want    string
	}{
		{"object", `{"error":{"message":"explicit error"}}`, `{"error":{"message":"explicit error"}}`},
		{"scalar", "42", `{"output":42}`}, {"array", "[1,2]", `{"output":[1,2]}`},
		{"null", "null", `{"output":null}`}, {"text", "hello", `{"output":"hello"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := vertexContinuationRequest()
			request.Messages[1].ToolCalls = request.Messages[1].ToolCalls[:1]
			request.Messages = request.Messages[:3]
			request.Messages[2].ToolCallId = "call_a"
			request.Messages[2].Content = tc.content
			var wire struct {
				Contents []struct {
					Parts []struct {
						Response *struct {
							Value json.RawMessage `json:"response"`
						} `json:"functionResponse"`
					} `json:"parts"`
				} `json:"contents"`
			}
			require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
			require.NotNil(t, wire.Contents[2].Parts[0].Response)
			require.JSONEq(t, tc.want, string(wire.Contents[2].Parts[0].Response.Value))
		})
	}
}
