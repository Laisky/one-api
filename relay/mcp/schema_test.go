package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestArgumentsMatchSchema_ObjectRequired verifies required fields are enforced.
func TestArgumentsMatchSchema_ObjectRequired(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string"},
		},
		"required": []any{"url"},
	}

	match, err := ArgumentsMatchSchema(map[string]any{"url": "https://example.com"}, schema)
	require.NoError(t, err)
	require.True(t, match)

	match, err = ArgumentsMatchSchema(map[string]any{"query": "missing"}, schema)
	require.NoError(t, err)
	require.False(t, match)
}

// TestArgumentsMatchSchema_TypeMismatch verifies type validation for properties.
func TestArgumentsMatchSchema_TypeMismatch(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
		"required": []any{"count"},
	}

	match, err := ArgumentsMatchSchema(map[string]any{"count": 3.0}, schema)
	require.NoError(t, err)
	require.True(t, match)

	match, err = ArgumentsMatchSchema(map[string]any{"count": "three"}, schema)
	require.NoError(t, err)
	require.False(t, match)
}
