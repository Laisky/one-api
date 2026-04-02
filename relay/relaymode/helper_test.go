package relaymode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetByPathRealtime(t *testing.T) {
	t.Parallel()
	require.Equal(t, Realtime, GetByPath("/v1/realtime"), "expected Realtime")
	require.Equal(t, Realtime, GetByPath("/v1/realtime?model=gpt-4o-realtime-preview"), "expected Realtime with query")
}

func TestGetByPathVideos(t *testing.T) {
	t.Parallel()
	require.Equal(t, Videos, GetByPath("/v1/videos"), "expected Videos")
	require.Equal(t, Videos, GetByPath("/v1/videos/video_123"), "expected Videos with path segment")
}

func TestGetByPathOCR(t *testing.T) {
	t.Parallel()
	require.Equal(t, OCR, GetByPath("/api/paas/v4/layout_parsing"), "expected OCR for /api/paas/v4/layout_parsing")
	require.Equal(t, OCR, GetByPath("/api/paas/v4/layout_parsing?model=glm-ocr"), "expected OCR with query params")
}

func TestGetByPathOCR_NotMatched(t *testing.T) {
	t.Parallel()
	// Paths that should NOT match OCR
	require.Equal(t, Unknown, GetByPath("/v1/layout_parsing"), "v1 prefix should not match OCR")
	require.Equal(t, Unknown, GetByPath("/api/paas/v3/layout_parsing"), "v3 should not match OCR")
	require.Equal(t, Unknown, GetByPath("/layout_parsing"), "bare path should not match OCR")
}

func TestOCRModeString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "ocr", String(OCR))
}
