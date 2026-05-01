package cloudflare

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Cloudflare Workers AI model entries. Slices
// keep the table compact and consistent with sibling adaptors.
var (
	cfTextInputs   = []string{"text"}
	cfVisionInputs = []string{"text", "image"}
	cfTextOutputs  = []string{"text"}

	cfChatFeatures      = []string{}
	cfToolsFeatures     = []string{"tools"}
	cfReasoningFeatures = []string{"reasoning"}

	cfBasicSamplingParams = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Cloudflare Workers AI pricing (retrieved 2026-04-28).
// Source: https://developers.cloudflare.com/workers-ai/platform/pricing/
var ModelRatios = map[string]adaptor.ModelConfig{
	// Meta Llama Models
	"@cf/meta/llama-3.2-1b-instruct": {
		Ratio: 0.027 * ratio.MilliTokensUsd, CompletionRatio: 0.201 / 0.027,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-3.2-1B-Instruct",
		Description:   "Meta Llama 3.2 1B Instruct served by Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.2-3b-instruct": {
		Ratio: 0.051 * ratio.MilliTokensUsd, CompletionRatio: 0.335 / 0.051,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-3.2-3B-Instruct",
		Description:   "Meta Llama 3.2 3B Instruct served by Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.1-8b-instruct-fp8-fast": {
		Ratio: 0.045 * ratio.MilliTokensUsd, CompletionRatio: 0.384 / 0.045,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-3.1-8B-Instruct",
		Description:   "Meta Llama 3.1 8B Instruct (fp8-fast) on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.2-11b-vision-instruct": {
		Ratio: 0.049 * ratio.MilliTokensUsd, CompletionRatio: 0.676 / 0.049,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-3.2-11B-Vision-Instruct",
		Description:   "Meta Llama 3.2 11B Vision Instruct on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.1-70b-instruct-fp8-fast": {
		Ratio: 0.293 * ratio.MilliTokensUsd, CompletionRatio: 2.253 / 0.293,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-3.1-70B-Instruct",
		Description:   "Meta Llama 3.1 70B Instruct (fp8-fast) on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.3-70b-instruct-fp8-fast": {
		Ratio: 0.293 * ratio.MilliTokensUsd, CompletionRatio: 2.253 / 0.293,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-3.3-70B-Instruct",
		Description:   "Meta Llama 3.3 70B Instruct (fp8-fast) on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.1-8b-instruct": {
		Ratio: 0.282 * ratio.MilliTokensUsd, CompletionRatio: 0.827 / 0.282,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-3.1-8B-Instruct",
		Description:   "Meta Llama 3.1 8B Instruct (fp16) on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.1-8b-instruct-fp8": {
		Ratio: 0.152 * ratio.MilliTokensUsd, CompletionRatio: 0.287 / 0.152,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-3.1-8B-Instruct",
		Description:   "Meta Llama 3.1 8B Instruct served at fp8 by Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.1-8b-instruct-awq": {
		Ratio: 0.123 * ratio.MilliTokensUsd, CompletionRatio: 0.266 / 0.123,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "int4",
		HuggingFaceID: "meta-llama/Llama-3.1-8B-Instruct",
		Description:   "Meta Llama 3.1 8B Instruct AWQ-quantized on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3-8b-instruct": {
		Ratio: 0.282 * ratio.MilliTokensUsd, CompletionRatio: 0.827 / 0.282,
		ContextLength: 8192, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Meta-Llama-3-8B-Instruct",
		Description:   "Meta Llama 3 8B Instruct on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3-8b-instruct-awq": {
		Ratio: 0.123 * ratio.MilliTokensUsd, CompletionRatio: 0.266 / 0.123,
		ContextLength: 8192, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "int4",
		HuggingFaceID: "meta-llama/Meta-Llama-3-8B-Instruct",
		Description:   "Meta Llama 3 8B Instruct AWQ-quantized on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-2-7b-chat-fp16": {
		Ratio: 0.556 * ratio.MilliTokensUsd, CompletionRatio: 6.667 / 0.556,
		ContextLength: 4096, MaxOutputTokens: 2048,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-2-7b-chat-hf",
		Description:   "Meta Llama 2 7B Chat (fp16) on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-guard-3-8b": {
		Ratio: 0.484 * ratio.MilliTokensUsd, CompletionRatio: 0.030 / 0.484,
		ContextLength: 128000, MaxOutputTokens: 256,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-Guard-3-8B",
		Description:   "Meta Llama Guard 3 8B safety classifier on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-4-scout-17b-16e-instruct": {
		Ratio: 0.270 * ratio.MilliTokensUsd, CompletionRatio: 0.850 / 0.270,
		ContextLength: 131072, MaxOutputTokens: 8192,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-4-Scout-17B-16E-Instruct",
		Description:   "Meta Llama 4 Scout 17B (16 experts) on Cloudflare Workers AI.",
	},

	// Mistral Models
	"@cf/mistral/mistral-7b-instruct-v0.1": {
		Ratio: 0.110 * ratio.MilliTokensUsd, CompletionRatio: 0.190 / 0.110,
		ContextLength: 32768, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "mistralai/Mistral-7B-Instruct-v0.1",
		Description:   "Mistral 7B Instruct v0.1 on Cloudflare Workers AI.",
	},
	"@cf/mistralai/mistral-small-3.1-24b-instruct": {
		Ratio: 0.351 * ratio.MilliTokensUsd, CompletionRatio: 0.555 / 0.351,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "mistralai/Mistral-Small-3.1-24B-Instruct-2503",
		Description:   "Mistral Small 3.1 24B Instruct multimodal model on Cloudflare Workers AI.",
	},

	// DeepSeek Models
	"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b": {
		Ratio: 0.497 * ratio.MilliTokensUsd, CompletionRatio: 4.881 / 0.497,
		ContextLength: 64000, MaxOutputTokens: 16000,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
		Description:   "DeepSeek R1 distilled into Qwen 32B reasoning model on Cloudflare Workers AI.",
	},

	// Google Models
	"@cf/google/gemma-3-12b-it": {
		Ratio: 0.345 * ratio.MilliTokensUsd, CompletionRatio: 0.556 / 0.345,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "google/gemma-3-12b-it",
		Description:   "Google Gemma 3 12B Instruct multimodal model on Cloudflare Workers AI.",
	},
	"@cf/google/gemma-4-26b-a4b-it": {
		Ratio: 0.100 * ratio.MilliTokensUsd, CompletionRatio: 0.300 / 0.100,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "google/gemma-4-26b-a4b-it",
		Description:   "Google Gemma 4 26B (4B-active MoE) on Cloudflare Workers AI.",
	},

	// Qwen Models
	"@cf/qwen/qwq-32b": {
		Ratio: 0.660 * ratio.MilliTokensUsd, CompletionRatio: 1.000 / 0.660,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "Qwen/QwQ-32B",
		Description:   "Alibaba Qwen QwQ 32B reasoning model on Cloudflare Workers AI.",
	},
	"@cf/qwen/qwen2.5-coder-32b-instruct": {
		Ratio: 0.660 * ratio.MilliTokensUsd, CompletionRatio: 1.000 / 0.660,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "Qwen/Qwen2.5-Coder-32B-Instruct",
		Description:   "Alibaba Qwen 2.5 Coder 32B Instruct on Cloudflare Workers AI.",
	},
	"@cf/qwen/qwen3-30b-a3b-fp8": {
		Ratio: 0.051 * ratio.MilliTokensUsd, CompletionRatio: 0.335 / 0.051,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "Qwen/Qwen3-30B-A3B",
		Description:   "Alibaba Qwen3 30B/A3B mixture-of-experts model (fp8) on Cloudflare Workers AI.",
	},

	// IBM / ZAI / NVIDIA / Moonshot
	"@cf/ibm-granite/granite-4.0-h-micro": {
		Ratio: 0.017 * ratio.MilliTokensUsd, CompletionRatio: 0.112 / 0.017,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "ibm-granite/granite-4.0-h-micro",
		Description:   "IBM Granite 4.0 Hybrid Micro on Cloudflare Workers AI.",
	},
	"@cf/zai-org/glm-4.7-flash": {
		Ratio: 0.060 * ratio.MilliTokensUsd, CompletionRatio: 0.400 / 0.060,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "zai-org/glm-4.7-flash",
		Description:   "Zhipu GLM 4.7 Flash on Cloudflare Workers AI.",
	},
	"@cf/nvidia/nemotron-3-120b-a12b": {
		Ratio: 0.500 * ratio.MilliTokensUsd, CompletionRatio: 1.500 / 0.500,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "nvidia/Nemotron-3-120B-A12B",
		Description:   "NVIDIA Nemotron 3 120B/A12B mixture-of-experts on Cloudflare Workers AI.",
	},
	"@cf/moonshotai/kimi-k2.5": {
		Ratio: 0.600 * ratio.MilliTokensUsd, CompletionRatio: 3.000 / 0.600, CachedInputRatio: 0.100 * ratio.MilliTokensUsd,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "moonshotai/Kimi-K2-Instruct",
		Description:   "Moonshot Kimi K2.5 long-context chat model on Cloudflare Workers AI.",
	},
	"@cf/moonshotai/kimi-k2.6": {
		Ratio: 0.950 * ratio.MilliTokensUsd, CompletionRatio: 4.000 / 0.950, CachedInputRatio: 0.160 * ratio.MilliTokensUsd,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "moonshotai/Kimi-K2-Instruct",
		Description:   "Moonshot Kimi K2.6 long-context chat model on Cloudflare Workers AI.",
	},

	// OpenAI OSS
	"@cf/openai/gpt-oss-120b": {
		Ratio: 0.350 * ratio.MilliTokensUsd, CompletionRatio: 0.750 / 0.350,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "openai/gpt-oss-120b",
		Description:   "OpenAI gpt-oss 120B reasoning model on Cloudflare Workers AI.",
	},
	"@cf/openai/gpt-oss-20b": {
		Ratio: 0.200 * ratio.MilliTokensUsd, CompletionRatio: 1.5,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "openai/gpt-oss-20b",
		Description:   "OpenAI gpt-oss 20B reasoning model on Cloudflare Workers AI.",
	},

	// Other LLMs
	"@cf/aisingapore/gemma-sea-lion-v4-27b-it": {
		Ratio: 0.351 * ratio.MilliTokensUsd, CompletionRatio: 0.555 / 0.351,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "aisingapore/Gemma-SEA-LION-v4-27B-IT",
		Description:   "AI Singapore Gemma SEA-LION v4 27B Instruct on Cloudflare Workers AI.",
	},

	// Embedding Models
	"@cf/baai/bge-small-en-v1.5": {
		Ratio: 0.020 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-small-en-v1.5",
		Description:   "BAAI bge-small-en-v1.5 embedding model on Cloudflare Workers AI.",
	},
	"@cf/baai/bge-base-en-v1.5": {
		Ratio: 0.067 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-base-en-v1.5",
		Description:   "BAAI bge-base-en-v1.5 embedding model on Cloudflare Workers AI.",
	},
	"@cf/baai/bge-large-en-v1.5": {
		Ratio: 0.204 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-large-en-v1.5",
		Description:   "BAAI bge-large-en-v1.5 embedding model on Cloudflare Workers AI.",
	},
	"@cf/baai/bge-m3": {
		Ratio: 0.012 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-m3",
		Description:   "BAAI bge-m3 multilingual embedding model on Cloudflare Workers AI.",
	},
	"@cf/pfnet/plamo-embedding-1b": {
		Ratio: 0.019 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "pfnet/plamo-embedding-1b",
		Description:   "PFNet PLaMo Embedding 1B (Japanese) on Cloudflare Workers AI.",
	},
	"@cf/qwen/qwen3-embedding-0.6b": {
		Ratio: 0.012 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   32768,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "Qwen/Qwen3-Embedding-0.6B",
		Description:   "Alibaba Qwen3 Embedding 0.6B on Cloudflare Workers AI.",
	},

	// Audio Models
	"@cf/openai/whisper": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0005 / 60},
		OutputModalities: cfTextOutputs,
		HuggingFaceID:    "openai/whisper-large-v3",
		Description:      "OpenAI Whisper speech-to-text on Cloudflare Workers AI.",
	},
	"@cf/openai/whisper-large-v3-turbo": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0005 / 60},
		OutputModalities: cfTextOutputs,
		HuggingFaceID:    "openai/whisper-large-v3-turbo",
		Description:      "OpenAI Whisper Large v3 Turbo speech-to-text on Cloudflare Workers AI.",
	},
	"@cf/myshell-ai/melotts": {
		Audio:           &adaptor.AudioPricingConfig{UsdPerSecond: 0.0002 / 60},
		InputModalities: cfTextInputs,
		HuggingFaceID:   "myshell-ai/MeloTTS-English",
		Description:     "MyShell MeloTTS multi-lingual TTS on Cloudflare Workers AI.",
	},
	"@cf/deepgram/aura-1": {
		Ratio:           15.0 * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		InputModalities: cfTextInputs,
		Description:     "Deepgram Aura 1 TTS on Cloudflare Workers AI.",
	},
	"@cf/deepgram/nova-3": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0052 / 60},
		OutputModalities: cfTextOutputs,
		Description:      "Deepgram Nova 3 speech-to-text on Cloudflare Workers AI.",
	},
	"@cf/deepgram/flux": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0077 / 60},
		OutputModalities: cfTextOutputs,
		Description:      "Deepgram Flux conversational ASR on Cloudflare Workers AI.",
	},
	"@cf/pipecat-ai/smart-turn-v2": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.00033795 / 60},
		OutputModalities: cfTextOutputs,
		Description:      "Pipecat smart-turn v2 voice activity detector on Cloudflare Workers AI.",
	},
	"@cf/deepgram/aura-2-en": {
		Ratio:           30.0 * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		InputModalities: cfTextInputs,
		Description:     "Deepgram Aura 2 English TTS on Cloudflare Workers AI.",
	},
	"@cf/deepgram/aura-2-es": {
		Ratio:           30.0 * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		InputModalities: cfTextInputs,
		Description:     "Deepgram Aura 2 Spanish TTS on Cloudflare Workers AI.",
	},

	// Other (Classification, Reranker, Translation, etc.)
	"@cf/huggingface/distilbert-sst-2-int8": {
		Ratio: 0.026 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "int8",
		HuggingFaceID: "distilbert-base-uncased-finetuned-sst-2-english",
		Description:   "DistilBERT SST-2 sentiment classifier (int8) on Cloudflare Workers AI.",
	},
	"@cf/baai/bge-reranker-base": {
		Ratio: 0.003 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-reranker-base",
		Description:   "BAAI bge-reranker-base on Cloudflare Workers AI.",
	},
	"@cf/meta/m2m100-1.2b": {
		Ratio: 0.342 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   1024,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "facebook/m2m100_1.2B",
		Description:   "Meta M2M100 1.2B multilingual translation on Cloudflare Workers AI.",
	},
	"@cf/ai4bharat/indictrans2-en-indic-1B": {
		Ratio: 0.342 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   1024,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "ai4bharat/indictrans2-en-indic-1B",
		Description:   "AI4Bharat IndicTrans2 English to Indic translation on Cloudflare Workers AI.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CloudflareToolingDefaults notes Workers AI publishes only neuron-based model pricing (no server-side tool billing as of 2026-04-28).
// Source: https://developers.cloudflare.com/workers-ai/platform/pricing/
var CloudflareToolingDefaults = adaptor.ChannelToolConfig{}
