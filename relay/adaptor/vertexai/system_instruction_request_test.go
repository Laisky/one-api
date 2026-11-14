package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	nativegemini "github.com/Laisky/one-api/relay/adaptor/gemini"
	openaiapi "github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexMappedSystemInstructionInputOwnership keeps caller-owned Chat, Claude, and Responses requests immutable.
func TestVertexMappedSystemInstructionInputOwnership(t *testing.T) {
	for _, kind := range []string{"chat", "claude", "responses"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-4-hydrogen"})
			var source any
			switch kind {
			case "chat":
				source = &model.GeneralOpenAIRequest{Model: "alias", Messages: []model.Message{{Role: "system", Content: "Instruction"}, {Role: "user", Content: "Question"}}}
			case "claude":
				source = &model.ClaudeRequest{Model: "alias", MaxTokens: 128, System: "Instruction", Messages: []model.ClaudeMessage{{Role: "user", Content: "Question"}}}
			case "responses":
				var request openaiapi.ResponseAPIRequest
				require.NoError(t, json.Unmarshal([]byte(`{"model":"alias","instructions":"Instruction","input":"Question"}`), &request))
				source = &request
			}
			before, err := json.Marshal(source)
			require.NoError(t, err)
			var result any
			switch request := source.(type) {
			case *model.GeneralOpenAIRequest:
				result, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			case *model.ClaudeRequest:
				result, err = (&Adaptor{}).ConvertClaudeRequest(c, request)
			case *openaiapi.ResponseAPIRequest:
				var chat *model.GeneralOpenAIRequest
				chat, err = openaiapi.ConvertResponseAPIToChatCompletionRequestWithOptions(request, nil, openaiapi.ResponseFallbackConversionOptions{NativeMedia: true})
				require.NoError(t, err)
				result, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, chat)
			}
			require.NoError(t, err)
			converted := result.(*nativegemini.ChatRequest)
			require.NotNil(t, converted.SystemInstruction)
			require.Equal(t, "Instruction", converted.SystemInstruction.Parts[0].Text)
			require.Len(t, converted.Contents, 1)
			after, err := json.Marshal(source)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}
