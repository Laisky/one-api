package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestClaude431VertexNativePreservation checks that native Claude payloads do not pass through a lossy Chat round trip.
func TestClaude431VertexNativePreservation(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	meta.Set2Context(c, &meta.Meta{ActualModelName: "claude-sonnet-5-5", OriginModelName: "claude-sonnet-5-5"})
	var request model.ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-5-5","max_tokens":64,"thinking":{"type":"between_tools"},"system":[{"type":"text","text":"policy","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"signed"},{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"ok"}]}]}`), &request))
	converted, err := (&Adaptor{}).ConvertClaudeRequest(c, &request)
	require.NoError(t, err)
	require.True(t, c.GetBool(ctxkey.ClaudeDirectPassthrough))
	require.Same(t, &request, converted)
}

// TestClaude431VertexChatControls checks the child wire object, not just catalog metadata.
func TestClaude431VertexChatControls(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	meta.Set2Context(c, &meta.Meta{ActualModelName: "claude-sonnet-5-5", OriginModelName: "claude-sonnet-5-5"})
	var request model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-5-5","max_tokens":64,"thinking":{"type":"between_tools"},"output_config":{"effort":"medium"},"stop":["END"],"tool_choice":"none","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"Hello"}]}`), &request))
	converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &got))
	require.JSONEq(t, `{"type":"between_tools"}`, string(got["thinking"]))
	require.JSONEq(t, `{"effort":"medium"}`, string(got["output_config"]))
	require.JSONEq(t, `{"type":"none"}`, string(got["tool_choice"]))
	require.JSONEq(t, `["END"]`, string(got["stop_sequences"]))
	require.Equal(t, "64", string(got["max_tokens"]))
}
