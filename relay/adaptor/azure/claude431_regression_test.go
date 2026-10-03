package azure

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestClaude431AzureMappedControls checks canonical-model semantics with an opaque deployment wire name.
func TestClaude431AzureMappedControls(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-5-5", ActualModelName: "production-deployment"})
	var request model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"production-deployment","max_tokens":64,"thinking":{"type":"disabled"},"temperature":0.6,"output_config":{"effort":"low"},"messages":[{"role":"user","content":"Hello"}]}`), &request))
	converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &got))
	require.JSONEq(t, `"production-deployment"`, string(got["model"]))
	require.JSONEq(t, `{"type":"between_tools"}`, string(got["thinking"]))
	require.JSONEq(t, `{"effort":"low"}`, string(got["output_config"]))
	require.NotContains(t, got, "temperature")
}
