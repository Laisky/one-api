package deepinfra

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// TestModelCatalogCoversSupportedModalities verifies representative models for each relay family.
func TestModelCatalogCoversSupportedModalities(t *testing.T) {
	t.Parallel()

	require.Contains(t, ModelRatios, "deepseek-ai/DeepSeek-V4-Flash")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-VL-235B-A22B-Instruct")
	require.Contains(t, ModelRatios, "BAAI/bge-m3")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-Embedding-8B")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-Reranker-8B")
	require.Contains(t, ModelRatios, "openai/whisper-large-v3-turbo")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-TTS")
	require.Contains(t, ModelRatios, "black-forest-labs/FLUX-2-klein-4b")
}

// TestModelPricing verifies token, cache, embedding, and image pricing metadata.
func TestModelPricing(t *testing.T) {
	t.Parallel()

	flash := ModelRatios["deepseek-ai/DeepSeek-V4-Flash-0731"]
	require.InDelta(t, 0.08*billingratio.MilliTokensUsd, flash.Ratio, 1e-12)
	require.InDelta(t, 0.18/0.08, flash.CompletionRatio, 1e-12)
	require.InDelta(t, 0.016*billingratio.MilliTokensUsd, flash.CachedInputRatio, 1e-12)

	embedding := ModelRatios["BAAI/bge-m3"]
	require.NotNil(t, embedding.Embedding)
	require.InDelta(t, embedding.Ratio, embedding.Embedding.TextTokenRatio, 1e-12)

	reranker := ModelRatios["Qwen/Qwen3-Reranker-4B"]
	require.InDelta(t, 0.025*billingratio.MilliTokensUsd, reranker.Ratio, 1e-12)

	asr := ModelRatios["openai/whisper-large-v3-turbo"]
	require.NotNil(t, asr.Audio)
	require.InDelta(t, 10.0, asr.Audio.PromptTokensPerSecond, 1e-12)
	require.InDelta(t, 0.00020*float64(billingratio.QuotaPerUsd)/600.0, asr.Ratio, 1e-12)

	image := ModelRatios["black-forest-labs/FLUX-2-klein-4b"]
	require.NotNil(t, image.Image)
	require.InDelta(t, 0.014, image.Image.PricePerImageUsd, 1e-12)
}

// TestGetModelListIsSorted verifies deterministic frontend and API model ordering.
func TestGetModelListIsSorted(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	require.Greater(t, len(models), 70)
	require.True(t, sort.StringsAreSorted(models))
}
