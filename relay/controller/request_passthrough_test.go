package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeControlledPassthroughJSONFlattensExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","extra_body":{"chat_template_kwargs":{"enable_thinking":false},"priority":7}}`)
	updated := []byte(`{"model":"qwen"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, float64(7), root["priority"])
	kwargs := root["chat_template_kwargs"].(map[string]any)
	require.Equal(t, false, kwargs["enable_thinking"])
}

func TestMergeControlledPassthroughJSONDoesNotOverrideExplicitRoot(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","chat_template_kwargs":{"enable_thinking":false}}`)
	updated := []byte(`{"model":"qwen","extra_body":{"chat_template_kwargs":{"enable_thinking":true}}}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Zero(t, stats.ExtraBodyRejected)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	kwargs := root["chat_template_kwargs"].(map[string]any)
	require.Equal(t, false, kwargs["enable_thinking"])
}

func TestMergeControlledPassthroughJSONPreservesOnlyAllowlistedRootKeys(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","chat_template_kwargs":{"enable_thinking":false},"unknown_vendor_flag":true}`)
	updated := []byte(`{"model":"mapped-qwen"}`)

	merged, _, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, "mapped-qwen", root["model"])
	require.Contains(t, root, "chat_template_kwargs")
	require.NotContains(t, root, "unknown_vendor_flag")
}

func TestMergeControlledPassthroughJSONRejectsMalformedExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","extra_body":"not-an-object"}`)
	updated := []byte(`{"model":"qwen","extra_body":"still-not-an-object"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyRejected)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, "qwen", root["model"])
}

func TestMergeControlledPassthroughJSONRejectsBlankExtraBodyKeys(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","extra_body":{"   ":1,"priority":7}}`)
	updated := []byte(`{"model":"qwen"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyRejected)
	require.Equal(t, 1, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, float64(7), root["priority"])
	require.NotContains(t, root, "   ")
}

func TestMergeControlledPassthroughJSONPreservesUnknownKeysWhenAllowed(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","x-trace-id":"abc123"}`)
	updated := []byte(`{"model":"mapped-qwen"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, true)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.UnknownPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, "mapped-qwen", root["model"])
	require.Equal(t, "abc123", root["x-trace-id"])
}

// --- enable_thinking passthrough tests (DashScope / Ali Bailian) ---

func TestMergeControlledPassthroughJSONEnableThinkingViaExtraBody(t *testing.T) {
	t.Parallel()
	// Simulates DashScope/Bailian: user sends enable_thinking in extra_body,
	// it should be flattened to root level.
	original := []byte(`{"model":"qwen3.5-27b","messages":[{"role":"user","content":"hi"}],"extra_body":{"enable_thinking":false}}`)
	updated := []byte(`{"model":"qwen3.5-27b","messages":[{"role":"user","content":"hi"}]}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, false, root["enable_thinking"])
}

func TestMergeControlledPassthroughJSONEnableThinkingTrueViaExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen3.5-27b","extra_body":{"enable_thinking":true}}`)
	updated := []byte(`{"model":"qwen3.5-27b"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, true, root["enable_thinking"])
}

func TestMergeControlledPassthroughJSONEnableThinkingAtRootLevel(t *testing.T) {
	t.Parallel()
	// Simulates DashScope/Bailian: user sends enable_thinking at root level,
	// it should be preserved even when allowUnknown is false.
	original := []byte(`{"model":"qwen3.5-27b","messages":[{"role":"user","content":"hi"}],"enable_thinking":false}`)
	updated := []byte(`{"model":"qwen3.5-27b","messages":[{"role":"user","content":"hi"}]}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.AllowedRootPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, false, root["enable_thinking"])
}

