package ollama

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Reusable metadata fragments for Ollama-served open-weight models. Ollama runs
// locally and ships GGUF builds quantized to q4_K_M by default, which maps to
// the OpenRouter "int4" precision label.
//
// Sources:
//   - https://ollama.com/library
//   - Per-model pages under https://ollama.com/library/<id>
//   - Upstream HuggingFace cards linked from each Ollama README.
var (
	// ollamaTextModalities advertises the text-only modality used by every model
	// currently enumerated in this adaptor.
	ollamaTextModalities = []string{"text"}

	// ollamaChatSamplingParams enumerates the OpenAI-compatible sampling parameters
	// the Ollama HTTP /api/chat bridge accepts for typical chat models. Ollama
	// additionally honors top_k and repetition_penalty (mapped to repeat_penalty
	// upstream), which are not part of the strict OpenAI schema but are surfaced
	// here so callers can rely on them.
	ollamaChatSamplingParams = []string{
		"temperature",
		"top_p",
		"top_k",
		"max_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"repetition_penalty",
		"seed",
	}

	// ollamaChatFeatures lists the standard tool/JSON capabilities advertised by
	// Ollama tool-capable chat builds (Llama 3, Qwen 2+, etc.). Older base models
	// (Llama 2, original Qwen) override this with a nil slice.
	ollamaChatFeatures = []string{"tools", "json_mode"}
)

// ModelRatios contains a curated Ollama compatibility list.
// Model list is derived from the keys of this map, eliminating redundancy.
// Ollama's official search page now exposes a much broader local and cloud catalog, but this adaptor intentionally keeps a
// small stable set until there is an explicit product decision to broaden the supported surface.
//
// Pricing is symbolic (Ollama runs locally with no metered billing); the
// non-zero ratios preserve the historical behavior of charging a tiny token
// fee so usage still surfaces in dashboards.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Ollama Models - typically free for local usage
	"codellama:7b-instruct": {
		Ratio:                       0.01 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             16384,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "codellama/CodeLlama-7b-Instruct-hf",
		Description:                 "Meta Code Llama 7B instruct, fine-tuned for code generation and discussion; 16K context, q4_K_M GGUF.",
	},
	"llama2:7b": {
		Ratio:                       0.01 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "meta-llama/Llama-2-7b-chat-hf",
		Description:                 "Meta Llama 2 7B chat-tuned foundation model with 4K context; predates native tool calling.",
	},
	"llama2:latest": {
		Ratio:                       0.01 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "meta-llama/Llama-2-7b-chat-hf",
		Description:                 "Alias for llama2:7b chat (Meta Llama 2 7B chat, 4K context).",
	},
	"llama3:latest": {
		Ratio:                       0.01 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedFeatures:           ollamaChatFeatures,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "meta-llama/Meta-Llama-3-8B-Instruct",
		Description:                 "Meta Llama 3 8B Instruct chat model with 8K context; Ollama default tag points at the 8B build.",
	},
	"phi3:latest": {
		Ratio:                       0.01 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "microsoft/Phi-3-mini-128k-instruct",
		Description:                 "Microsoft Phi-3 Mini 3.8B instruct, lightweight reasoning-focused open model with 128K context.",
	},
	"qwen:0.5b-chat": {
		Ratio:                       0.005 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "Qwen/Qwen1.5-0.5B-Chat",
		Description:                 "Alibaba Qwen 1.5 0.5B chat-tuned model; ultra-compact 32K-context build for low-resource hosts.",
	},
	"qwen:7b": {
		Ratio:                       0.01 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ollamaTextModalities,
		OutputModalities:            ollamaTextModalities,
		SupportedSamplingParameters: ollamaChatSamplingParams,
		Quantization:                "int4",
		HuggingFaceID:               "Qwen/Qwen-7B-Chat",
		Description:                 "Alibaba Qwen 7B chat foundation model with 32K context; original Qwen lineage (pre-Qwen 1.5).",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// OllamaToolingDefaults notes that Ollama runs locally and publishes no tool pricing (retrieved 2026-04-28).
// Source: https://ollama.com/search
var OllamaToolingDefaults = adaptor.ChannelToolConfig{}
