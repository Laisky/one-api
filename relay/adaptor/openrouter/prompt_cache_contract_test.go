package openrouter_test

import (
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/stretchr/testify/require"
)

// TestImplicitPromptCacheWrites scopes automatic funding to published host
// contracts, including normalized IDs and mutable family aliases. It accepts t
// and returns no values; older, unknown and other-provider models stay excluded.
func TestImplicitPromptCacheWrites(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"openai/gpt-6.2-astra", "openai/gpt-6.2-astra-pro",
		"openai/gpt-6.2-sol", "openai/gpt-6.2-sol-pro",
		" OPENAI/GPT-6.2-SOL ", "openai/gpt-5.6-luna",
		"openai/gpt-5.6-sol-pro", "openai/gpt-5.6-terra",
		"openai/gpt-6-astra", "openai/gpt-6.1-terra-pro",
		"~openai/gpt-astra-latest", "~openai/gpt-sol-latest",
		"~openai/gpt-terra-latest", "~openai/gpt-luna-latest",
	} {
		require.True(t, openrouter.ImplicitPromptCacheWrites(name), name)
	}
	for _, name := range []string{
		"", "gpt-6.2-astra", "openai/gpt-6.2-unknown",
		"openai/gpt-6.2-astra:batch", "openai/gpt-6.2-astra-2026-11-17",
		"openai/gpt-5.5", "openai/gpt-4o", "~openai/gpt-mini-latest",
		"~openai/gpt-latest", "~openai/gpt-unknown-latest",
		"anthropic/claude-fable-5.2", "google/gemini-4-neon",
		"deepseek/deepseek-v4.1-flash-1017", "qwen/qwen3-coder-plus",
	} {
		require.False(t, openrouter.ImplicitPromptCacheWrites(name), name)
	}
}
