package gemini_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	vertexgemini "github.com/Laisky/one-api/relay/adaptor/vertexai/gemini"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestReview607NativeThinkingWireContract verifies the real Gemini/Vertex
// adaptor output, including mapped model IDs and rejection of unsupported levels.
func TestReview607NativeThinkingWireContract(t *testing.T) {
	for _, provider := range []string{"gemini", "vertex"} {
		for _, level := range []string{"absent", "default", "minimal", "high", "low", "medium", "none", "xhigh"} {
			t.Run(provider+"/"+level, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-3.1-flash-lite-image"})
				request := &model.GeneralOpenAIRequest{
					Model: "public-alias", Messages: []model.Message{{Role: "user", Content: "Draw a bird"}},
				}
				if level != "absent" {
					request.ReasoningEffort = &level
				}
				var converted any
				var err error
				if provider == "gemini" {
					converted, err = (&gemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
				} else {
					converted, err = (&vertexgemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
				}
				require.Equal(t, "public-alias", request.Model, "conversion must not mutate the caller")
				switch level {
				case "absent", "default", "minimal", "high":
					require.NoError(t, err)
					encoded, err := json.Marshal(converted)
					require.NoError(t, err)
					want := "minimal"
					if level == "high" {
						want = "high"
					}
					require.Contains(t, string(encoded), `"thinkingLevel":"`+want+`"`)
				default:
					require.ErrorContains(t, err, "reasoning effort")
					require.Nil(t, converted)
				}
			})
		}
	}
}
