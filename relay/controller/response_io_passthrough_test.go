package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

func TestNormalizeResponseAPIRawBodyFlattensExtraBody(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "model": "Qwen/Qwen3.5-35B-A3B",
	  "input": "hello",
	  "extra_body": {
	    "chat_template_kwargs": {"enable_thinking": false},
	    "priority": 5
	  }
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, _, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.OpenAICompatible)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, float64(5), root["priority"])
	kwargs := root["chat_template_kwargs"].(map[string]any)
	require.Equal(t, false, kwargs["enable_thinking"])
}
