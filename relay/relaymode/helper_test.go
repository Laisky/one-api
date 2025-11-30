package relaymode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetByPathRealtime(t *testing.T) {
	require.Equal(t, Realtime, GetByPath("/v1/realtime"), "expected Realtime")
	require.Equal(t, Realtime, GetByPath("/v1/realtime?model=gpt-4o-realtime-preview"), "expected Realtime with query")
}

func TestGetByPathVideos(t *testing.T) {
	require.Equal(t, Videos, GetByPath("/v1/videos"), "expected Videos")
	require.Equal(t, Videos, GetByPath("/v1/videos/video_123"), "expected Videos with path segment")
}
