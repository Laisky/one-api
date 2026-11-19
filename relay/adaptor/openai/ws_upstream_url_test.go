package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
)

// TestValidateProviderWebSocketURL pins the shared dial-site policy: plain
// ws/wss targets built from channel configuration pass unchanged, while
// userinfo, fragments, other schemes, whitespace and control characters are
// refused without echoing the URL. Parameters: t is the test handle.
// Returns: none.
func TestValidateProviderWebSocketURL(t *testing.T) {
	t.Parallel()
	for _, valid := range []string{
		"wss://api.openai.com/v1/realtime?model=gpt-realtime",
		"wss://api.openai.com/v1/realtime?intent=transcription",
		"ws://127.0.0.1:8080/v1/realtime",
		"ws://localhost:8080/v1/responses",
		"wss://my_service.internal:443/custom/realtime",
		"wss://[::1]:9000/v1/realtime",
		"wss://example.openai.azure.com/openai/v1/realtime?model=dep",
		"wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContent",
		"wss://us-central1-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1.LlmBidiService/BidiGenerateContent",
	} {
		got, err := ValidateProviderWebSocketURL(valid)
		require.NoError(t, err, "url=%q", valid)
		require.Equal(t, valid, got)
	}
	for _, invalid := range []string{
		"",
		"https://api.openai.com/v1/realtime",
		"ftp://api.openai.com/v1/realtime",
		"wss://user:secret@api.openai.com/v1/realtime",
		"wss://api.openai.com/v1/realtime#secret",
		"wss:///v1/realtime",
		"wss://api.openai.com/v1/realtime?model=a b",
		"wss://api.openai.com/v1/realtime?model=a\r\nX-Injected: 1",
		"wss://api.openai.com/v1/real time",
		"wss:opaque-secret",
		"wss://api openai.com/v1/realtime",
	} {
		got, err := ValidateProviderWebSocketURL(invalid)
		require.ErrorIs(t, err, ErrInvalidProviderWebSocketURL, "url=%q", invalid)
		require.Empty(t, got)
		require.NotContains(t, err.Error(), "secret")
	}
}

// TestProviderWebSocketBuildersFailClosed verifies a channel URL that cannot
// yield a valid provider WebSocket URL is refused rather than dialed: the
// realtime builder used to fall back to wss://api.openai.com (sending the
// channel credential to a host the administrator never configured).
// Parameters: t is the test handle. Returns: none.
func TestProviderWebSocketBuildersFailClosed(t *testing.T) {
	t.Parallel()
	for _, base := range []string{
		"https://user:secret@channel.example",
		"ftp://channel.example",
		"https://channel.example/%zz",
		"://broken",
	} {
		got, _, err := realtimeWebSocketUpstreamURL(realtimeMeta(base, "", "gpt-realtime"), "model=gpt-realtime")
		require.ErrorIs(t, err, ErrInvalidProviderWebSocketURL, "base=%q", base)
		require.Empty(t, got)
		require.NotContains(t, err.Error(), "secret")
	}
	got, _, err := realtimeWebSocketUpstreamURL(realtimeMeta("https://channel.example", "https://user:secret@override.example/rt", "gpt-realtime"), "")
	require.ErrorIs(t, err, ErrInvalidProviderWebSocketURL)
	require.Empty(t, got)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	resolved, _, err := resolveResponseAPIWebSocketUpstreamURL(ctx, &rmeta.Meta{BaseURL: "https://user:secret@channel.example"})
	require.ErrorIs(t, err, ErrInvalidProviderWebSocketURL)
	require.Empty(t, resolved)
}
