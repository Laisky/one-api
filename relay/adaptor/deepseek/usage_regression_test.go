package deepseek

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDeepSeekReviewAuthoritativeUsage proves the actual streamed/non-streamed
// response handler and quota calculator use upstream token/cache counts instead
// of turning a conservative image reservation into a fixed final charge.
func TestDeepSeekReviewAuthoritativeUsage(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		for _, stream := range []bool{false, true} {
			t.Run(name+"/stream-"+strconv.FormatBool(stream), func(t *testing.T) {
				t.Parallel()
				const usageJSON = `{"prompt_tokens":300,"completion_tokens":50,"total_tokens":350,"prompt_cache_hit_tokens":100,"prompt_cache_miss_tokens":200}`
				body := `{"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":` + usageJSON + `}`
				contentType := "application/json"
				if stream {
					body = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n" +
						"data: {\"choices\":[],\"usage\":" + usageJSON + "}\n\n" + "data: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				metadata := &meta.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: name, PromptTokens: 4096, IsStream: stream, Mode: relaymode.ChatCompletions}
				provider := &Adaptor{}
				usage, apiErr := provider.DoResponse(c, &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)),
				}, metadata)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				require.Equal(t, 300, usage.PromptTokens)
				require.Equal(t, 50, usage.CompletionTokens)
				require.NotNil(t, usage.PromptTokensDetails)
				require.Equal(t, 100, usage.PromptTokensDetails.CachedTokens)
				input := quota.ComputeInput{Usage: usage, ModelName: name, GroupRatio: 1, PricingAdaptor: provider, RequestTime: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)}
				final := quota.Compute(input)
				// ceil((200*0.15 + 100*0.003 + 50*0.60) * 500000/1000000).
				require.Equal(t, int64(31), final.TotalQuota)
				require.Equal(t, 300, final.PromptTokens)
				require.Equal(t, 100, final.CachedPromptTokens)
				input.Usage = &model.Usage{PromptTokens: metadata.PromptTokens}
				require.Greater(t, quota.Compute(input).TotalQuota, final.TotalQuota, "unused image reservation must not become final usage")
			})
		}
	}
}