func TestMergeControlledPassthroughJSONEnableThinkingTrueAtRootLevel(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen3.5-27b","enable_thinking":true}`)
	updated := []byte(`{"model":"qwen3.5-27b"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.AllowedRootPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["enable_thinking"])
}

func TestMergeControlledPassthroughJSONEnableThinkingRootWinsOverExtraBody(t *testing.T) {
	t.Parallel()
	// When enable_thinking is in both root and extra_body, root-level value wins.
	original := []byte(`{"model":"qwen3.5-27b","enable_thinking":false,"extra_body":{"enable_thinking":true}}`)
	updated := []byte(`{"model":"qwen3.5-27b"}`)

	merged, _, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	// Root-level value (false) should take precedence
	require.Equal(t, false, root["enable_thinking"])
}

func TestMergeControlledPassthroughJSONEnableThinkingNotOverriddenByUpdated(t *testing.T) {
	t.Parallel()
	// If the converted request already has enable_thinking (e.g. set by server logic),
	// the user's extra_body value should not override it.
	original := []byte(`{"model":"qwen3.5-27b","extra_body":{"enable_thinking":false}}`)
	updated := []byte(`{"model":"qwen3.5-27b","enable_thinking":true}`)

	merged, stats, _, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.Equal(t, 1, stats.ExtraBodySkipped)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	// The updated (server-set) value wins
	require.Equal(t, true, root["enable_thinking"])
}

func TestMergeControlledPassthroughJSONEnableThinkingWithAllowUnknownTrue(t *testing.T) {
	t.Parallel()
	// With allowUnknown=true, enable_thinking at root is preserved via
	// AllowedRootPreserved (not UnknownPreserved), because it's in the allowlist.
	original := []byte(`{"model":"qwen3.5-27b","enable_thinking":false,"x-custom":"val"}`)
	updated := []byte(`{"model":"qwen3.5-27b"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, true)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.AllowedRootPreserved) // enable_thinking
	require.Equal(t, 1, stats.UnknownPreserved)      // x-custom

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, false, root["enable_thinking"])
	require.Equal(t, "val", root["x-custom"])
}

func TestMergeControlledPassthroughJSONFullDashScopeRequest(t *testing.T) {
	t.Parallel()
	// Full simulation of a DashScope Bailian request with all typical fields.
	original := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"system","content":"回复问题"},{"role":"user","content":"你好"}],
		"enable_thinking":false,
		"stream":false
	}`)
	updated := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"system","content":"回复问题"},{"role":"user","content":"你好"}],
		"stream":false
	}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.AllowedRootPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, false, root["enable_thinking"])
	require.Equal(t, false, root["stream"])
	require.Equal(t, "qwen3.5-27b", root["model"])
}

func TestMergeControlledPassthroughJSONFullDashScopeRequestWithExtraBody(t *testing.T) {
	t.Parallel()
	// Full simulation: user sends enable_thinking via extra_body (the reported bug scenario).
	original := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"system","content":"回复问题"},{"role":"user","content":"你好"}],
		"extra_body":{"enable_thinking":false},
		"stream":false
	}`)
	updated := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"system","content":"回复问题"},{"role":"user","content":"你好"}],
		"stream":false
	}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, false, root["enable_thinking"])
	require.Equal(t, false, root["stream"])
}

func TestMergeControlledPassthroughJSONEnableThinkingWithBothInOriginal(t *testing.T) {
	t.Parallel()
	// User accidentally sends enable_thinking at both root and in extra_body
	// (the exact scenario from the bug report).
	original := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"user","content":"你好"}],
		"enable_thinking":false,
		"extra_body":{"enable_thinking":false},
		"stream":false
	}`)
	updated := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"user","content":"你好"}],
		"stream":false
	}`)

	merged, _, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, false, root["enable_thinking"])
}

// --- allowlist regression tests ---

// --- DashScope thinking_budget passthrough tests ---

func TestMergeControlledPassthroughJSONThinkingBudgetViaExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen3.5-27b","extra_body":{"enable_thinking":true,"thinking_budget":4096}}`)
	updated := []byte(`{"model":"qwen3.5-27b"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, true, root["enable_thinking"])
	require.Equal(t, float64(4096), root["thinking_budget"])
}

func TestMergeControlledPassthroughJSONThinkingBudgetAtRootLevel(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen3.5-27b","enable_thinking":true,"thinking_budget":2048}`)
	updated := []byte(`{"model":"qwen3.5-27b"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, stats.AllowedRootPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["enable_thinking"])
	require.Equal(t, float64(2048), root["thinking_budget"])
}

// --- DashScope preserve_thinking passthrough tests ---

func TestMergeControlledPassthroughJSONPreserveThinkingViaExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen3.6-plus","extra_body":{"preserve_thinking":true}}`)
	updated := []byte(`{"model":"qwen3.6-plus"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["preserve_thinking"])
}

func TestMergeControlledPassthroughJSONPreserveThinkingAtRootLevel(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen3.6-plus","preserve_thinking":true}`)
	updated := []byte(`{"model":"qwen3.6-plus"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.AllowedRootPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["preserve_thinking"])
}

// --- DashScope enable_search passthrough tests ---

func TestMergeControlledPassthroughJSONEnableSearchViaExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen-plus","extra_body":{"enable_search":true}}`)
	updated := []byte(`{"model":"qwen-plus"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["enable_search"])
}

func TestMergeControlledPassthroughJSONEnableSearchAtRootLevel(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen-plus","enable_search":true}`)
	updated := []byte(`{"model":"qwen-plus"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.AllowedRootPreserved)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["enable_search"])
}

// --- DashScope search_options passthrough tests ---

func TestMergeControlledPassthroughJSONSearchOptionsViaExtraBody(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen-plus","extra_body":{"enable_search":true,"search_options":{"forced_search":true}}}`)
	updated := []byte(`{"model":"qwen-plus"}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, stats.ExtraBodyMerged)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["enable_search"])
	opts := root["search_options"].(map[string]any)
	require.Equal(t, true, opts["forced_search"])
}

// --- Full DashScope request with all thinking parameters ---

func TestMergeControlledPassthroughJSONFullDashScopeThinkingRequest(t *testing.T) {
	t.Parallel()
	// Complete DashScope request with all thinking-related parameters.
	original := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"user","content":"你好"}],
		"enable_thinking":true,
		"thinking_budget":4096,
		"stream":true,
		"stream_options":{"include_usage":true}
	}`)
	updated := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"user","content":"你好"}],
		"stream":true,
		"stream_options":{"include_usage":true}
	}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, stats.AllowedRootPreserved) // enable_thinking + thinking_budget

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, true, root["enable_thinking"])
	require.Equal(t, float64(4096), root["thinking_budget"])
	require.Equal(t, true, root["stream"])
}

