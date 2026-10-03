package validator

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestJinaNativeParameterWarnings verifies provider/mode scoping and unknown retention.
func TestJinaNativeParameterWarnings(t *testing.T) {
	t.Parallel()
	names := []string{"task", "embedding_type", "return_documents", "unknown_option"}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Equal(t, names, filterJinaNativeParameters(c, names))
	m := &meta.Meta{ChannelType: channeltype.Jina, Mode: relaymode.Embeddings}
	c.Set(ctxkey.Meta, m)
	require.Equal(t, []string{"return_documents", "unknown_option"}, filterJinaNativeParameters(c, names))
	m.Mode = relaymode.Rerank
	require.Equal(t, []string{"task", "embedding_type", "unknown_option"}, filterJinaNativeParameters(c, names))
	m.ChannelType = channeltype.OpenAI
	require.Equal(t, names, filterJinaNativeParameters(c, names))
}
