package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// Reusable metadata fragments. Fireworks publishes per-model cards at
// https://fireworks.ai/models/<provider>/<slug> that report context length,
// HuggingFace lineage, calibration (FP8 quantization), and function calling
// support. The values below were retrieved 2026-04-21 .. 2026-05-01 from the
// model-card pages and standardized to the OpenRouter-compatible vocabulary
// expected by adaptor.ModelConfig.
//
// Sources:
//   - https://fireworks.ai/models
//   - https://fireworks.ai/pricing
//   - Per-model cards under https://fireworks.ai/models/<provider>/<slug>
var (
	// fwTextOnlyModalities advertises a chat model that consumes and emits text only.
	fwTextOnlyModalities = []string{"text"}
	// fwTextImageInModalities advertises text+image input with text output.
	fwTextImageInModalities = []string{"text", "image"}

	// fwChatSamplingParams enumerates the OpenAI-compatible sampling parameters
	// Fireworks accepts for typical chat completion endpoints. Source:
	// https://docs.fireworks.ai/api-reference/post-chatcompletions
	fwChatSamplingParams = []string{
		"temperature",
		"top_p",
		"top_k",
		"frequency_penalty",
		"presence_penalty",
		"repetition_penalty",
		"stop",
		"seed",
		"max_tokens",
		"logprobs",
		"top_logprobs",
		"response_format",
		"tools",
		"tool_choice",
		"n",
	}

	// fwReasoningSamplingParams is the restricted set Fireworks recommends
	// (and, for some models, enforces) on reasoning-style endpoints such as
	// DeepSeek-R1 and Qwen3 thinking variants. Source:
	// https://docs.fireworks.ai/guides/reasoning
	fwReasoningSamplingParams = []string{
		"max_tokens",
		"seed",
		"stop",
	}

	// fwChatFeatures lists capabilities Fireworks advertises for general chat
	// models — tool calling, JSON mode, and structured outputs are universally
	// available on the Fireworks chat completions endpoint.
	fwChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// fwReasoningFeatures adds the "reasoning" capability for thinking models.
	fwReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// fwEmbedSamplingParams is the parameter set the Fireworks embedding API
	// recognizes (input text only). Source:
	// https://docs.fireworks.ai/api-reference/post-embeddings
	fwEmbedSamplingParams = []string{"input", "encoding_format", "dimensions"}

	// fwRerankSamplingParams covers the rerank endpoint's accepted fields.
	// Source: https://docs.fireworks.ai/api-reference/post-rerank
	fwRerankSamplingParams = []string{"query", "documents", "top_n", "return_documents"}
)

// ModelRatios contains Fireworks serverless models with their per-token pricing
// and capability metadata. Family-specific maps are defined in dedicated files
// (models_deepseek.go, models_glm.go, etc.) and merged here at package init.
//
// Fireworks model IDs always use the "accounts/fireworks/models/<slug>" resource name.
// Pricing reference: https://fireworks.ai/pricing#serverless-pricing (retrieved 2026-04-28).
//
// Pricing buckets:
//   - <4B dense: $0.10/1M flat
//   - 4B-16B dense: $0.20/1M flat
//   - >16B dense: $0.90/1M flat
//   - MoE 0-56B: $0.50/1M flat
//   - MoE 56.1-176B: $1.20/1M flat
//
// Popular flagship models use per-model pricing listed on each family page.
//
// Capability metadata (context length, modalities, HuggingFaceID, quantization)
// is sourced from the per-model Fireworks cards. Fireworks typically serves
// open-weight checkpoints as FP16/BF16; "Calibrated: Yes" cards run in FP8.
var ModelRatios = mergeModelMaps(
	deepseekModels,
	glmModels,
	kimiModels,
	gptOssModels,
	qwenModels,
	minimaxModels,
	llamaModels,
	mistralModels,
	rerankModels,
	embeddingModels,
)

// mergeModelMaps combines per-family model maps into a single map. Duplicate
// keys are overwritten by later maps; family files must keep keys disjoint.
func mergeModelMaps(maps ...map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	merged := make(map[string]adaptor.ModelConfig, total)
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}

// FireworksToolingDefaults records that Fireworks does not publish provider-level
// built-in tool pricing (retrieved 2026-04-21).
var FireworksToolingDefaults = adaptor.ChannelToolConfig{}
