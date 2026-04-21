package fireworks

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestGetRequestURL_PreservesAllSurfaces(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	base := "https://api.fireworks.ai/inference"

	cases := map[string]string{
		"/v1/chat/completions": base + "/v1/chat/completions",
		"/v1/completions":      base + "/v1/completions",
		"/v1/embeddings":       base + "/v1/embeddings",
		"/v1/responses":        base + "/v1/responses",
		// Anthropic-compatible Messages endpoint must be forwarded natively so
		// the controller can stream Anthropic SSE events unchanged.
		"/v1/messages": base + "/v1/messages",
	}

	for path, want := range cases {
		got, err := a.GetRequestURL(&meta.Meta{
			BaseURL:        base,
			RequestURLPath: path,
			ChannelType:    channeltype.Fireworks,
		})
		require.NoError(t, err, "path=%s", path)
		require.Equal(t, want, got, "path=%s", path)
	}
}

func TestConvertClaudeRequest_SetsDirectPassthroughFlags(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	a := &Adaptor{}
	req := &model.ClaudeRequest{
		Model:     "accounts/fireworks/models/kimi-k2p5",
		MaxTokens: 32,
	}

	converted, err := a.ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	require.Equal(t, req, converted, "passthrough should return the original request")

	passthrough, ok := c.Get(ctxkey.ClaudeDirectPassthrough)
	require.True(t, ok, "ClaudeDirectPassthrough must be set so the controller forwards the body verbatim")
	flag, ok := passthrough.(bool)
	require.True(t, ok)
	require.True(t, flag)

	native, ok := c.Get(ctxkey.ClaudeMessagesNative)
	require.True(t, ok)
	nativeFlag, ok := native.(bool)
	require.True(t, ok)
	require.True(t, nativeFlag)

	claudeModel, ok := c.Get(ctxkey.ClaudeModel)
	require.True(t, ok)
	require.Equal(t, req.Model, claudeModel)
}

func TestPricingLookup_KnownFlagship(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}

	// A model we curated with custom pricing.
	flagship := "accounts/fireworks/models/kimi-k2p5"
	require.Greater(t, a.GetModelRatio(flagship), 0.0, "flagship model should have non-zero ratio")
	require.Greater(t, a.GetCompletionRatio(flagship), 1.0, "flagship completion ratio must exceed 1.0")

	// Unknown model falls back to DefaultPricingMethods.
	unknown := "accounts/fireworks/models/does-not-exist"
	require.Greater(t, a.GetModelRatio(unknown), 0.0)
	require.Equal(t, 1.0, a.GetCompletionRatio(unknown), "unknown model falls back to 1.0 completion ratio")
}

func TestGetModelList_Nonempty(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	require.NotEmpty(t, models)
	require.Equal(t, len(ModelRatios), len(models))
}

func TestConvertRerankRequest_PassesThroughRequest(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	req := &model.RerankRequest{
		Model:     "accounts/fireworks/models/qwen3-reranker-8b",
		Query:     "q",
		Documents: []string{"a", "b"},
	}

	converted, err := (&Adaptor{}).ConvertRerankRequest(c, req)
	require.NoError(t, err)
	// Fireworks already speaks the canonical rerank shape, so the adaptor must
	// hand back the exact same pointer rather than allocate a copy.
	require.Same(t, req, converted)
}

func TestHandleRerankResponse_ParsesFireworksEnvelope(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := []byte(`{"object":"list","model":"accounts/fireworks/models/qwen3-reranker-8b","data":[{"index":0,"relevance_score":0.9,"document":"a"}],"usage":{"prompt_tokens":12,"completion_tokens":0,"total_tokens":12}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}

	errResp, usage := handleRerankResponse(c, resp, 0)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 12, usage.PromptTokens)
	require.Equal(t, 12, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), `"relevance_score":0.9`)
}
