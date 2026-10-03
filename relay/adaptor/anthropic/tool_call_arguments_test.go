package anthropic

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertRequestSurvivesMalformedToolCalls pins that a client cannot crash the
// converter through the `tool_calls` field.
//
// model.Function.Arguments is `any` and model.Tool.Function is a pointer, both
// filled straight from the request body. ConvertRequest used to do
// ToolCalls[i].Function.Arguments.(string) with no comma-ok, so three shapes a
// client can send panicked: an object-valued `arguments`, an absent `arguments`,
// and a tool call with no `function` at all.
func TestConvertRequestSurvivesMalformedToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name      string
		toolCall  model.Tool
		wantName  string
		wantInput map[string]any
	}{
		{
			name:      "arguments sent as an object",
			toolCall:  model.Tool{Id: "t1", Type: "function", Function: &model.Function{Name: "lookup", Arguments: map[string]any{"q": "x"}}},
			wantName:  "lookup",
			wantInput: map[string]any{"q": "x"},
		},
		{
			name:      "arguments omitted",
			toolCall:  model.Tool{Id: "t2", Type: "function", Function: &model.Function{Name: "lookup"}},
			wantName:  "lookup",
			wantInput: map[string]any{},
		},
		{
			name:      "function omitted entirely",
			toolCall:  model.Tool{Id: "t3", Type: "function"},
			wantName:  "",
			wantInput: map[string]any{},
		},
		{
			name:      "arguments sent as a JSON string",
			toolCall:  model.Tool{Id: "t4", Type: "function", Function: &model.Function{Name: "lookup", Arguments: `{"q":"y"}`}},
			wantName:  "lookup",
			wantInput: map[string]any{"q": "y"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

			var converted *Request
			var err error
			require.NotPanics(t, func() {
				converted, err = ConvertRequest(c, model.GeneralOpenAIRequest{
					Model: "claude-3-5-sonnet",
					Messages: []model.Message{
						{Role: "assistant", Content: "calling a tool", ToolCalls: []model.Tool{tc.toolCall}},
					},
				})
			})
			require.NoError(t, err)
			require.NotNil(t, converted)

			var toolUse *Content
			for i := range converted.Messages {
				for j := range converted.Messages[i].Content {
					if converted.Messages[i].Content[j].Type == "tool_use" {
						toolUse = &converted.Messages[i].Content[j]
					}
				}
			}
			require.NotNil(t, toolUse, "converted request must carry the tool_use block")
			require.Equal(t, tc.wantName, toolUse.Name)
			require.Equal(t, tc.wantInput, toolUse.Input)
		})
	}
}
