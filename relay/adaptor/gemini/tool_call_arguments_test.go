package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertRequestSurvivesMalformedToolCalls mirrors the Anthropic guard: a
// client-supplied tool call whose `arguments` is an object, is absent, or whose
// `function` is missing entirely must not panic the Gemini converter. The call
// site used to type-assert Arguments.(string) with no comma-ok.
func TestConvertRequestSurvivesMalformedToolCalls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		toolCall model.Tool
		wantArgs map[string]any
	}{
		{
			name:     "arguments sent as an object",
			toolCall: model.Tool{Id: "t1", Type: "function", Function: &model.Function{Name: "lookup", Arguments: map[string]any{"q": "x"}}},
			wantArgs: map[string]any{"q": "x"},
		},
		{
			name:     "arguments omitted",
			toolCall: model.Tool{Id: "t2", Type: "function", Function: &model.Function{Name: "lookup"}},
			wantArgs: map[string]any{},
		},
		{
			name:     "function omitted entirely",
			toolCall: model.Tool{Id: "t3", Type: "function"},
			wantArgs: map[string]any{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var converted *ChatRequest
			require.NotPanics(t, func() {
				converted = ConvertRequest(model.GeneralOpenAIRequest{
					Model: "gemini-2.5-flash",
					Messages: []model.Message{
						{Role: "assistant", Content: "calling a tool", ToolCalls: []model.Tool{tc.toolCall}},
					},
				})
			})
			require.NotNil(t, converted)

			var found bool
			for _, content := range converted.Contents {
				for _, part := range content.Parts {
					if part.FunctionCall != nil {
						found = true
						require.Equal(t, tc.wantArgs, part.FunctionCall.Arguments)
					}
				}
			}
			require.True(t, found, "converted request must carry the function call")
		})
	}
}

// TestConvertRequestOmitsEmptyToolDeclarations pins that an explicitly empty
// tools/functions list produces no `tools` block at all.
//
// ChatTools.FunctionDeclarations is an `any` field, so `omitempty` does not drop
// an empty slice: the guard used to be `textRequest.Tools != nil`, which is true
// for `"tools": []` and emitted `"tools":[{"function_declarations":[]}]`. Gemini
// rejects an empty function_declarations array with 400 INVALID_ARGUMENT.
func TestConvertRequestOmitsEmptyToolDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request model.GeneralOpenAIRequest
	}{
		{
			name:    "explicitly empty tools",
			request: model.GeneralOpenAIRequest{Model: "gemini-2.5-flash", Tools: []model.Tool{}},
		},
		{
			name:    "explicitly empty functions",
			request: model.GeneralOpenAIRequest{Model: "gemini-2.5-flash", Functions: []model.Function{}},
		},
		{
			name:    "tool entry without a function object",
			request: model.GeneralOpenAIRequest{Model: "gemini-2.5-flash", Tools: []model.Tool{{Id: "t1", Type: "function"}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.request.Messages = []model.Message{{Role: "user", Content: "hi"}}

			var converted *ChatRequest
			require.NotPanics(t, func() { converted = ConvertRequest(tc.request) })
			require.NotNil(t, converted)

			body, err := json.Marshal(converted)
			require.NoError(t, err)
			require.NotContains(t, string(body), `"function_declarations":[]`,
				"an empty declaration list must be omitted, not sent: %s", body)
			require.Empty(t, converted.Tools)
		})
	}
}
