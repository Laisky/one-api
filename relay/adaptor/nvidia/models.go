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
var ModelRatios = adaptor.JoinModelCatalogs(buildModelRatios())

// buildModelRatios returns the complete NVIDIA-hosted model map. It accepts no
// parameters and preserves existing model fields and compatibility metadata.
// Hosted specifications, trial terms, and native schemas were checked on
// 2026-10-12 and 2026-11-16 alongside the following model cards:
// https://build.nvidia.com/z-ai/glm-5-3
// https://build.nvidia.com/z-ai/glm-5-3/modelcard
// https://build.nvidia.com/z-ai/glm-5-3-flash
// https://build.nvidia.com/z-ai/glm-5-3-flash/modelcard
// https://build.nvidia.com/mistralai/mistral-large-4-1t-a52b
// https://build.nvidia.com/nvidia/nemotron-3.5-bolt-e4b
// https://build.nvidia.com/minimaxai/minimax-m3.1-flash
// https://build.nvidia.com/deepseek-ai/deepseek-v4.1-flash
// https://build.nvidia.com/nvidia/nemotron-3.5-super-vl-120b-a12b
// https://build.nvidia.com/nvidia/nemotron-3.5-lightning-30b-a3b
// https://build.nvidia.com/stepfun-ai/step-5-preview
// https://build.nvidia.com/moonshotai/kimi-k3_1-pro
// https://build.nvidia.com/meta/muse-glimmer-30b
func buildModelRatios() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{

		// deepseek-ai models.
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
		"deepseek-ai/deepseek-v4.1-flash": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               1_048_576,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Description:                 "NVIDIA-hosted DeepSeek V4.1 Flash with text/image input and a 1048576-token context. Numeric checkpoint reasoning controls are not translated into an undocumented string reasoning_effort interface." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},

		// google models.
		"google/gemma-2-2b-it": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               8192,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Google Gemma 2 2B IT: lightweight instruction-tuned chat model. Free hosted endpoint.",
		},
		"google/gemma-4-31b-it": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Google Gemma 4 31B IT: dense instruction-tuned reasoning model. Free hosted endpoint.",
		},

		// meta models.
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
		"meta/llama-3.2-11b-vision-instruct": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               131072,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Meta Llama 3.2 11B Vision Instruct. Free hosted endpoint.",
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
		"meta/llama-3.2-3b-instruct": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               131072,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Meta Llama 3.2 3B Instruct. Free hosted endpoint.",
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
		"meta/llama-3.3-70b-instruct": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               131072,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Meta Llama 3.3 70B Instruct. Free hosted endpoint.",
		},
		"meta/llama-4-maverick-17b-128e-instruct": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               1000000,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Meta Llama 4 Maverick: multimodal MoE instruct model. Free hosted endpoint.",
		},
		"meta/muse-glimmer-30b": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               131_072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Description:                 "NVIDIA-hosted Muse Glimmer 30B with text/image input and 131072-token hosted context. Tools and reasoning are available; structured output is explicitly unsupported. The embedded effort schema refers to a different model, so no effort vocabulary or default is inferred from that inconsistent evidence." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},

		// microsoft models.
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
			Description:                 "Microsoft Phi-4 Multimodal Instruct: image+audio multimodal chat model. Free hosted endpoint. DEPRECATED: NVIDIA states this hosted API will be deprecated 2026-07-15 and no longer supported afterward.",
		},

		// minimaxai models.
		"minimaxai/minimax-m2.7": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "MiniMax M2.7: MoE chat/agent model. Free hosted endpoint.",
		},
		"minimaxai/minimax-m3": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               196608,
			MaxOutputTokens:             8192,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "MiniMax M3: multimodal MoE vision-language model. Free hosted endpoint.",
		},
		"minimaxai/minimax-m3.1-flash": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               262_144,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Quantization:                "fp4",
			Description:                 "NVIDIA-hosted MiniMax M3.1 Flash NVFP4. Hosted specifications publish 262144 context; the checkpoint supports up to 1M depending on serving configuration. Text and image chat are advertised here; native video and chat-template reasoning controls require their own payload contracts." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},

		// mistralai models.
		"mistralai/mistral-large-3-675b-instruct-2512": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Mistral Large 3: large vision-language chat/agent model. Free hosted endpoint.",
		},
		"mistralai/mistral-large-4-1t-a52b": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               262_144,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "reasoning_effort"},
			SupportedReasoningEfforts:   []string{"none", "high"},
			DefaultReasoningEffort:      "high",
			Description:                 "NVIDIA-hosted Mistral Large 4. Hosted specifications publish 262144 context, while the checkpoint card describes 1M. The hosted API schema still exposes none/high reasoning, default high; it does not inherit the first-party Mistral effort vocabulary." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},
		"mistralai/mistral-medium-3.5-128b": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Mistral Medium 3.5: general-purpose chat model. Free hosted endpoint.",
		},
		"mistralai/mistral-nemotron": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Mistral Nemotron: NVIDIA-tuned agentic chat model with function calling. Free hosted endpoint.",
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
		"mistralai/mixtral-8x7b-instruct": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               32768,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Mistral Mixtral 8x7B Instruct (v0.1): sparse MoE chat model. Free hosted endpoint.",
		},

		// moonshotai models.
		"moonshotai/kimi-k2.6": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Moonshot Kimi K2.6: multimodal MoE chat/agent model. Free hosted endpoint.",
		},
		"moonshotai/kimi-k3.1-pro": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               1_048_576,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "reasoning_effort"},
			SupportedReasoningEfforts:   []string{"low", "medium", "high", "max"},
			DefaultReasoningEffort:      "low",
			Description:                 "NVIDIA-hosted Kimi K3.1 Pro, exact dotted API ID despite the underscore in the catalog URL. Hosted text/image input, 1048576-token context and low/medium/high/max effort, default low. A separate maximum output ceiling is undisclosed." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},

		// nvidia models.
		"nvidia/llama-3.1-nemotron-nano-8b-v1": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               131072,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Llama 3.1 Nemotron Nano 8B: compact reasoning model. Free hosted endpoint.",
		},
		"nvidia/llama-3.1-nemotron-nano-vl-8b-v1": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Llama 3.1 Nemotron Nano VL 8B: compact vision-language model. Free hosted endpoint.",
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
		"nvidia/llama-3.3-nemotron-super-49b-v1.5": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               131072,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Llama 3.3 Nemotron Super 49B v1.5: Llama-derived reasoning model. Free hosted endpoint.",
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
		"nvidia/nemotron-3-nano-omni-30b-a3b-reasoning": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             omniInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Nemotron 3 Nano Omni: omni-modal (image/video/speech/text) reasoning model. Free hosted endpoint.",
		},
		"nvidia/nemotron-3-super-120b-a12b": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               1000000,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Nemotron 3 Super: high-throughput reasoning MoE. Free hosted endpoint.",
		},
		"nvidia/nemotron-3-ultra-550b-a55b": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               1000000,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Nemotron 3 Ultra: flagship hybrid Mamba-Transformer MoE with a 1M context window and reasoning. Free hosted endpoint.",
		},
		"nvidia/nemotron-3.5-bolt-e4b": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               65_536,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Quantization:                "bf16",
			Description:                 "NVIDIA Nemotron 3.5 Bolt E4B BF16, text-only hosted 64K context. Native reasoning is controlled by chat_template_kwargs.enable_thinking; no unsupported string effort vocabulary is advertised." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},
		"nvidia/nemotron-3.5-lightning-30b-a3b": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               1_048_576,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Quantization:                "fp4",
			Description:                 "NVIDIA Nemotron 3.5 Lightning NVFP4 with text-only 1048576-token context. Native enable_thinking and numeric reasoning_budget controls are distinct from string reasoning_effort; no effort vocabulary is inferred." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},
		"nvidia/nemotron-3.5-super-vl-120b-a12b": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               262_144,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "reasoning_effort"},
			SupportedReasoningEfforts:   []string{"none", "low", "high"},
			DefaultReasoningEffort:      "high",
			Quantization:                "fp4",
			Description:                 "NVIDIA Nemotron 3.5 Super VL NVFP4. Hosted specifications publish 262144 context rather than the checkpoint's potential 1M. Native reasoning_effort supports none/low/high, default high. Text/image chat is advertised; native video transport is not inferred." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},
		"nvidia/nemotron-mini-4b-instruct": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Nemotron Mini 4B: small instruction-tuned chat model with function calling. Free hosted endpoint.",
		},
		"nvidia/nemotron-nano-12b-v2-vl": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength:               262144,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Nemotron Nano 12B v2 VL: vision-language model for multi-image and video understanding. Free hosted endpoint.",
		},
		"nvidia/nvidia-nemotron-nano-9b-v2": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "NVIDIA Nemotron Nano 9B v2: high-efficiency hybrid Transformer-Mamba model excelling in reasoning and agentic tasks, with a configurable thinking budget. Free hosted endpoint.",
		},

		// openai models.
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

		// qwen models.
		"qwen/qwen3-next-80b-a3b-instruct": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           chatFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Qwen3-Next 80B A3B Instruct: long-context chat model. Free hosted endpoint.",
		},
		"qwen/qwen3.5-122b-a10b": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Qwen3.5 122B A10B: multimodal MoE chat model. Free hosted endpoint.",
		},
		"qwen/qwen3.5-397b-a17b": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             visionInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Qwen3.5 397B A17B: large vision-language MoE for chat, RAG, and agents. Free hosted endpoint.",
		},

		// stepfun-ai models.
		"stepfun-ai/step-5-preview": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               1_048_576,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Quantization:                "fp4",
			Description:                 "NVIDIA-hosted Step 5 Preview with NVFP4 experts, text/image input and 1048576-token context. Reasoning is always enabled in this endpoint; the hosted schema does not support disabling thinking. Structured output and an exact hosted output ceiling are not advertised." + " Free trial endpoint only; paid partner or private deployment tariffs require operator configuration. No separate maximum output limit is assumed.",
		},

		// z-ai models.
		"z-ai/glm-5.1": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Z.ai GLM-5.1: general-purpose reasoning chat model. Free hosted endpoint. DEPRECATED: Free Endpoint availability is now listed as Deprecated on NVIDIA's catalog (Partner Endpoint/Download remain available); superseded by z-ai/glm-5.2.",
		},
		"z-ai/glm-5.2": {
			Ratio: 0, CompletionRatio: 1,
			InputModalities:             textInputs,
			OutputModalities:            textOutputs,
			SupportedFeatures:           reasoningFeatures,
			SupportedSamplingParameters: samplingParams,
			Description:                 "Z.ai GLM-5.2: flagship LLM for agentic workflows, coding, and long-horizon reasoning tasks. Free hosted endpoint.",
		},
		"z-ai/glm-5.3": {
			Ratio: 0, CompletionRatio: 1, ContextLength: 1048576,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			SupportedReasoningEfforts:   []string{"low", "high", "max"}, DefaultReasoningEffort: "max", Quantization: "fp4",
			Description: "NVIDIA-hosted GLM-5.3, exact dotted API ID, text-only 1M context. This endpoint serves NVFP4 weights, not the creator's FP8 checkpoint. Low/high/max effort, default max. Free trial endpoint; paid partner/deployment pricing requires channel overrides. Native chat_template_kwargs.clear_thinking defaults to false; NVIDIA recommends true for chat scenarios.",
		},
		"z-ai/glm-5.3-flash": {
			Ratio: 0, CompletionRatio: 1, ContextLength: 1048576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			SupportedReasoningEfforts:   []string{"low", "high", "max"}, DefaultReasoningEffort: "max", Quantization: "fp8",
			Description: "NVIDIA-hosted GLM-5.3-Flash, exact dotted API ID, native FP8 checkpoint and text/image input (up to eight images upstream). Low/high/max effort, default max; 1M context. Free trial endpoint only, not a paid partner tariff. Native chat_template_kwargs.clear_thinking defaults to false; NVIDIA recommends true for chat scenarios.",
		},
	}
}

// ModelList is derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// NvidiaToolingDefaults declares no built-in tool metering: NVIDIA does not
// publish per-tool pricing for the hosted API (retrieved 2026-06-14).
var NvidiaToolingDefaults = adaptor.ChannelToolConfig{}
