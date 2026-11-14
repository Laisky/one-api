package vertexai

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVertexCatalogLifecycle verifies suggestions independently of legacy routing.
// Parameters: t owns assertions. Returns: none.
func TestVertexCatalogLifecycle(t *testing.T) {
	t.Parallel()
	models := (&Adaptor{}).GetModelList()
	require.Contains(t, models, "gemini-3.8-flash", "positive current-model control")
	for _, id := range []string{
		"gemini-2.0-flash", "gemini-2.5-pro", "claude-3-haiku@20240307",
		"claude-3-7-sonnet@20250219", "veo-2.0-generate-001",
		"veo-3.1-generate-preview", "openai/gpt-oss-20b-maas",
		"gemini-omni-1.1-flash", "gemini-embedding-001",
		"deepseek-ai/deepseek-v3.2-maas", "qwen/qwen3-next-80b-a3b-instruct-maas",
	} {
		require.NotContains(t, models, id, "retired or unqualified models must not be suggested")
		require.NotNil(t, GetAdaptor(id), "removing suggestions must preserve existing routes")
	}
	require.True(t, slices.IsSorted(models))
	require.Equal(t, len(models), len(slices.Compact(slices.Clone(models))))
}

// TestVertexCatalogResultIsolation verifies callers cannot mutate later catalogs.
// Parameters: t owns assertions. Returns: none.
func TestVertexCatalogResultIsolation(t *testing.T) {
	t.Parallel()
	first := (&Adaptor{}).GetModelList()
	require.NotEmpty(t, first)
	original := first[0]
	first[0] = "caller-mutated"
	require.Contains(t, (&Adaptor{}).GetModelList(), original)
	require.NotContains(t, (&Adaptor{}).GetModelList(), "caller-mutated")
}

// TestVertexCatalogCurrentRoutes verifies current suggestions use registered paths.
// Parameters: t owns assertions. Returns: none.
func TestVertexCatalogCurrentRoutes(t *testing.T) {
	t.Parallel()
	models := (&Adaptor{}).GetModelList()
	for _, id := range []string{
		"gemini-4-hydrogen", "gemini-4-carbon", "gemini-4-carbon-cyber-permissive",
		"gemini-4-argon-cyber-permissive", "gemini-3.8-flash-cyber",
		"gemini-nano-banana-2.1", "claude-fable-5-2", "claude-haiku-5-5",
		"openai/gpt-oss-120b-maas", "deepseek-ai/deepseek-v4.1-flash-maas",
		"zai-org/glm-5.2-maas", "google/gemma-4-26b-a4b-it-maas",
		"meta/llama-4-maverick-17b-128e-instruct-maas",
		"meta/llama-4-scout-17b-16e-instruct-maas",
	} {
		require.Contains(t, models, id)
		require.NotNil(t, GetAdaptor(id), "current model %s must retain an actual route", id)
	}
}
