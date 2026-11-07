package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestVertexClaudeRequestURLFollowsStreaming verifies a streaming Claude
// request calls streamRawPredict and a non-streaming one rawPredict, the
// pairing Google documents and Anthropic's Vertex client uses. Calling
// rawPredict for a stream returned one JSON body to a reader expecting SSE.
func TestVertexClaudeRequestURLFollowsStreaming(t *testing.T) {
	t.Parallel()
	for stream, specifier := range map[bool]string{false: "rawPredict", true: "streamRawPredict"} {
		requestMeta := &meta.Meta{ActualModelName: "claude-opus-5-5", BaseURL: "https://aiplatform.googleapis.com", IsStream: stream}
		requestMeta.Config.VertexAIProjectID = "stream-test-project"
		requestMeta.Config.Region = "global"
		url, err := (&Adaptor{}).GetRequestURL(requestMeta)
		require.NoError(t, err)
		require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/stream-test-project/locations/global/publishers/anthropic/models/claude-opus-5-5:"+specifier, url)
	}
}
