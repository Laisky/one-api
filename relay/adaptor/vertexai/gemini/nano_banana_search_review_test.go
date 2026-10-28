package vertexai

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

// TestVertexGeminiOctober28RejectDroppedSearch verifies Vertex inherits the
// native Gemini grounding guard and preserves regular generation requests.
func TestVertexGeminiOctober28RejectDroppedSearch(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	bad := &model.GeneralOpenAIRequest{
		Model:    "gemini-nano-banana-2.1",
		Tools:    []model.Tool{{Type: "web_search"}},
		Messages: []model.Message{{Role: "user", Content: "Search and summarize."}},
	}
	converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, bad)
	require.Error(t, err)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), "web_search")

	good := &model.GeneralOpenAIRequest{
		Model:    "gemini-nano-banana-2.1",
		Messages: []model.Message{{Role: "user", Content: "Draw a duck."}},
	}
	converted, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, good)
	require.NoError(t, err)
	require.NotNil(t, converted)
}

// TestVertexGeminiRejectUnsupportedCapabilities verifies unsupported native settings are not silently ignored or forwarded.
func TestVertexGeminiRejectUnsupportedCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tools  []model.Tool
		format *model.ResponseFormat
	}{
		{name: "unknown_tool", tools: []model.Tool{{Type: "provider_grounding"}}},
		{name: "json_object", format: &model.ResponseFormat{Type: "json_object"}},
		{name: "json_schema", format: &model.ResponseFormat{Type: "json_schema", JsonSchema: &model.JSONSchema{Name: "result", Schema: map[string]any{"type": "object"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := &model.GeneralOpenAIRequest{Model: "gemini-nano-banana-2.1", Tools: tc.tools, ResponseFormat: tc.format, Messages: []model.Message{{Role: "user", Content: "hello"}}}
			got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
	for _, name := range []string{"gemini-nano-banana-2.1", "gemini-2.5-flash"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := &model.GeneralOpenAIRequest{Model: name, ResponseFormat: &model.ResponseFormat{Type: "text"}, Messages: []model.Message{{Role: "user", Content: "hello"}}}
		got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
		require.NoError(t, err)
		require.NotNil(t, got)
	}
}

// TestVertexGeminiMappedNanoBananaPayload verifies conversion uses the effective model for aliases.
func TestVertexGeminiMappedNanoBananaPayload(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
	effort := "high"
	req := &model.GeneralOpenAIRequest{Model: "alias", ReasoningEffort: &effort, Messages: []model.Message{{Role: "system", Content: "instruction"}, {Role: "user", Content: "draw"}}}
	got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"thinkingLevel":"high"`)
	require.Contains(t, string(raw), `"system_instruction"`)
	require.Contains(t, string(raw), `"responseModalities":["TEXT","IMAGE"]`)
}

// TestVertexGeminiNanoBananaRejectsVideo verifies unsupported native video is rejected before dispatch.
func TestVertexGeminiNanoBananaRejectsVideo(t *testing.T) {
	var req model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-nano-banana-2.1","messages":[{"role":"user","content":[{"type":"video","video_url":"https://example.test/video.mp4"}]}]}`), &req))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &req)
	require.Error(t, err)
	require.Nil(t, got)
}
