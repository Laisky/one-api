package gemini

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
