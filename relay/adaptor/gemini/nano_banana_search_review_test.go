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

// TestGeminiOctober28RejectDroppedSearch reproduces the silent grounding loss:
// unsupported web_search requests must fail before a native request is forwarded.
func TestGeminiOctober28RejectDroppedSearch(t *testing.T) {
	for _, tt := range []struct {
		name    string
		request model.GeneralOpenAIRequest
	}{
		{"explicit_tool", model.GeneralOpenAIRequest{
			Model:    "gemini-nano-banana-2.1",
			Tools:    []model.Tool{{Type: "web_search"}},
			Messages: []model.Message{{Role: "user", Content: "What happened today?"}},
		}},
		{"mixed_case_model", model.GeneralOpenAIRequest{
			Model:    "Gemini-Nano-Banana-2.1",
			Tools:    []model.Tool{{Type: "web_search"}},
			Messages: []model.Message{{Role: "user", Content: "What happened today?"}},
		}},
		{"web_search_options", model.GeneralOpenAIRequest{
			Model:            "gemini-nano-banana-2.1",
			WebSearchOptions: &model.WebSearchOptions{},
			Messages:         []model.Message{{Role: "user", Content: "What happened today?"}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			got, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, &tt.request)
			require.Error(t, err, "grounding requests must not be accepted then discarded")
			require.Nil(t, got)
			require.Contains(t, err.Error(), "web_search")
		})
	}

	ordinary := model.GeneralOpenAIRequest{
		Model:    "gemini-nano-banana-2.1",
		Messages: []model.Message{{Role: "user", Content: "Draw a duck."}},
	}
	got, err := (&Adaptor{}).ConvertRequest(nil, relaymode.ChatCompletions, &ordinary)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestGeminiOctober28MappedNanoBananaRejectsDroppedTools verifies authorization
// follows the effective upstream model even when the client uses a channel alias.
func TestGeminiOctober28MappedNanoBananaRejectsDroppedTools(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	// A mixed-case effective model ID must not bypass tool admission even
	// when the user-facing model is a custom channel alias.
	ctx.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "Gemini-Nano-Banana-2.1"})
	req := &model.GeneralOpenAIRequest{
		Model:    "custom-channel-image-alias",
		Tools:    []model.Tool{{Type: "web_search_preview"}},
		Messages: []model.Message{{Role: "user", Content: "Search for flowers."}},
	}
	got, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, req)
	require.ErrorContains(t, err, "web_search")
	require.Nil(t, got)

	req.Tools = nil
	req.Functions = []model.Function{{Name: "unsupported_function"}}
	got, err = (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, req)
	require.ErrorContains(t, err, "function calling")
	require.Nil(t, got)

	// Other models keep their previous converter behavior; this admission fix
	// is intentionally scoped to Nano Banana to avoid surprising old channels.
	req.Model = "gemini-3.8-flash"
	ctx.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-3.8-flash"})
	got, err = (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestGeminiRejectUnsupportedCapabilities verifies unsupported native settings are not silently ignored or forwarded.
func TestGeminiRejectUnsupportedCapabilities(t *testing.T) {
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

// TestGeminiMappedNanoBananaPayload verifies conversion uses the effective model for aliases.
func TestGeminiMappedNanoBananaPayload(t *testing.T) {
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

// TestGeminiNanoBananaRejectsVideo verifies unsupported native video is rejected before dispatch.
func TestGeminiNanoBananaRejectsVideo(t *testing.T) {
	var req model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-nano-banana-2.1","messages":[{"role":"user","content":[{"type":"video","video_url":"https://example.test/video.mp4"}]}]}`), &req))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &req)
	require.Error(t, err)
	require.Nil(t, got)
}
