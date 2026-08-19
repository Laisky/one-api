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
	require.Contains(t, ModelRatios, "anthropic/claude-sonnet-5")
	require.Contains(t, ModelRatios, "thinkingmachines/Inkling")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-VL-235B-A22B-Instruct")
	require.Contains(t, ModelRatios, "BAAI/bge-m3")
	require.Contains(t, ModelRatios, "nvidia/llama-nemotron-embed-vl-1b-v2")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-Reranker-8B")
	require.Contains(t, ModelRatios, "openai/whisper-large-v3-turbo")
	require.Contains(t, ModelRatios, "Qwen/Qwen3-TTS")
	require.Contains(t, ModelRatios, "XiaomiMiMo/MiMo-V2.5-tts-voiceclone")
	require.Contains(t, ModelRatios, "black-forest-labs/FLUX-2-klein-4b")
	require.Contains(t, ModelRatios, "ByteDance/Seedream-4.5")

	// These live catalog entries require relay or billing contracts that one-api
	// cannot represent exactly yet, so they must not be advertised as supported.
	require.NotContains(t, ModelRatios, "nvidia/llama-nemotron-rerank-vl-1b-v2")
	require.NotContains(t, ModelRatios, "Qwen/Qwen-Image-Edit")
	require.NotContains(t, ModelRatios, "google/nano-banana-pro")
}

// TestModelPricing verifies token, cache, embedding, audio, and image pricing metadata.
func TestModelPricing(t *testing.T) {
	t.Parallel()

	flash := ModelRatios["deepseek-ai/DeepSeek-V4-Flash-0731"]
	require.InDelta(t, 0.08*billingratio.MilliTokensUsd, flash.Ratio, 1e-12)
	require.InDelta(t, 0.18/0.08, flash.CompletionRatio, 1e-12)
	require.InDelta(t, 0.016*billingratio.MilliTokensUsd, flash.CachedInputRatio, 1e-12)

	claude := ModelRatios["anthropic/claude-sonnet-5"]
	require.InDelta(t, 2.0*billingratio.MilliTokensUsd, claude.Ratio, 1e-12)
	require.InDelta(t, 5.0, claude.CompletionRatio, 1e-12)
	require.Equal(t, []string{"text", "image"}, claude.InputModalities)

	inkling := ModelRatios["thinkingmachines/Inkling"]
	require.Equal(t, []string{"text", "image", "audio"}, inkling.InputModalities)
	require.InDelta(t, 0.16*billingratio.MilliTokensUsd, inkling.CachedInputRatio, 1e-12)

	embedding := ModelRatios["BAAI/bge-m3"]
	require.NotNil(t, embedding.Embedding)
	require.InDelta(t, embedding.Ratio, embedding.Embedding.TextTokenRatio, 1e-12)

	multimodalEmbedding := ModelRatios["nvidia/llama-nemotron-embed-vl-1b-v2"]
	require.NotNil(t, multimodalEmbedding.Embedding)
	require.Equal(t, []string{"text", "image"}, multimodalEmbedding.InputModalities)

	reranker := ModelRatios["Qwen/Qwen3-Reranker-4B"]
	require.InDelta(t, 0.025*billingratio.MilliTokensUsd, reranker.Ratio, 1e-12)

	asr := ModelRatios["openai/whisper-large-v3-turbo"]
	require.NotNil(t, asr.Audio)
	require.InDelta(t, 10.0, asr.Audio.PromptTokensPerSecond, 1e-12)
	require.InDelta(t, 0.00020*float64(billingratio.QuotaPerUsd)/600.0, asr.Ratio, 1e-12)

	freeTTS := ModelRatios["XiaomiMiMo/MiMo-V2.5-tts"]
	require.Zero(t, freeTTS.Ratio)
	require.Equal(t, []string{"audio"}, freeTTS.OutputModalities)

	areaPricedImage := ModelRatios["black-forest-labs/FLUX-2-klein-4b"]
	require.NotNil(t, areaPricedImage.Image)
	require.InDelta(t, 0.014, areaPricedImage.Image.PricePerImageUsd, 1e-12)
	require.NotEmpty(t, areaPricedImage.Image.SizeMultipliers)

	flatImage := ModelRatios["ByteDance/Seedream-4.5"]
	require.NotNil(t, flatImage.Image)
	require.InDelta(t, 0.04, flatImage.Image.PricePerImageUsd, 1e-12)
	require.Empty(t, flatImage.Image.SizeMultipliers)
	require.Equal(t, []string{"text", "image"}, flatImage.InputModalities)
}

// TestGetModelListIsSorted verifies deterministic ordering and guards the catalog snapshot size.
func TestGetModelListIsSorted(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	require.Len(t, models, 184)
	require.True(t, sort.StringsAreSorted(models))
}
