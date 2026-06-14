package nvidia

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// Reusable modality / feature / sampling fragments for NVIDIA-hosted models.
//
// Sources (retrieved 2026-06-14):
//   - https://build.nvidia.com/models                 (catalog grid; per-card type + "Free Endpoint" badge)
//   - https://build.nvidia.com/nvidia                 (NVIDIA's own nvidia/* namespace)
//   - https://docs.api.nvidia.com/nim/reference/llm-apis (OpenAI-compatible LLM API reference)
//   - https://assets.ngc.nvidia.com/products/api-catalog/featured-models.json (canonical dotted API ids)
var (
	// textInputs advertises a chat model that consumes and emits text only.
	textInputs = []string{"text"}
	// visionInputs advertises text+image input with text output (VL / vision models).
	visionInputs = []string{"text", "image"}
	// omniInputs advertises text+image+audio+video input with text output (omni models).
	omniInputs = []string{"text", "image", "audio", "video"}
	// textOutputs advertises text-only output (every chat/VL model below emits text).
	textOutputs = []string{"text"}

	// chatFeatures is the standard non-thinking capability set. NVIDIA's
	// OpenAI-compatible chat completions endpoint exposes tools and JSON mode.
	chatFeatures = []string{"tools", "json_mode"}
	// reasoningFeatures adds the reasoning capability for thinking-mode models
	// (Nemotron reasoning line, DeepSeek V4, gpt-oss, etc.).
	reasoningFeatures = []string{"tools", "json_mode", "reasoning"}

	// samplingParams enumerates the OpenAI-compatible sampling parameters NVIDIA
	// accepts on chat completions. NVIDIA's API reference mirrors the OpenAI
	// chat schema; reasoning controls are passed via extra-body fields rather
	// than these parameters.
	samplingParams = []string{
		"temperature",
		"top_p",
		"max_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"seed",
		"n",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"response_format",
		"tools",
		"tool_choice",
	}
)

