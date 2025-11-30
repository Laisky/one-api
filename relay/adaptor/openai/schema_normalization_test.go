package openai

import (
	"reflect"
	"testing"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

func TestNormalizeStructuredJSONSchema_RemovesNumericBounds(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score": map[string]any{
				"type":             "number",
				"minimum":          0,
				"maximum":          1,
				"exclusiveMinimum": 0,
			},
		},
	}

	normalized, changed := NormalizeStructuredJSONSchema(schema, channeltype.OpenAI)
	require.True(t, changed, "expected schema normalization to report changes")

	props, ok := normalized["properties"].(map[string]any)
	require.True(t, ok, "expected properties map, got %T", normalized["properties"])

	score, ok := props["score"].(map[string]any)
	require.True(t, ok, "expected score map, got %T", props["score"])

	_, exists := score["minimum"]
	require.False(t, exists, "minimum key should be removed")
	_, exists = score["maximum"]
	require.False(t, exists, "maximum key should be removed")
	_, exists = score["exclusiveMinimum"]
	require.False(t, exists, "exclusiveMinimum key should be removed")
}

func TestNormalizeStructuredJSONSchema_AzureAddsAdditionalProperties(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic": map[string]any{"type": "string"},
		},
	}

	normalized, changed := NormalizeStructuredJSONSchema(schema, channeltype.Azure)
	require.True(t, changed, "expected azure normalization to set additionalProperties=false")

	val, ok := normalized["additionalProperties"].(bool)
	require.True(t, ok, "expected additionalProperties to be bool, got %T", normalized["additionalProperties"])
	require.False(t, val, "expected additionalProperties=false")
}

func TestNormalizeStructuredJSONSchema_NoChangeWhenClean(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"topic": map[string]any{"type": "string"},
		},
	}

	copy := reflect.ValueOf(schema).Interface().(map[string]any)
	normalized, changed := NormalizeStructuredJSONSchema(schema, channeltype.OpenAI)
	require.False(t, changed, "expected no changes for already clean schema")
	require.True(t, reflect.DeepEqual(normalized, copy), "schema should remain unchanged")
}
