package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestResponseAli2OpenAICapturesCachedTokens verifies that the DashScope-native
// non-streaming usage object maps cache-hit tokens (usage.prompt_tokens_details.cached_tokens)
// into model.Usage.PromptTokensDetails.CachedTokens while keeping PromptTokens as the
// full input count (DashScope input_tokens already includes the cached portion).
func TestResponseAli2OpenAICapturesCachedTokens(t *testing.T) {
	t.Parallel()

	resp := &ChatResponse{
		Output: Output{},
		Usage: Usage{
			InputTokens:  3019,
			OutputTokens: 101,
			TotalTokens:  3120,
			PromptTokensDetails: &PromptTokensDetails{
				CachedTokens: 2048,
			},
		},
	}

	full := responseAli2OpenAI(resp)
	require.Equal(t, 3019, full.Usage.PromptTokens, "PromptTokens must remain the full input count")
	require.NotNil(t, full.Usage.PromptTokensDetails, "PromptTokensDetails must be populated when cache hit reported")
	require.Equal(t, 2048, full.Usage.PromptTokensDetails.CachedTokens, "cached tokens must be propagated")
}

// TestResponseAli2OpenAICapturesTopLevelCachedTokens verifies the multimodal
// DashScope-native shape where the cache-hit count is reported as a top-level
// usage.cached_tokens (alongside input_tokens_details) is also captured.
func TestResponseAli2OpenAICapturesTopLevelCachedTokens(t *testing.T) {
	t.Parallel()

	resp := &ChatResponse{
		Usage: Usage{
			InputTokens:  1292,
			OutputTokens: 87,
			TotalTokens:  1379,
			CachedTokens: 1152,
		},
	}

	full := responseAli2OpenAI(resp)
	require.Equal(t, 1292, full.Usage.PromptTokens)
	require.NotNil(t, full.Usage.PromptTokensDetails)
	require.Equal(t, 1152, full.Usage.PromptTokensDetails.CachedTokens)
}

// TestStreamHandlerCapturesCachedTokens verifies the DashScope-native streaming
// path forwards cache-hit tokens into the billing usage snapshot.
func TestStreamHandlerCapturesCachedTokens(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, glog.Shared.Named("ali-stream-test"))

	sse := "data: {\"output\":{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"hi\"},\"finish_reason\":\"stop\"}]},\"usage\":{\"input_tokens\":3019,\"output_tokens\":101,\"total_tokens\":3120,\"prompt_tokens_details\":{\"cached_tokens\":2048}}}\n\n"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
	}

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 3019, usage.PromptTokens, "PromptTokens must remain the full input count")
	require.Equal(t, 101, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails, "PromptTokensDetails must be populated from streamed usage")
	require.Equal(t, 2048, usage.PromptTokensDetails.CachedTokens)
}