func TestMergeControlledPassthroughJSONFullDashScopeThinkingViaExtraBody(t *testing.T) {
	t.Parallel()
	// Complete DashScope request passing thinking parameters via extra_body.
	original := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"user","content":"你好"}],
		"extra_body":{"enable_thinking":true,"thinking_budget":4096},
		"stream":true
	}`)
	updated := []byte(`{
		"model":"qwen3.5-27b",
		"messages":[{"role":"user","content":"你好"}],
		"stream":true
	}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2, stats.ExtraBodyMerged) // enable_thinking + thinking_budget

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.NotContains(t, root, "extra_body")
	require.Equal(t, true, root["enable_thinking"])
	require.Equal(t, float64(4096), root["thinking_budget"])
}

// --- allowlist regression tests ---

func TestIsAllowedExtraBodyKeyEnableThinking(t *testing.T) {
	t.Parallel()
	require.True(t, isAllowedExtraBodyKey("enable_thinking"),
		"enable_thinking must be in the allowlist for DashScope/Bailian passthrough")
}

func TestIsAllowedExtraBodyKeyDashScopeKeys(t *testing.T) {
	t.Parallel()
	// Regression: DashScope-specific parameters must remain in the allowlist.
	dashScopeKeys := []string{
		"enable_thinking",
		"thinking_budget",
		"preserve_thinking",
		"enable_search",
		"search_options",
	}
	for _, key := range dashScopeKeys {
		require.True(t, isAllowedExtraBodyKey(key),
			"DashScope parameter %q must be in the allowlist", key)
	}
}

func TestIsAllowedExtraBodyKeyVLLMKeys(t *testing.T) {
	t.Parallel()
	// Regression: vLLM-specific parameters must remain in the allowlist.
	vllmKeys := []string{
		"chat_template_kwargs",
		"priority",
		"vllm_xargs",
		"repetition_penalty",
		"min_p",
		"top_k",
		"min_tokens",
		"stop_token_ids",
		"skip_special_tokens",
	}
	for _, key := range vllmKeys {
		require.True(t, isAllowedExtraBodyKey(key),
			"vLLM parameter %q must be in the allowlist", key)
	}
}

func TestIsAllowedExtraBodyKeyRejectsUnknown(t *testing.T) {
	t.Parallel()
	unknownKeys := []string{
		"unknown_param",
		"api_key",
		"authorization",
		"",
	}
	for _, key := range unknownKeys {
		require.False(t, isAllowedExtraBodyKey(key),
			"unknown parameter %q must NOT be in the allowlist", key)
	}
}

func TestMergeControlledPassthroughJSONPrefersRawExtraBodyOverTypedDefaults(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"qwen","extra_body":{"priority":9}}`)
	updated := []byte(`{"model":"qwen","extra_body":{"priority":3}}`)

	merged, stats, changed, err := mergeControlledPassthroughJSON(original, updated, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ExtraBodyMerged)
	require.Zero(t, stats.ExtraBodySkipped)

	var root map[string]any
	require.NoError(t, json.Unmarshal(merged, &root))
	require.Equal(t, float64(9), root["priority"])
	require.NotContains(t, root, "extra_body")
}
