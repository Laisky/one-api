package vertexai

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexNanoBananaRejectsHistoricalFunctionHistory confirms the shared
// guard applies to mapped Vertex Gemini channels, not just the Gemini API.
func TestVertexNanoBananaRejectsHistoricalFunctionHistory(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
	req := &model.GeneralOpenAIRequest{Model: "vertex-alias", Messages: []model.Message{
		{Role: "assistant", ToolCalls: []model.Tool{{Type: "function", Function: &model.Function{Name: "lookup"}}}},
	}}
	got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.ErrorContains(t, err, "function calling")
	require.Nil(t, got)
}

// TestVertexNanoBananaNoneToolChoicePreservesNoFunctionSemantics confirms
// aliases use the same normalized transport contract as direct Gemini requests.
func TestVertexNanoBananaNoneToolChoicePreservesNoFunctionSemantics(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
	req := &model.GeneralOpenAIRequest{Model: "vertex-alias", ToolChoice: "none",
		Messages: []model.Message{{Role: "user", Content: "draw"}}}
	result, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.NotNil(t, result)
}
