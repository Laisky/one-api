package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeClaudeToolChoice(t *testing.T) {
	t.Parallel()

	t.Run("converts tool choice type", func(t *testing.T) {
		original := map[string]any{
			"type": "tool",
			"name": "get_weather",
		}

		converted := normalizeClaudeToolChoice(original)

		choiceMap, ok := converted.(map[string]any)
		require.True(t, ok, "expected map[string]any, got %T", converted)

		assert.Equal(t, "function", choiceMap["type"], "type should be rewritten to function")
		functionPayload, ok := choiceMap["function"].(map[string]any)
		assert.True(t, ok, "function payload should exist")
		assert.Equal(t, "get_weather", functionPayload["name"], "function name should be preserved")
		_, hasName := choiceMap["name"]
		assert.False(t, hasName, "top-level name should be removed after conversion")
	})

	t.Run("preserves existing function payload", func(t *testing.T) {
		original := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "already_normalized",
			},
			"name": "ignored",
		}

		converted := normalizeClaudeToolChoice(original)
		choiceMap := converted.(map[string]any)
		functionPayload := choiceMap["function"].(map[string]any)

		assert.Equal(t, "function", choiceMap["type"])
		assert.Equal(t, "already_normalized", functionPayload["name"])
		_, hasName := choiceMap["name"]
		assert.False(t, hasName)
	})

	t.Run("passes through non map types", func(t *testing.T) {
		assert.Equal(t, "auto", normalizeClaudeToolChoice("auto"))
	})
}