// ModelRatios contains the NVIDIA API Catalog models exposed by this adaptor.
//
// PRICING NOTE — every model is registered FREE (Ratio: 0, CompletionRatio: 1):
//
//	NVIDIA does NOT publish per-token dollar pricing for the hosted
//	integrate.api.nvidia.com endpoint. Access is a trial experience metered in
//	"API credits" (1000 granted on sign-up, up to 5000), not currency, and every
//	model card below carries a "Free Endpoint" badge. There is therefore no
//	authoritative per-1M-token rate to encode, so we follow the project rule of
//	never fabricating prices and keep all ratios at 0 (the same idiom used for
//	Tencent hunyuan-lite). Operators routing to NVIDIA AI Enterprise or a paid
//	"Partner Endpoint" with real costs can set per-channel pricing overrides.
//
// CONTEXT LENGTH NOTE — ContextLength is populated only where a value is
// well-established (stable Llama 3.x / Mixtral / Gemma) or was explicitly stated
// on the catalog card (the "1M context" models). Unspecified entries are left at
// 0 so the display layer applies its default rather than inviting a fabricated
// number.
//
// Model IDs are the slash-namespaced dotted strings expected by the API `model`
// field (NOT the underscore-substituted catalog URL slugs).
// Source: https://build.nvidia.com/models (retrieved 2026-06-14)
var ModelRatios = map[string]adaptor.ModelConfig{
	// ---- NVIDIA Nemotron — text reasoning / chat (nvidia/*, Free Endpoint) ----
	"nvidia/nemotron-3-ultra-550b-a55b": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               1000000,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Nemotron 3 Ultra: flagship hybrid Mamba-Transformer MoE with a 1M context window and reasoning. Free hosted endpoint.",
	},
	"nvidia/nemotron-3-super-120b-a12b": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Nemotron 3 Super: high-throughput reasoning MoE. Free hosted endpoint.",
	},
	"nvidia/nemotron-3-nano-30b-a3b": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               1000000,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Nemotron 3 Nano: efficient reasoning MoE with a 1M context window. Free hosted endpoint.",
	},
	"nvidia/nemotron-mini-4b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Nemotron Mini 4B: small instruction-tuned chat model with function calling. Free hosted endpoint.",
	},
	"nvidia/llama-3.3-nemotron-super-49b-v1.5": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Llama 3.3 Nemotron Super 49B v1.5: Llama-derived reasoning model. Free hosted endpoint.",
	},
	"nvidia/llama-3.3-nemotron-super-49b-v1": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Llama 3.3 Nemotron Super 49B v1: Llama-derived reasoning model. Free hosted endpoint.",
	},
	"nvidia/llama-3.1-nemotron-nano-8b-v1": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Llama 3.1 Nemotron Nano 8B: compact reasoning model. Free hosted endpoint.",
	},

	// ---- NVIDIA Nemotron — vision / multimodal (nvidia/*, Free Endpoint) ----
	"nvidia/nemotron-nano-12b-v2-vl": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               262144,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Nemotron Nano 12B v2 VL: vision-language model for multi-image and video understanding. Free hosted endpoint.",
	},
	"nvidia/llama-3.1-nemotron-nano-vl-8b-v1": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Llama 3.1 Nemotron Nano VL 8B: compact vision-language model. Free hosted endpoint.",
	},
	"nvidia/nemotron-3-nano-omni-30b-a3b-reasoning": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             omniInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "NVIDIA Nemotron 3 Nano Omni: omni-modal (image/video/speech/text) reasoning model. Free hosted endpoint.",
	},

	// ---- Meta Llama (meta/*, Free Endpoint) ----
	"meta/llama-4-maverick-17b-128e-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               1000000,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 4 Maverick: multimodal MoE instruct model. Free hosted endpoint.",
	},
	"meta/llama-3.3-70b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.3 70B Instruct. Free hosted endpoint.",
	},
	"meta/llama-3.1-70b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.1 70B Instruct. Free hosted endpoint.",
	},
	"meta/llama-3.1-8b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.1 8B Instruct. Free hosted endpoint.",
	},
	"meta/llama-3.2-3b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.2 3B Instruct. Free hosted endpoint.",
	},
	"meta/llama-3.2-1b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.2 1B Instruct. Free hosted endpoint.",
	},
	"meta/llama-3.2-11b-vision-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.2 11B Vision Instruct. Free hosted endpoint.",
	},
	"meta/llama-3.2-90b-vision-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Meta Llama 3.2 90B Vision Instruct. Free hosted endpoint.",
	},

	// ---- DeepSeek (deepseek-ai/*, Free Endpoint) ----
	"deepseek-ai/deepseek-v4-flash": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               1000000,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "DeepSeek V4 Flash: MoE reasoning/coding model with a 1M context window. Free hosted endpoint.",
	},
	"deepseek-ai/deepseek-v4-pro": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               1000000,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "DeepSeek V4 Pro: large MoE reasoning/coding model with a 1M context window. Free hosted endpoint.",
	},

	// ---- Qwen (qwen/*, Free Endpoint) ----
	"qwen/qwen3.5-397b-a17b": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Qwen3.5 397B A17B: large vision-language MoE for chat, RAG, and agents. Free hosted endpoint.",
	},
	"qwen/qwen3.5-122b-a10b": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Qwen3.5 122B A10B: multimodal MoE chat model. Free hosted endpoint.",
	},
	"qwen/qwen3-next-80b-a3b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Qwen3-Next 80B A3B Instruct: long-context chat model. Free hosted endpoint.",
	},

	// ---- Mistral (mistralai/*, Free Endpoint) ----
	"mistralai/mistral-medium-3.5-128b": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Mistral Medium 3.5: general-purpose chat model. Free hosted endpoint.",
	},
	"mistralai/mistral-small-4-119b-2603": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               262144,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Mistral Small 4: hybrid MoE multimodal chat model. Free hosted endpoint.",
	},
	"mistralai/mistral-large-3-675b-instruct-2512": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Mistral Large 3: large vision-language chat/agent model. Free hosted endpoint.",
	},
	"mistralai/mistral-nemotron": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Mistral Nemotron: NVIDIA-tuned agentic chat model with function calling. Free hosted endpoint.",
	},
	"mistralai/mixtral-8x7b-instruct": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               32768,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Mistral Mixtral 8x7B Instruct (v0.1): sparse MoE chat model. Free hosted endpoint.",
	},

	// ---- OpenAI gpt-oss (openai/*, Free Endpoint) ----
	"openai/gpt-oss-120b": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "OpenAI gpt-oss 120B: open-weight MoE reasoning model (text only). Free hosted endpoint.",
	},
	"openai/gpt-oss-20b": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               131072,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "OpenAI gpt-oss 20B: open-weight MoE reasoning model (text only). Free hosted endpoint.",
	},

	// ---- Google Gemma (google/*, Free Endpoint) ----
	"google/gemma-4-31b-it": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Google Gemma 4 31B IT: dense instruction-tuned reasoning model. Free hosted endpoint.",
	},
	"google/gemma-2-2b-it": {
		Ratio: 0, CompletionRatio: 1,
		ContextLength:               8192,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Google Gemma 2 2B IT: lightweight instruction-tuned chat model. Free hosted endpoint.",
	},

	// ---- Microsoft Phi (microsoft/*, Free Endpoint) ----
	"microsoft/phi-4-mini-instruct": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Microsoft Phi-4 Mini Instruct: compact chat model. Free hosted endpoint.",
	},
	"microsoft/phi-4-multimodal-instruct": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           chatFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Microsoft Phi-4 Multimodal Instruct: image+audio multimodal chat model. Free hosted endpoint.",
	},

	// ---- Other popular hosted chat models (Free Endpoint) ----
	"moonshotai/kimi-k2.6": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Moonshot Kimi K2.6: multimodal MoE chat/agent model. Free hosted endpoint.",
	},
	"z-ai/glm-5.1": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "Z.ai GLM-5.1: general-purpose reasoning chat model. Free hosted endpoint.",
	},
	"minimaxai/minimax-m3": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "MiniMax M3: multimodal MoE vision-language model. Free hosted endpoint.",
	},
	"minimaxai/minimax-m2.7": {
		Ratio: 0, CompletionRatio: 1,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		Description:                 "MiniMax M2.7: MoE chat/agent model. Free hosted endpoint.",
	},
}

// ModelList is derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// NvidiaToolingDefaults declares no built-in tool metering: NVIDIA does not
// publish per-tool pricing for the hosted API (retrieved 2026-06-14).
var NvidiaToolingDefaults = adaptor.ChannelToolConfig{}
