package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
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

// TestNormalizeResponseAPIRawBodyPreservesCustomToolFields verifies that native
// Responses passthrough does not erase Codex custom-tool grammar metadata that
// is intentionally unknown to one-api's typed compatibility model.
func TestNormalizeResponseAPIRawBodyPreservesCustomToolFields(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "model": "deepseek-v4-flash",
	  "input": "apply the patch",
	  "tools": [{
	    "type": "custom",
	    "name": "apply_patch",
	    "description": "Apply a patch",
	    "format": {"type": "grammar", "syntax": "lark", "definition": "start: /.+/"},
	    "provider_extension": {"mode": "strict"}
	  }]
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, _, _, err := normalizeResponseAPIRawBody(raw, &req, channeltype.DeepSeek)
	require.NoError(t, err)

	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	tools, ok := root["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Contains(t, tool, "format")
	require.Contains(t, tool, "provider_extension")
}

// TestNormalizeResponseAPIRawBodyPreservesUnknownFieldsAcrossSiblingChanges
// verifies that sanitizing one typed tool does not erase extension fields from
// an unchanged custom sibling in the same Codex request.
func TestNormalizeResponseAPIRawBodyPreservesUnknownFieldsAcrossSiblingChanges(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "model": "deepseek-v4-flash",
	  "input": "apply the patch",
	  "tools": [
	    {
	      "type": "custom",
	      "name": "apply_patch",
	      "format": {"type": "grammar", "syntax": "lark", "definition": "start: /.+/"}
	    },
	    {
	      "type": "function",
	      "name": "read_file",
	      "description": "original",
	      "parameters": {"type": "object"}
	    }
	  ]
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))
	req.Tools[1].Description = "sanitized"
	req.Tools[1].Function.Description = "sanitized"

	patched, _, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.DeepSeek)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	tools := root["tools"].([]any)
	require.Len(t, tools, 2)
	custom := tools[0].(map[string]any)
	require.Contains(t, custom, "format")
	function := tools[1].(map[string]any)
	require.Equal(t, "sanitized", function["description"])
}
