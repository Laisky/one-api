package azure

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// newAzureTestContext builds a gin context whose inbound *http.Request is set, so
// the header/convert helpers (which read c.Request) do not panic.
func newAzureTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

// TestSetupRequestHeader_DispatchesByModelFamily pins the auth-header seam: Claude
// models must use the native Anthropic auth (x-api-key + anthropic-version), while
// OpenAI models must use the Azure OpenAI api-key header. Getting this wrong sends
// the credential in the wrong header and every request 401s.
func TestSetupRequestHeader_DispatchesByModelFamily(t *testing.T) {
	t.Parallel()

	t.Run("claude uses native anthropic auth", func(t *testing.T) {
		a := &Adaptor{}
		c := newAzureTestContext(t, "/v1/messages")
		m := &meta.Meta{
			ChannelType:     channeltype.Azure,
			APIKey:          "secret-key",
			OriginModelName: "claude-sonnet-5",
			ActualModelName: "claude-sonnet-5",
		}
		req := httptest.NewRequest(http.MethodPost, "https://x.services.ai.azure.com/anthropic/v1/messages", nil)

		require.NoError(t, a.SetupRequestHeader(c, req, m))
		require.Equal(t, "secret-key", req.Header.Get("x-api-key"))
		require.NotEmpty(t, req.Header.Get("anthropic-version"))
		require.Empty(t, req.Header.Get("api-key"), "Azure OpenAI api-key header must not be set for Claude")
		require.Empty(t, req.Header.Get("Authorization"), "native Anthropic auth uses x-api-key, not Authorization")
	})

	t.Run("openai uses azure api-key auth", func(t *testing.T) {
		a := &Adaptor{}
		c := newAzureTestContext(t, "/v1/chat/completions")
		m := &meta.Meta{
			ChannelType:     channeltype.Azure,
			APIKey:          "secret-key",
			OriginModelName: "gpt-4o-mini",
			ActualModelName: "gpt-4o-mini",
		}
		req := httptest.NewRequest(http.MethodPost, "https://x.services.ai.azure.com/openai/deployments/gpt-4o-mini/chat/completions", nil)

		require.NoError(t, a.SetupRequestHeader(c, req, m))
		require.Equal(t, "secret-key", req.Header.Get("api-key"))
		require.Empty(t, req.Header.Get("x-api-key"), "native Anthropic header must not leak onto the OpenAI surface")
	})
}

// TestConvertClaudeRequest_DispatchesToAnthropicPassthrough verifies the native
// /v1/messages seam: a Claude request on Azure is delegated to the anthropic
// adaptor, which marks the request for direct passthrough and records the model.
func TestConvertClaudeRequest_DispatchesToAnthropicPassthrough(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	c := newAzureTestContext(t, "/v1/messages")
	m := &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-5", ActualModelName: "claude-sonnet-5"}
	meta.Set2Context(c, m)

	req := &model.ClaudeRequest{
		Model:     "claude-sonnet-5",
		MaxTokens: 128,
		Messages:  []model.ClaudeMessage{{Role: "user", Content: "hello"}},
	}

	out, err := a.ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	require.Same(t, req, out.(*model.ClaudeRequest), "native passthrough must return the original request unchanged")
	require.True(t, c.GetBool(ctxkey.ClaudeDirectPassthrough), "native Claude request must be flagged for direct passthrough")
	require.True(t, c.GetBool(ctxkey.ClaudeMessagesNative))
	require.Equal(t, "claude-sonnet-5", c.GetString(ctxkey.ClaudeModel))
}

// TestConvertRequest_DispatchesByModelFamily verifies the OpenAI-shaped chat
// completions seam: a Claude model is converted through the anthropic adaptor
// (which stamps ctxkey.ClaudeModel), while an OpenAI model stays on the OpenAI path.
func TestConvertRequest_DispatchesByModelFamily(t *testing.T) {
	t.Parallel()

	t.Run("claude routes through anthropic conversion", func(t *testing.T) {
		a := &Adaptor{}
		c := newAzureTestContext(t, "/v1/chat/completions")
		m := &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-5", ActualModelName: "claude-sonnet-5"}
		meta.Set2Context(c, m)
		req := &model.GeneralOpenAIRequest{
			Model:     "claude-sonnet-5",
			MaxTokens: 128,
			Messages:  []model.Message{{Role: "user", Content: "hi"}},
		}

		out, err := a.ConvertRequest(c, relaymode.ChatCompletions, req)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Equal(t, "claude-sonnet-5", c.GetString(ctxkey.ClaudeModel))
	})

	t.Run("openai stays on the openai path", func(t *testing.T) {
		a := &Adaptor{}
		c := newAzureTestContext(t, "/v1/chat/completions")
		m := &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "gpt-4o-mini", ActualModelName: "gpt-4o-mini"}
		meta.Set2Context(c, m)
		req := &model.GeneralOpenAIRequest{
			Model:     "gpt-4o-mini",
			MaxTokens: 128,
			Messages:  []model.Message{{Role: "user", Content: "hi"}},
		}

		out, err := a.ConvertRequest(c, relaymode.ChatCompletions, req)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Empty(t, c.GetString(ctxkey.ClaudeModel), "OpenAI models must not be routed through anthropic conversion")
	})
}

// TestDoResponse_DispatchesClaudeToAnthropicHandler verifies the response seam:
// a Claude response on Azure is parsed by the anthropic handler and usage is
// extracted correctly for billing.
func TestDoResponse_DispatchesClaudeToAnthropicHandler(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	c := newAzureTestContext(t, "/v1/messages")
	m := &meta.Meta{
		ChannelType:     channeltype.Azure,
		OriginModelName: "claude-sonnet-5",
		ActualModelName: "claude-sonnet-5",
		PromptTokens:    3,
	}

	body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5",` +
		`"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",` +
		`"usage":{"input_tokens":3,"output_tokens":5}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, errResp := a.DoResponse(c, resp, m)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.CompletionTokens, "completion tokens must come from the Anthropic response's output_tokens")
	require.Equal(t, 3, usage.PromptTokens)
}
