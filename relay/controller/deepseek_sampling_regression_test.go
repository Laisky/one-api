package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDeepSeekReviewSamplingAcrossFormats checks actual request parsing and
// serialization for every API name. Provider-controlled top_p must survive both
// sanitizers, not only the canonical name that bypassed the old V4 predicate.
func TestDeepSeekReviewSamplingAcrossFormats(t *testing.T) {
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		for _, format := range []string{"chat", "responses"} {
			t.Run(name+"/"+format, func(t *testing.T) {
				body := `{"model":"` + name + `","messages":[{"role":"user","content":"hello"}],"thinking":{"type":"enabled"},"top_p":0.97}`
				if format == "responses" {
					body = `{"model":"` + name + `","input":"hello","reasoning":{"effort":"high"},"top_p":0.97}`
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				var outgoing any
				if format == "chat" {
					request, err := getAndValidateTextRequest(c, relaymode.ChatCompletions)
					require.NoError(t, err)
					sanitizeChatCompletionRequest(request)
					outgoing, err = (&deepseek.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
					require.NoError(t, err)
				} else {
					request, err := getAndValidateResponseAPIRequest(c)
					require.NoError(t, err)
					sanitizeResponseAPIRequest(request, channeltype.DeepSeek)
					outgoing = request
				}
				encoded, err := json.Marshal(outgoing)
				require.NoError(t, err)
				var wire map[string]any
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Equal(t, 0.97, wire["top_p"], "a supported sampling control must not disappear from the upstream payload")
			})
		}
	}
}

// TestDeepSeekReviewOpenAISamplingControl keeps OpenAI's existing fixed-sampling
// handling as a negative control when DeepSeek's sampling support is corrected.
func TestDeepSeekReviewOpenAISamplingControl(t *testing.T) {
	t.Parallel()
	topP := 0.97
	chat := &relaymodel.GeneralOpenAIRequest{Model: "gpt-5-mini", TopP: &topP}
	sanitizeChatCompletionRequest(chat)
	require.Nil(t, chat.TopP)
	response := &openai.ResponseAPIRequest{Model: "gpt-5-mini", TopP: &topP}
	sanitizeResponseAPIRequest(response, channeltype.OpenAI)
	require.Nil(t, response.TopP)
}
