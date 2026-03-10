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
