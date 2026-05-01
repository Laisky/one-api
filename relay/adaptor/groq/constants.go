package groq

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Reusable metadata fragments. Groq advertises "TruePoint Numerics" which is a
// Groq-proprietary mixed-precision scheme rather than a standard OpenRouter
// quantization label, so the Quantization field is intentionally left empty
// (omitempty) for those models.
//
// Sources:
//   - https://console.groq.com/docs/models
//   - https://console.groq.com/docs/agentic-tooling
//   - Per-model pages under https://console.groq.com/docs/model/<id>
var (
	// groqTextOnlyModalities advertises a chat model that consumes and emits text only.
	groqTextOnlyModalities = []string{"text"}
	// groqTextImageInModalities advertises text+image input with text output.
	groqTextImageInModalities = []string{"text", "image"}

	// groqChatSamplingParams enumerates the OpenAI-compatible sampling parameters
	// that Groq's chat completion endpoints accept for typical Llama/Qwen/Mixtral
	// style models. Source: https://console.groq.com/docs/api-reference#chat
	groqChatSamplingParams = []string{
		"temperature",
		"top_p",
		"max_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"seed",
		"n",
		"response_format",
		"logprobs",
		"top_logprobs",
		"tools",
		"tool_choice",
	}

	// groqReasoningSamplingParams is the restricted sampling-parameter subset
	// Groq accepts for reasoning-style endpoints (gpt-oss, qwen3 thinking, etc.).
	groqReasoningSamplingParams = []string{
		"max_tokens",
		"stop",
		"seed",
		"response_format",
		"tools",
		"tool_choice",
		"reasoning_effort",
	}

	// groqClassifierSamplingParams covers the minimal parameter set for
	// classifier-style guard models that emit a fixed label set.
	groqClassifierSamplingParams = []string{"max_tokens", "seed"}
)

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Pricing source: https://groq.com/pricing/
// Capability source: https://console.groq.com/docs/models
var ModelRatios = map[string]adaptor.ModelConfig{
	// Production Models
	"llama-3.3-70b-versatile": {
		Ratio:                       0.59 * ratio.MilliTokensUsd,
		CompletionRatio:             0.79 / 0.59,
		ContextLength:               131072,
		MaxOutputTokens:             32768,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-3.3-70B-Instruct",
		Description:                 "Meta's 70B Llama 3.3 instruct model served on Groq's LPU with 131K context and tool/JSON support.",
	},
	"llama-3.1-8b-instant": {
		Ratio:                       0.05 * ratio.MilliTokensUsd,
		CompletionRatio:             0.08 / 0.05,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-3.1-8B-Instruct",
		Description:                 "Meta's compact 8B Llama 3.1 instruct model with 131K context, optimized for low-latency chat.",
	},
	"meta-llama/llama-guard-4-12b": {
		Ratio:                       0.2 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             1024,
		InputModalities:             groqTextImageInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: groqClassifierSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-Guard-4-12B",
		Description:                 "12B safety classifier built on the Llama 4 Scout backbone; accepts text and up to five image inputs.",
	},
	"whisper-large-v3": {
		Ratio:                       0.111 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: []string{"language", "prompt", "response_format", "temperature"},
		HuggingFaceID:               "openai/whisper-large-v3",
		Description:                 "OpenAI Whisper large-v3 speech-to-text model (audio input, text output) with 99+ language support.",
	},
	"whisper-large-v3-turbo": {
		Ratio:                       0.04 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: []string{"language", "prompt", "response_format", "temperature"},
		HuggingFaceID:               "openai/whisper-large-v3-turbo",
		Description:                 "Whisper large-v3 turbo variant (audio input, text output) with 216x real-time speed factor.",
	},
	"openai/gpt-oss-120b": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.075 * ratio.MilliTokensUsd,
		CompletionRatio:             0.60 / 0.15,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		HuggingFaceID:               "openai/gpt-oss-120b",
		Description:                 "OpenAI's 120B open-weight Mixture-of-Experts reasoning model with built-in web search and code execution.",
	},
	"openai/gpt-oss-20b": {
		Ratio:                       0.075 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.0375 * ratio.MilliTokensUsd,
		CompletionRatio:             0.30 / 0.075,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		HuggingFaceID:               "openai/gpt-oss-20b",
		Description:                 "OpenAI's 20B open-weight MoE reasoning model with browser search and code execution support.",
	},

	// Preview Models
	"meta-llama/llama-4-maverick-17b-128e-instruct": {
		Ratio:                       0.2 * ratio.MilliTokensUsd,
		CompletionRatio:             0.6 / 0.2,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextImageInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-4-Maverick-17B-128E-Instruct",
		Description:                 "Meta Llama 4 Maverick (17B activated, 400B total MoE) multimodal model accepting up to 5 image inputs.",
	},
	"meta-llama/llama-4-scout-17b-16e-instruct": {
		Ratio:                       0.11 * ratio.MilliTokensUsd,
		CompletionRatio:             0.34 / 0.11,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextImageInModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-4-Scout-17B-16E-Instruct",
		Description:                 "Meta Llama 4 Scout (17B activated MoE) multimodal model with early-fusion image understanding.",
	},
	"meta-llama/llama-prompt-guard-2-22m": {
		Ratio:                       0.03 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               512,
		MaxOutputTokens:             512,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: groqClassifierSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-Prompt-Guard-2-22M",
		Description:                 "22M-parameter classifier that flags prompt injection and jailbreak attempts in real time.",
	},
	"meta-llama/llama-prompt-guard-2-86m": {
		Ratio:                       0.04 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               512,
		MaxOutputTokens:             512,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedSamplingParameters: groqClassifierSamplingParams,
		HuggingFaceID:               "meta-llama/Llama-Prompt-Guard-2-86M",
		Description:                 "86M-parameter multilingual classifier (mDeBERTa) detecting prompt injections across 8 languages.",
	},
	"moonshotai/kimi-k2-instruct-0905": {
		Ratio:                       1 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             3,
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: groqChatSamplingParams,
		HuggingFaceID:               "moonshotai/Kimi-K2-Instruct-0905",
		Description:                 "Moonshot AI Kimi K2 (1T-parameter MoE, 32B activated) coding/agent model with 256K context.",
	},
	"openai/gpt-oss-safeguard-20b": {
		Ratio:                       0.075 * ratio.MilliTokensUsd,
		CompletionRatio:             0.30 / 0.075,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		HuggingFaceID:               "openai/gpt-oss-safeguard-20b",
		Description:                 "20B GPT-OSS variant fine-tuned for policy-following safety classification with custom taxonomies.",
	},
	"qwen/qwen3-32b": {
		Ratio:                       0.29 * ratio.MilliTokensUsd,
		CompletionRatio:             0.59 / 0.29,
		ContextLength:               131072,
		MaxOutputTokens:             40960,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: groqReasoningSamplingParams,
		HuggingFaceID:               "Qwen/Qwen3-32B",
		Description:                 "Alibaba Qwen3-32B with switchable thinking/non-thinking modes for reasoning and general dialog.",
	},

	// New Models (Jan 2026)
	"canopylabs/orpheus-arabic-saudi": {
		Ratio:            40.0 * ratio.MilliTokensUsd, // per 1M characters
		CompletionRatio:  1,
		ContextLength:    4000,
		MaxOutputTokens:  50000,
		InputModalities:  groqTextOnlyModalities,
		OutputModalities: groqTextOnlyModalities,
		Description:      "Canopy Labs Orpheus text-to-speech model (Saudi Arabic). Output is rendered audio billed per character.",
	},
	"canopylabs/orpheus-v1-english": {
		Ratio:            22.0 * ratio.MilliTokensUsd, // per 1M characters
		CompletionRatio:  1,
		ContextLength:    4000,
		MaxOutputTokens:  50000,
		InputModalities:  groqTextOnlyModalities,
		OutputModalities: groqTextOnlyModalities,
		HuggingFaceID:    "canopylabs/orpheus-3b-0.1-ft",
		Description:      "Canopy Labs Orpheus v1 English text-to-speech (Llama-3.2-3B backbone) with bracketed vocal direction tags.",
	},
	"groq/compound": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             0.60 / 0.15,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqChatSamplingParams,
		Description:                 "Groq Compound agentic system with built-in web search, visit-website, code execution, browser automation, and Wolfram Alpha tools.",
	},
	"groq/compound-mini": {
		Ratio:                       0.11 * ratio.MilliTokensUsd,
		CompletionRatio:             0.34 / 0.11,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             groqTextOnlyModalities,
		OutputModalities:            groqTextOnlyModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"},
		SupportedSamplingParameters: groqChatSamplingParams,
		Description:                 "Lower-latency single-tool-call variant of Groq Compound, ~3x faster than groq/compound.",
	},
}

// ModelList derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// GroqToolingDefaults enumerates Groq's Compound and GPT-OSS built-in tool pricing.
// Source: https://groq.com/pricing/
var GroqToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"basic_search":          {UsdPerCall: 0.005},
		"advanced_search":       {UsdPerCall: 0.008},
		"visit_website":         {UsdPerCall: 0.001},
		"code_execution":        {UsdPerCall: 0.18},
		"browser_automation":    {UsdPerCall: 0.08},
		"browser_search_basic":  {UsdPerCall: 0.005},
		"browser_search_visit":  {UsdPerCall: 0.001},
		"code_execution_python": {UsdPerCall: 0.18},
	},
}
