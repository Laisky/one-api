package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

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
			Model: "gemini-nano-banana-2.1",
			Tools: []model.Tool{{Type: "web_search"}},
			Messages: []model.Message{{Role: "user", Content: "What happened today?"}},
		}},
		{"web_search_options", model.GeneralOpenAIRequest{
			Model: "gemini-nano-banana-2.1",
			WebSearchOptions: &model.WebSearchOptions{},
			Messages: []model.Message{{Role: "user", Content: "What happened today?"}},
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
		Model: "gemini-nano-banana-2.1",
		Messages: []model.Message{{Role: "user", Content: "Draw a duck."}},
	}
	got, err := (&Adaptor{}).ConvertRequest(nil, relaymode.ChatCompletions, &ordinary)
	require.NoError(t, err)
	require.NotNil(t, got)
}
