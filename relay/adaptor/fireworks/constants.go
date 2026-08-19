package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// Reusable metadata fragments. Fireworks publishes per-model cards at
// https://app.fireworks.ai/models/<provider>/<slug> that report serverless
// availability, pricing, context length, Hugging Face lineage, calibration,
// modalities, and function-calling support. The values below were verified
// against the live model library and model cards on 2026-08-18 and standardized
// to the OpenRouter-compatible vocabulary expected by adaptor.ModelConfig.
//
// Sources:
//   - https://app.fireworks.ai/models
//   - https://fireworks.ai/pricing
//   - Per-model cards under https://app.fireworks.ai/models/<provider>/<slug>
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

	// fwChatFeatures lists the common capabilities advertised by Fireworks
	// models that support tool calling. Models with narrower model-card support
	// override this list instead of inheriting unsupported features.
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

// ModelRatios contains curated Fireworks catalog models with their per-token
// pricing and capability metadata. Family-specific maps are defined in
// dedicated files and merged here at package init.
//
// Current serverless catalog entries use the
// "accounts/fireworks/models/<slug>" resource name. Legacy dedicated-only
// embedding entries retain the model IDs accepted by their older endpoints.
//
// Per-model serverless prices and availability are sourced from the live model
// library and individual cards (verified 2026-08-18). Generic dedicated
// deployment buckets remain documented at https://fireworks.ai/pricing.
//
// Capability metadata (context length, modalities, Hugging Face lineage, and
// quantization where explicitly published) is sourced from the model cards and
// upstream model repositories.
var ModelRatios = mergeModelMaps(
	deepseekModels,
	glmModels,
	kimiModels,
	gptOssModels,
	qwenModels,
	minimaxModels,
	nvidiaModels,
	frontierModels,
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
// built-in tool pricing (retrieved 2026-08-18).
var FireworksToolingDefaults = adaptor.ChannelToolConfig{}
