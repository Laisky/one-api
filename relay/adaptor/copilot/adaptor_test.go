package copilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/meta"
)

func TestNormalizeCopilotRequestPath(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/chat/completions", normalizeCopilotRequestPath("/v1/chat/completions"))
	require.Equal(t, "/embeddings", normalizeCopilotRequestPath("/v1/embeddings"))
	require.Equal(t, "/models", normalizeCopilotRequestPath("/v1/models"))
	require.Equal(t, "/v1/responses", normalizeCopilotRequestPath("/v1/responses"))
}

func TestGetRequestURL(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}

	full, err := a.GetRequestURL(&meta.Meta{BaseURL: "https://api.githubcopilot.com", RequestURLPath: "/v1/chat/completions"})
	require.NoError(t, err)
	require.Equal(t, "https://api.githubcopilot.com/chat/completions", full)

	full, err = a.GetRequestURL(&meta.Meta{BaseURL: "https://api.githubcopilot.com/", RequestURLPath: "/v1/embeddings?x=1"})
	require.NoError(t, err)
	require.Equal(t, "https://api.githubcopilot.com/embeddings?x=1", full)
}

func TestSetupRequestHeader_UsesCopilotTokenAndDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldFetcher := fetchCopilotTokenFunc
	t.Cleanup(func() {
		fetchCopilotTokenFunc = oldFetcher
		tokenMu.Lock()
		defer tokenMu.Unlock()
		tokenCache = make(map[int]cachedToken)
	})

	fetchCopilotTokenFunc = func(_ctx context.Context, _token string) (string, time.Time, error) {
		return "copilot-api-token", time.Now().Add(time.Hour), nil
	}

	r, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer github-access-token")
	req.Header.Set("Content-Type", "application/json")
	r.Request = req

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://api.githubcopilot.com/chat/completions", nil)

	a := &Adaptor{}
	m := &meta.Meta{ChannelId: 123, APIKey: "github-access-token", IsStream: false}
	err := a.SetupRequestHeader(r, upstreamReq, m)
	require.NoError(t, err)

	require.Equal(t, "Bearer copilot-api-token", upstreamReq.Header.Get("Authorization"))
	require.Equal(t, defaultEditorVersion, upstreamReq.Header.Get("editor-version"))
	require.Equal(t, defaultEditorPluginVersion, upstreamReq.Header.Get("editor-plugin-version"))
	require.Equal(t, defaultIntegrationID, upstreamReq.Header.Get("Copilot-Integration-Id"))
	require.Equal(t, defaultUserAgent, upstreamReq.Header.Get("User-Agent"))
}

func TestSetupRequestHeader_AllowsXHeaderOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldFetcher := fetchCopilotTokenFunc
	t.Cleanup(func() {
		fetchCopilotTokenFunc = oldFetcher
		tokenMu.Lock()
		defer tokenMu.Unlock()
		tokenCache = make(map[int]cachedToken)
	})

	fetchCopilotTokenFunc = func(_ctx context.Context, _token string) (string, time.Time, error) {
		return "copilot-api-token", time.Now().Add(time.Hour), nil
	}

	r, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer github-access-token")
	req.Header.Set("X-editor-version", "vscode/9.9.9")
	r.Request = req

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://api.githubcopilot.com/chat/completions", nil)

	a := &Adaptor{}
	m := &meta.Meta{ChannelId: 999, APIKey: "github-access-token"}
	err := a.SetupRequestHeader(r, upstreamReq, m)
	require.NoError(t, err)

	require.Equal(t, "vscode/9.9.9", upstreamReq.Header.Get("editor-version"))
}

func TestGetCopilotAPIToken_RejectsEmptyGithubToken(t *testing.T) {
	t.Parallel()

	_, err := GetCopilotAPIToken(context.Background(), 1, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "github access token is empty")
}
