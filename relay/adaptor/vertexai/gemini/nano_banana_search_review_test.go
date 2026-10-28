package vertexai

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexGeminiOctober28RejectDroppedSearch verifies Vertex inherits the
// native Gemini grounding guard and preserves regular generation requests.
func TestVertexGeminiOctober28RejectDroppedSearch(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	bad := &model.GeneralOpenAIRequest{
		Model: "gemini-nano-banana-2.1",
		Tools: []model.Tool{{Type: "web_search"}},
		Messages: []model.Message{{Role: "user", Content: "Search and summarize."}},
	}
	converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, bad)
	require.Error(t, err)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), "web_search")

	good := &model.GeneralOpenAIRequest{
		Model: "gemini-nano-banana-2.1",
		Messages: []model.Message{{Role: "user", Content: "Draw a duck."}},
	}
	converted, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, good)
	require.NoError(t, err)
	require.NotNil(t, converted)
}
