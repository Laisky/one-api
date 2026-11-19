package cloudflare

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Cloudflare Workers AI model entries. Slices
// keep the table compact and consistent with sibling adaptors.
var (
	cloudflareToolsAndReasoningFeatures = []string{"tools", "reasoning"}
	cfTextInputs                        = []string{"text"}
	cfVisionInputs                      = []string{"text", "image"}
	cfTextOutputs                       = []string{"text"}

	cfChatFeatures      = []string{}
	cfToolsFeatures     = []string{"tools"}
	cfReasoningFeatures = []string{"reasoning"}

	cfBasicSamplingParams = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Cloudflare Workers AI pricing (retrieved 2026-05-19).
//
// Models removed by the 2026-05-30 retirement are absent from this effective
// catalog. Upcoming retirements retain their historical prices and descriptions.
//
// Sources:
//   - https://developers.cloudflare.com/workers-ai/platform/pricing/
//   - https://developers.cloudflare.com/workers-ai/models/
//   - https://developers.cloudflare.com/workers-ai/changelog/
//
// Family model cards and lifecycle notices were also checked 2026-09-24
// and 2026-11-16:
// https://developers.cloudflare.com/workers-ai/models/glm-5.3/
// https://developers.cloudflare.com/workers-ai/models/glm-5.3-flash/
// https://developers.cloudflare.com/workers-ai/models/deepseek-v4.1-flash-1017/
// https://developers.cloudflare.com/workers-ai/models/inkling-2-small/
// https://developers.cloudflare.com/workers-ai/models/embeddinggemma-2/
// https://developers.cloudflare.com/changelog/product/workers-ai/
var ModelRatios = adaptor.JoinModelCatalogs(map[string]adaptor.ModelConfig{

	// ai4bharat models.
	"@cf/ai4bharat/indictrans2-en-indic-1B": {
		Ratio: 0.342 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   1024,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "ai4bharat/indictrans2-en-indic-1B",
		Description:   "AI4Bharat IndicTrans2 English to Indic translation on Cloudflare Workers AI.",
	},

	// aisingapore models.
	"@cf/aisingapore/gemma-sea-lion-v4-27b-it": {
		Ratio: 0.351 * ratio.MilliTokensUsd, CompletionRatio: 0.555 / 0.351,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "aisingapore/Gemma-SEA-LION-v4-27B-IT",
		Description:   "AI Singapore Gemma SEA-LION v4 27B Instruct on Cloudflare Workers AI.",
	},

	// baai models.
	"@cf/baai/bge-base-en-v1.5": {
		Ratio: 0.067 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-base-en-v1.5",
		Description:   "BAAI bge-base-en-v1.5 embedding model on Cloudflare Workers AI.",
		Embedding:     &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.067 * ratio.MilliTokensUsd},
	},
	"@cf/baai/bge-large-en-v1.5": {
		Ratio: 0.204 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-large-en-v1.5",
		Description:   "BAAI bge-large-en-v1.5 embedding model on Cloudflare Workers AI.",
		Embedding:     &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.204 * ratio.MilliTokensUsd},
	},
	"@cf/baai/bge-m3": {
		Ratio: 0.012 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-m3",
		Description:   "BAAI bge-m3 multilingual embedding model on Cloudflare Workers AI.",
		Embedding:     &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.012 * ratio.MilliTokensUsd},
	},
	"@cf/baai/bge-reranker-base": {
		Ratio: 0.003 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-reranker-base",
		Description:   "BAAI bge-reranker-base on Cloudflare Workers AI.",
	},
	"@cf/baai/bge-small-en-v1.5": {
		Ratio: 0.020 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "BAAI/bge-small-en-v1.5",
		Description:   "BAAI bge-small-en-v1.5 embedding model on Cloudflare Workers AI.",
		Embedding:     &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.020 * ratio.MilliTokensUsd},
	},

	// black-forest-labs models.
	"@cf/black-forest-labs/flux-1-schnell": {
		Image: &adaptor.ImagePricingConfig{
			// $0.0000528 per 512x512 tile; a 1024x1024 image = 4 tiles ≈ $0.000211.
			PricePerImageUsd: 0.0000528 * 4,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"image"},
		HuggingFaceID:    "black-forest-labs/FLUX.1-schnell",
		Description:      "FLUX.1 [schnell] 12B rectified-flow text-to-image model on Cloudflare Workers AI.",
	},
	"@cf/black-forest-labs/flux-2-dev": {
		Image: &adaptor.ImagePricingConfig{
			// Published: $0.00021 per input tile, $0.00041 per output tile.
			// Default 1024x1024 = 4 output tiles ≈ $0.00164 per image.
			PricePerImageUsd: 0.00041 * 4,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"image"},
		HuggingFaceID:    "black-forest-labs/FLUX.2-dev",
		Description:      "FLUX.2 [dev] multi-reference text-to-image model on Cloudflare Workers AI.",
	},
	"@cf/black-forest-labs/flux-2-klein-4b": {
		Image: &adaptor.ImagePricingConfig{
			// Published: $0.000059 per input tile, $0.000287 per output tile.
			PricePerImageUsd: 0.000287 * 4,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"image"},
		Description:      "FLUX.2 Klein 4B distilled text-to-image model on Cloudflare Workers AI.",
	},
	"@cf/black-forest-labs/flux-2-klein-9b": {
		Image: &adaptor.ImagePricingConfig{
			// Published: $0.015 per first megapixel (1024x1024).
			PricePerImageUsd: 0.015,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"image"},
		Description:      "FLUX.2 Klein 9B distilled text-to-image model on Cloudflare Workers AI.",
	},

	// deepgram models.
	"@cf/deepgram/aura-1": {
		Ratio:            15.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1,
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"audio"},
		Description:      "Deepgram Aura 1 TTS on Cloudflare Workers AI.",
	},
	"@cf/deepgram/aura-2-en": {
		Ratio:            30.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1,
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"audio"},
		Description:      "Deepgram Aura 2 English TTS on Cloudflare Workers AI.",
	},
	"@cf/deepgram/aura-2-es": {
		Ratio:            30.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1,
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"audio"},
		Description:      "Deepgram Aura 2 Spanish TTS on Cloudflare Workers AI.",
	},
	"@cf/deepgram/flux": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0077 / 60},
		InputModalities:  []string{"audio"},
		OutputModalities: cfTextOutputs,
		Description:      "Deepgram Flux conversational ASR on Cloudflare Workers AI.",
	},
	"@cf/deepgram/nova-3": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0052 / 60},
		InputModalities:  []string{"audio"},
		OutputModalities: cfTextOutputs,
		Description:      "Deepgram Nova 3 speech-to-text on Cloudflare Workers AI.",
	},

	// deepseek-ai models.
	"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b": {
		Ratio: 0.497 * ratio.MilliTokensUsd, CompletionRatio: 4.881 / 0.497,
		ContextLength: 64000, MaxOutputTokens: 16000,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp16",
		HuggingFaceID:             "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
		Description:               "DeepSeek R1 distilled into Qwen 32B reasoning model on Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/deepseek-ai/deepseek-v4-flash-0731": {
		Ratio:                       0.440 * ratio.MilliTokensUsd,
		CompletionRatio:             1.320 / 0.440,
		CachedInputRatio:            0.014 * ratio.MilliTokensUsd,
		ContextLength:               1_048_576,
		InputModalities:             cfTextInputs,
		OutputModalities:            cfTextOutputs,
		SupportedFeatures:           cloudflareToolsAndReasoningFeatures,
		SupportedSamplingParameters: cfBasicSamplingParams,
		Description:                 "DeepSeek V4 Flash 0731 long-context reasoning and function-calling model on Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/deepseek-ai/deepseek-v4-pro-0813": {
		Ratio:                       1.320 * ratio.MilliTokensUsd,
		CompletionRatio:             3.960 / 1.320,
		CachedInputRatio:            0.044 * ratio.MilliTokensUsd,
		ContextLength:               1_048_576,
		InputModalities:             cfVisionInputs,
		OutputModalities:            cfTextOutputs,
		SupportedFeatures:           cloudflareToolsAndReasoningFeatures,
		SupportedSamplingParameters: cfBasicSamplingParams,
		Description:                 "DeepSeek V4 Pro 0813 long-context multimodal reasoning and function-calling model on Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/deepseek-ai/deepseek-v4.1-flash-1017": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CachedInputRatio: 0.006 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30,
		ContextLength:   1_048_576,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedReasoningEfforts:   []string{"none", "low", "high", "max"},
		DefaultReasoningEffort:      "high",
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format", "reasoning_effort"},
		Description:                 "Workers AI DeepSeek V4.1 Flash 1017 with vision, tools and a 1,048,576-token context. Native none/low/high/max reasoning, default high; upstream accepts additional compatibility aliases. USD/M input/cache/output: 0.30/0.006/1.20. Maximum output and hosted precision are not specified. Workers Paid or prepaid AI Gateway credits are required upstream.",
	},

	// google models.
	"@cf/google/embeddinggemma-2": {
		Ratio: 0.0017 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   8192,
		InputModalities: []string{"text"},
		Embedding:       &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.0017 * ratio.MilliTokensUsd},
		Description:     "Workers AI EmbeddingGemma 2, released 2026-11-12, at $0.0017/M input tokens. Standard text embeddings accept dimensions 128/256/512/768 (default 768) and float/base64 encoding. The provider's separate nested-conversation media input is not advertised through this standard embeddings contract.",
	},
	"@cf/google/gemma-4-26b-a4b-it": {
		// Catalog labels: Function calling, Reasoning, Vision. 256K context window per model card.
		Ratio: 0.100 * ratio.MilliTokensUsd, CompletionRatio: 0.300 / 0.100,
		ContextLength: 256000, MaxOutputTokens: 0,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: nil,
		DefaultReasoningEffort:    "",
		Quantization:              "fp16",
		HuggingFaceID:             "google/gemma-4-26b-a4b-it",
		Description:               "Gemma 4 26B/A4B on Workers AI with vision, reasoning and tools. The provider schema does not publish top-level reasoning_effort levels, a default effort, or a separate output-token ceiling.",
	},

	// huggingface models.
	"@cf/huggingface/distilbert-sst-2-int8": {
		Ratio: 0.026 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   512,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "int8",
		HuggingFaceID: "distilbert-base-uncased-finetuned-sst-2-english",
		Description:   "DistilBERT SST-2 sentiment classifier (int8) on Cloudflare Workers AI.",
	},

	// ibm-granite models.
	"@cf/ibm-granite/granite-4.0-h-micro": {
		// Catalog labels: Function calling.
		Ratio: 0.017 * ratio.MilliTokensUsd, CompletionRatio: 0.112 / 0.017,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "ibm-granite/granite-4.0-h-micro",
		Description:   "IBM Granite 4.0 Hybrid Micro instruct model with function calling on Cloudflare Workers AI.",
	},

	// leonardo models.
	"@cf/leonardo/lucid-origin": {
		Image: &adaptor.ImagePricingConfig{
			// $0.006996 per 512x512 tile.
			PricePerImageUsd: 0.006996 * 4,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"image"},
		Description:      "Leonardo Lucid Origin text-to-image model on Cloudflare Workers AI.",
	},
	"@cf/leonardo/phoenix-1.0": {
		Image: &adaptor.ImagePricingConfig{
			// $0.005830 per 512x512 tile.
			PricePerImageUsd: 0.005830 * 4,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"image"},
		Description:      "Leonardo Phoenix 1.0 text-to-image model on Cloudflare Workers AI.",
	},

	// meta models.
	"@cf/meta/llama-3.1-70b-instruct-fp8-fast": {
		Ratio: 0.293 * ratio.MilliTokensUsd, CompletionRatio: 2.253 / 0.293,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-3.1-70B-Instruct",
		Description:   "Meta Llama 3.1 70B Instruct (fp8-fast) on Cloudflare Workers AI.",
	},
	"@cf/meta/llama-3.1-8b-instruct-fp8": {
		Ratio: 0.152 * ratio.MilliTokensUsd, CompletionRatio: 0.287 / 0.152,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-3.1-8B-Instruct",
		Description:   "Meta Llama 3.1 8B Instruct served at fp8 by Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
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
	"@cf/meta/llama-3.2-1b-instruct": {
		Ratio: 0.027 * ratio.MilliTokensUsd, CompletionRatio: 0.201 / 0.027,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-3.2-1B-Instruct",
		Description:   "Meta Llama 3.2 1B Instruct served by Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/meta/llama-3.2-3b-instruct": {
		Ratio: 0.051 * ratio.MilliTokensUsd, CompletionRatio: 0.335 / 0.051,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfChatFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "meta-llama/Llama-3.2-3B-Instruct",
		Description:   "Meta Llama 3.2 3B Instruct served by Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
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
	"@cf/meta/llama-4-scout-17b-16e-instruct": {
		Ratio: 0.270 * ratio.MilliTokensUsd, CompletionRatio: 0.850 / 0.270,
		ContextLength: 131072, MaxOutputTokens: 8192,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp8",
		HuggingFaceID: "meta-llama/Llama-4-Scout-17B-16E-Instruct",
		Description:   "Meta Llama 4 Scout 17B (16 experts) on Cloudflare Workers AI.",
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
	"@cf/meta/m2m100-1.2b": {
		Ratio: 0.342 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   1024,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "facebook/m2m100_1.2B",
		Description:   "Meta M2M100 1.2B multilingual translation on Cloudflare Workers AI.",
	},

	// microsoft models.
	"@cf/microsoft/resnet-50": {
		// $2.51 per million images.
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 2.51 / 1_000_000,
			MinImages:        1,
		},
		InputModalities:  []string{"image"},
		OutputModalities: cfTextOutputs,
		HuggingFaceID:    "microsoft/resnet-50",
		Description:      "Microsoft ResNet-50 image classifier on Cloudflare Workers AI.",
	},

	// mistralai models.
	"@cf/mistralai/mistral-small-3.1-24b-instruct": {
		Ratio: 0.351 * ratio.MilliTokensUsd, CompletionRatio: 0.555 / 0.351,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfToolsFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		Quantization:  "fp16",
		HuggingFaceID: "mistralai/Mistral-Small-3.1-24B-Instruct-2503",
		Description:   "Mistral Small 3.1 24B Instruct multimodal model on Cloudflare Workers AI.",
	},

	// moondream models.
	"@cf/moondream/moondream3.1-9B-A2B": {
		// Reasoning is a boolean toggle (default: true) enabling a reasoning
		// trace on the `query` task, not an effort-tiered parameter, so no
		// SupportedReasoningEfforts/DefaultReasoningEffort is set. No
		// tools/function-calling documented for this model.
		Ratio: 0.300 * ratio.MilliTokensUsd, CompletionRatio: 1.000 / 0.300,
		ContextLength: 32768, MaxOutputTokens: 28672,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		HuggingFaceID: "moondream/moondream3.1-9B-A2B",
		Description:   "Moondream 3.1 9B/A2B mixture-of-experts vision-language model (query, caption, point, detect tasks) on Cloudflare Workers AI.",
	},

	// moonshotai models.
	"@cf/moonshotai/kimi-k2.6": {
		Ratio: 0.950 * ratio.MilliTokensUsd, CompletionRatio: 4.000 / 0.950, CachedInputRatio: 0.160 * ratio.MilliTokensUsd,
		ContextLength: 262144, MaxOutputTokens: 0,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: []string{"none", "high", "low", "medium", "max"},
		DefaultReasoningEffort:    "high",
		Quantization:              "fp8",
		HuggingFaceID:             "moonshotai/Kimi-K2.6",
		Description:               "Kimi K2.6 on Workers AI. Canonical reasoning controls are none/high (default high); low/medium/max are accepted aliases for high, not three different budgets. Text and image input; Cloudflare USD tariffs are independent of Moonshot native prices." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/moonshotai/kimi-k2.7-code": {
		// Added on Cloudflare Workers AI 2026-06-12; $0.95 in / $0.19 cached / $4.00 out.
		Ratio: 0.950 * ratio.MilliTokensUsd, CompletionRatio: 4.000 / 0.950, CachedInputRatio: 0.190 * ratio.MilliTokensUsd,
		ContextLength: 262144, MaxOutputTokens: 0,
		InputModalities: cfVisionInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: nil,
		DefaultReasoningEffort:    "",
		Quantization:              "fp8",
		HuggingFaceID:             "moonshotai/Kimi-K2.7-Code",
		Description:               "Kimi K2.7 Code on Workers AI with text/image input and always-on reasoning. No tunable top-level reasoning_effort vocabulary or separate output ceiling is published." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},

	// myshell-ai models.
	"@cf/myshell-ai/melotts": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0002 / 60},
		InputModalities:  cfTextInputs,
		OutputModalities: []string{"audio"},
		HuggingFaceID:    "myshell-ai/MeloTTS-English",
		Description:      "MyShell MeloTTS multi-lingual TTS on Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},

	// nvidia models.
	"@cf/nvidia/nemotron-3-120b-a12b": {
		Ratio: 0.500 * ratio.MilliTokensUsd, CompletionRatio: 1.500 / 0.500,
		ContextLength: 256000, MaxOutputTokens: 0,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: nil,
		DefaultReasoningEffort:    "",
		Quantization:              "fp16",
		HuggingFaceID:             "nvidia/Nemotron-3-120B-A12B",
		Description:               "Nemotron 3 Super on Workers AI. Reasoning is controlled upstream with chat_template_kwargs.enable_thinking and low_effort, not top-level reasoning_effort. This metadata update does not add unsupported template controls to the gateway request schema." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},

	// openai models.
	"@cf/openai/gpt-oss-120b": {
		Ratio: 0.350 * ratio.MilliTokensUsd, CompletionRatio: 0.750 / 0.350,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp16",
		HuggingFaceID:             "openai/gpt-oss-120b",
		Description:               "OpenAI gpt-oss 120B reasoning model on Cloudflare Workers AI.",
	},
	"@cf/openai/gpt-oss-20b": {
		Ratio: 0.200 * ratio.MilliTokensUsd, CompletionRatio: 1.5,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp16",
		HuggingFaceID:             "openai/gpt-oss-20b",
		Description:               "OpenAI gpt-oss 20B reasoning model on Cloudflare Workers AI.",
	},
	"@cf/openai/whisper": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0005 / 60},
		InputModalities:  []string{"audio"},
		OutputModalities: cfTextOutputs,
		HuggingFaceID:    "openai/whisper-large-v3",
		Description:      "OpenAI Whisper speech-to-text on Cloudflare Workers AI.",
	},
	"@cf/openai/whisper-large-v3-turbo": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.0005 / 60},
		InputModalities:  []string{"audio"},
		OutputModalities: cfTextOutputs,
		HuggingFaceID:    "openai/whisper-large-v3-turbo",
		Description:      "OpenAI Whisper Large v3 Turbo speech-to-text on Cloudflare Workers AI.",
	},

	// pfnet models.
	"@cf/pfnet/plamo-embedding-1b": {
		Ratio: 0.019 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   4096,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "pfnet/plamo-embedding-1b",
		Description:   "PFNet PLaMo Embedding 1B (Japanese) on Cloudflare Workers AI.",
		Embedding:     &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.019 * ratio.MilliTokensUsd},
	},

	// pipecat-ai models.
	"@cf/pipecat-ai/smart-turn-v2": {
		Audio:            &adaptor.AudioPricingConfig{UsdPerSecond: 0.00033795 / 60},
		InputModalities:  []string{"audio"},
		OutputModalities: cfTextOutputs,
		Description:      "Pipecat smart-turn v2 voice activity detector on Cloudflare Workers AI.",
	},

	// qwen models.
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
		Description:   "Alibaba Qwen3 30B/A3B mixture-of-experts model (fp8) on Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/qwen/qwen3-embedding-0.6b": {
		Ratio: 0.012 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   32768,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		Quantization:  "fp16",
		HuggingFaceID: "Qwen/Qwen3-Embedding-0.6B",
		Description:   "Alibaba Qwen3 Embedding 0.6B on Cloudflare Workers AI.",
		Embedding:     &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.012 * ratio.MilliTokensUsd},
	},
	"@cf/qwen/qwen3.8-27b": {
		Ratio:                       0.450 * ratio.MilliTokensUsd,
		CompletionRatio:             3.200 / 0.450,
		ContextLength:               262_144,
		InputModalities:             cfVisionInputs,
		OutputModalities:            cfTextOutputs,
		SupportedFeatures:           cloudflareToolsAndReasoningFeatures,
		SupportedSamplingParameters: cfBasicSamplingParams,
		Description:                 "Qwen 3.8 27B multimodal reasoning and function-calling model on Cloudflare Workers AI.",
	},
	"@cf/qwen/qwq-32b": {
		Ratio: 0.660 * ratio.MilliTokensUsd, CompletionRatio: 1.000 / 0.660,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: cfReasoningFeatures, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp16",
		HuggingFaceID:             "Qwen/QwQ-32B",
		Description:               "Alibaba Qwen QwQ 32B reasoning model on Cloudflare Workers AI." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},

	// thinkingmachines models.
	"@cf/thinkingmachines/inkling-2-small": {
		Ratio: 0.30 * ratio.MilliTokensUsd, CachedInputRatio: 0.10 * ratio.MilliTokensUsd, CompletionRatio: 1.00 / 0.30,
		ContextLength:   1_048_576,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedReasoningEfforts:   []string{"none", "minimal", "low", "medium", "high", "max"},
		DefaultReasoningEffort:      "high",
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format", "reasoning_effort"},
		Description:                 "Workers AI Inkling 2 Small, released 2026-11-12, with vision, tools and a 1,048,576-token context. Native none/minimal/low/medium/high/max reasoning, default high. USD/M input/cache/output: 0.30/0.10/1.00. Maximum output and hosted precision are not specified. Workers Paid or prepaid AI Gateway credits are required upstream.",
	},

	// zai-org models.
	"@cf/zai-org/glm-4.7-flash": {
		Ratio: 0.060 * ratio.MilliTokensUsd, CompletionRatio: 0.400 / 0.060,
		ContextLength: 131072, MaxOutputTokens: 0,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: nil,
		DefaultReasoningEffort:    "",
		Quantization:              "fp16",
		HuggingFaceID:             "zai-org/glm-4.7-flash",
		Description:               "GLM 4.7 Flash on Workers AI with reasoning and tools. No tunable top-level reasoning_effort levels or separate output ceiling are published. Input price follows the platform rate card ($0.060/M); the model page displays $0.0605/M, an unresolved official-source discrepancy.",
	},
	"@cf/zai-org/glm-5.2": {
		Ratio: 1.400 * ratio.MilliTokensUsd, CompletionRatio: 4.400 / 1.400, CachedInputRatio: 0.260 * ratio.MilliTokensUsd,
		ContextLength: 262144, MaxOutputTokens: 0,
		InputModalities: cfTextInputs, OutputModalities: cfTextOutputs,
		SupportedFeatures: []string{"tools", "reasoning"}, SupportedSamplingParameters: cfBasicSamplingParams,
		SupportedReasoningEfforts: []string{"none", "minimal", "low", "medium", "high", "max", "xhigh"},
		DefaultReasoningEffort:    "max",
		Quantization:              "fp8",
		HuggingFaceID:             "zai-org/GLM-5.2",
		Description:               "GLM 5.2 on Workers AI. Canonical reasoning modes are none/high/max (default max); minimal disables, low/medium map to high, and xhigh maps to max. No separately published output-token ceiling is inferred." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
	},
	"@cf/zai-org/glm-5.3": {
		Ratio: 1.4 * ratio.MilliTokensUsd, CachedInputRatio: 0.26 * ratio.MilliTokensUsd, CompletionRatio: 4.4 / 1.4,
		ContextLength:   1048576,
		InputModalities: []string{"text"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
		Description:                 "GLM 5.3 on Workers AI with the current 1,048,576-token context limit and low/high/max reasoning (default max). Reasoning cannot be disabled; upstream maps none/minimal/medium/xhigh to max. Paid-plan eligibility is controlled upstream.",
		HuggingFaceID:               "",
		MaxOutputTokens:             0,
		SupportedReasoningEfforts:   []string{"low", "high", "max"},
		DefaultReasoningEffort:      "max",
	},
	"@cf/zai-org/glm-5.3-flash": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.15,
		ContextLength:   1048576,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
		Description:                 "GLM 5.3 Flash on Workers AI with native vision, the current 1,048,576-token context limit, and low/high/max reasoning (default max). Reasoning cannot be disabled; upstream maps none/minimal/medium/xhigh to max." + " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations.",
		HuggingFaceID:               "",
		MaxOutputTokens:             0,
		SupportedReasoningEfforts:   []string{"low", "high", "max"},
		DefaultReasoningEffort:      "max",
	},
})

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CloudflareToolingDefaults notes Workers AI publishes only neuron-based model pricing (no server-side tool billing as of 2026-04-28).
// Source: https://developers.cloudflare.com/workers-ai/platform/pricing/
var CloudflareToolingDefaults = adaptor.ChannelToolConfig{}
