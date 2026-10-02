package zhipu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetRequestURLVideos verifies video generation requests route to Zhipu's
// async videos/generations endpoint.
func TestGetRequestURLVideos(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	m := &meta.Meta{BaseURL: "https://open.bigmodel.cn", Mode: relaymode.Videos, ActualModelName: "viduq1-text"}
	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/videos/generations", url)
}

// TestGetRequestURLVoiceClone verifies voice-clone requests route to Zhipu's
// native voice/clone endpoint.
func TestGetRequestURLVoiceClone(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	m := &meta.Meta{BaseURL: "https://open.bigmodel.cn", Mode: relaymode.VoiceClone, ActualModelName: "glm-tts-clone"}
	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/voice/clone", url)
}

// TestGetRequestURLChat verifies v4 chat routing is unchanged for GLM models.
func TestGetRequestURLChat(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	m := &meta.Meta{BaseURL: "https://open.bigmodel.cn", Mode: relaymode.ChatCompletions, ActualModelName: "glm-5.3"}
	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/chat/completions", url)
}

// TestRealtimeUpstreamURL verifies the GLM-Realtime WebSocket endpoint and the
// model query-parameter binding (verified against the live API).
func TestRealtimeUpstreamURL(t *testing.T) {
	t.Parallel()
	require.Equal(t, "wss://open.bigmodel.cn/api/paas/v4/realtime", realtimeUpstreamURL(""))
	require.Equal(t, "wss://open.bigmodel.cn/api/paas/v4/realtime?model=glm-realtime-flash", realtimeUpstreamURL("glm-realtime-flash"))
	require.Equal(t, "wss://open.bigmodel.cn/api/paas/v4/realtime?model=glm-realtime-air", realtimeUpstreamURL("glm-realtime-air"))
}
