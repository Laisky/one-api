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

func TestNormalizeResponseAPIRawBodyEnableThinkingViaExtraBody(t *testing.T) {
	t.Parallel()
	// DashScope Bailian: enable_thinking via extra_body should be flattened to root.
	raw := []byte(`{
	  "model": "qwen3.5-27b",
	  "input": "hello",
	  "extra_body": {
	    "enable_thinking": false
	  }
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, _, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.AliBailian)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, false, root["enable_thinking"])
}

func TestNormalizeResponseAPIRawBodyEnableThinkingAtRoot(t *testing.T) {
	t.Parallel()
	// DashScope Bailian: enable_thinking at root level should be preserved.
	raw := []byte(`{
	  "model": "qwen3.5-27b",
	  "input": "hello",
	  "enable_thinking": false
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, _, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.AliBailian)
	require.NoError(t, err)
	// The enable_thinking field should be present in the result
	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	require.Equal(t, false, root["enable_thinking"])
	_ = changed
}

func TestNormalizeResponseAPIRawBodyThinkingBudgetViaExtraBody(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "model": "qwen3.5-27b",
	  "input": "hello",
	  "extra_body": {
	    "enable_thinking": true,
	    "thinking_budget": 4096
	  }
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, _, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.AliBailian)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, true, root["enable_thinking"])
	require.Equal(t, float64(4096), root["thinking_budget"])
}
