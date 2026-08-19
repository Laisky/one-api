package deepinfra

import (
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	deepInfraTextInput              = []string{"text"}
	deepInfraVisionInput            = []string{"text", "image"}
	deepInfraTextOutput             = []string{"text"}
	deepInfraChatSamplingParameters = []string{
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
)

// ModelRatios is a maintained pricing and capability snapshot for current
// DeepInfra serverless models. DeepInfra changes its catalog frequently; channel
// model configuration remains the override path for newly released, versioned,
// private, or custom-deployment model IDs.
var ModelRatios = map[string]adaptor.ModelConfig{
	"moonshotai/Kimi-K3":               textModel(2.85, 14.25, 0.285, 1048576, false, true, "Long-context Kimi K3 reasoning model."),
	"Qwen/Qwen3.8-2.4T-A95B":           textModel(2.00, 6.00, 0.20, 262144, false, true, "Qwen 3.8 flagship mixture-of-experts model."),
	"Qwen/Qwen3.8-27B":                 textModel(0.40, 3.00, 0.04, 262144, false, true, "Qwen 3.8 27B instruction and reasoning model."),
	"Qwen/Qwen3.8-Max":                 textModel(1.65, 4.951, 0.206, 262144, false, true, "Qwen 3.8 Max hosted model."),
	"Qwen/Qwen3.7-Max":                 textModel(2.50, 7.50, 0.50, 250000, false, true, "Qwen 3.7 Max hosted model."),
	"Qwen/Qwen3-Max":                   textModel(1.20, 6.00, 0.24, 250000, false, false, "Qwen3 Max general-purpose model."),
	"Qwen/Qwen3-Max-Thinking":          textModel(1.20, 6.00, 0.24, 250000, false, true, "Qwen3 Max reasoning model."),
	"Qwen/Qwen3.6-35B-A3B":             textModel(0.10, 0.95, 0.00, 262144, false, true, "Qwen 3.6 sparse 35B model."),
	"Qwen/Qwen3.6-27B":                 textModel(0.32, 3.20, 0.00, 262144, false, true, "Qwen 3.6 dense 27B model."),
	"Qwen/Qwen3.5-397B-A17B":           textModel(0.45, 3.00, 0.22, 262144, false, true, "Qwen 3.5 flagship sparse model."),
	"Qwen/Qwen3.5-122B-A10B":           textModel(0.29, 2.40, 0.00, 262144, false, true, "Qwen 3.5 122B sparse model."),
	"Qwen/Qwen3.5-35B-A3B":             textModel(0.14, 1.00, 0.05, 262144, false, true, "Qwen 3.5 efficient sparse model."),
	"Qwen/Qwen3.5-27B":                 textModel(0.26, 2.60, 0.00, 262144, false, true, "Qwen 3.5 dense 27B model."),
	"Qwen/Qwen3.5-9B":                  textModel(0.10, 0.15, 0.00, 262144, false, false, "Qwen 3.5 compact 9B model."),
	"Qwen/Qwen3-VL-235B-A22B-Instruct": textModel(0.20, 0.88, 0.11, 262144, true, false, "Qwen3 VL flagship vision-language model."),
	"Qwen/Qwen3-VL-30B-A3B-Instruct":   textModel(0.15, 0.60, 0.00, 262144, true, false, "Qwen3 VL efficient vision-language model."),

	"deepseek-ai/DeepSeek-V4-Pro-0813":   textModel(1.30, 2.60, 0.10, 1048576, false, true, "DeepSeek V4 Pro dated release."),
	"deepseek-ai/DeepSeek-V4-Pro":        textModel(1.30, 2.60, 0.10, 1048576, false, true, "DeepSeek V4 Pro long-context reasoning model."),
	"deepseek-ai/DeepSeek-V4-Flash-0731": textModel(0.08, 0.18, 0.016, 1048576, false, true, "DeepSeek V4 Flash dated release."),
	"deepseek-ai/DeepSeek-V4-Flash":      textModel(0.09, 0.18, 0.018, 1048576, false, true, "DeepSeek V4 Flash low-cost reasoning model."),
	"deepseek-ai/DeepSeek-V3.2":          textModel(0.26, 0.38, 0.13, 160000, false, true, "DeepSeek V3.2 reasoning model."),

	"zai-org/GLM-5.2":       textModel(0.75, 2.40, 0.14, 1048576, false, true, "GLM 5.2 long-context reasoning model."),
	"zai-org/GLM-5.1":       textModel(1.05, 3.50, 0.205, 198000, false, true, "GLM 5.1 reasoning model."),
	"zai-org/GLM-5":         textModel(0.60, 2.08, 0.12, 198000, false, true, "GLM 5 reasoning model."),
	"zai-org/GLM-4.7-Flash": textModel(0.06, 0.40, 0.01, 198000, false, true, "GLM 4.7 Flash economical reasoning model."),

	"moonshotai/Kimi-K2.7-Code": textModel(0.68, 3.40, 0.136, 262144, false, true, "Kimi K2.7 optimized for software engineering."),
	"moonshotai/Kimi-K2.6":      textModel(0.75, 3.50, 0.15, 262144, false, true, "Kimi K2.6 general reasoning model."),
	"moonshotai/Kimi-K2.5":      textModel(0.45, 2.25, 0.07, 262144, false, true, "Kimi K2.5 general reasoning model."),

	"nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B": textModel(0.50, 2.20, 0.10, 262144, false, true, "Nemotron 3 Ultra sparse reasoning model."),
	"nvidia/NVIDIA-Nemotron-3-Super-120B-A12B": textModel(0.085, 0.40, 0.00, 262144, false, true, "Nemotron 3 Super efficient sparse model."),

	"XiaomiMiMo/MiMo-V2.5-Pro": textModel(1.00, 3.00, 0.20, 1048576, false, true, "MiMo V2.5 Pro long-context reasoning model."),
	"XiaomiMiMo/MiMo-V2.5":     textModel(0.40, 2.00, 0.08, 262144, false, true, "MiMo V2.5 general reasoning model."),

	"google/gemma-4-26B-A4B-it": textModel(0.07, 0.34, 0.00, 262144, false, false, "Gemma 4 26B sparse instruction model."),
	"google/gemma-4-31B-it":     textModel(0.13, 0.38, 0.00, 262144, false, false, "Gemma 4 31B instruction model."),

	"ByteDance/Seed-1.8":      textModel(0.25, 2.00, 0.05, 250000, false, true, "Seed 1.8 general reasoning model."),
	"ByteDance/Seed-2.0-code": textModel(0.50, 3.00, 0.10, 262144, false, true, "Seed 2.0 coding model."),
	"ByteDance/Seed-2.0-mini": textModel(0.10, 0.40, 0.02, 262144, false, false, "Seed 2.0 compact model."),
	"ByteDance/Seed-2.0-pro":  textModel(0.50, 3.00, 0.10, 262144, false, true, "Seed 2.0 Pro reasoning model."),

	"MiniMaxAI/MiniMax-M2.7":       textModel(0.25, 1.00, 0.05, 196608, false, true, "MiniMax M2.7 reasoning model."),
	"MiniMaxAI/MiniMax-M2.7-Turbo": textModel(0.38, 1.70, 0.07, 196608, false, true, "Low-latency MiniMax M2.7 variant."),
	"MiniMaxAI/MiniMax-M3":         textModel(0.28, 1.10, 0.056, 524288, false, true, "MiniMax M3 long-context reasoning model."),

	"BAAI/bge-base-en-v1.5":                            embeddingModel(0.005, 512, "Compact English embedding model."),
	"BAAI/bge-en-icl":                                  embeddingModel(0.010, 8192, "Instruction-aware English embedding model."),
	"BAAI/bge-large-en-v1.5":                           embeddingModel(0.010, 512, "High-quality English embedding model."),
	"BAAI/bge-m3":                                      embeddingModel(0.010, 8192, "Multilingual and multi-function BGE embedding model."),
	"BAAI/bge-m3-multi":                                embeddingModel(0.010, 8192, "BGE M3 multilingual embedding variant."),
	"Qwen/Qwen3-Embedding-0.6B":                        embeddingModel(0.010, 32768, "Compact Qwen3 text embedding model."),
	"Qwen/Qwen3-Embedding-4B":                          embeddingModel(0.020, 32768, "Mid-size Qwen3 text embedding model."),
	"Qwen/Qwen3-Embedding-8B":                          embeddingModel(0.010, 32768, "Large Qwen3 text embedding model."),
	"google/embeddinggemma-300m":                       embeddingModel(0.002, 2048, "Compact multilingual EmbeddingGemma model."),
	"intfloat/e5-base-v2":                              embeddingModel(0.005, 512, "E5 base English text embedding model."),
	"intfloat/e5-large-v2":                             embeddingModel(0.010, 512, "E5 large English text embedding model."),
	"intfloat/multilingual-e5-large":                   embeddingModel(0.010, 512, "Multilingual E5 large embedding model."),
	"intfloat/multilingual-e5-large-instruct":          embeddingModel(0.010, 512, "Instruction-tuned multilingual E5 embedding model."),
	"nvidia/Nemotron-3-Embed-1B-BF16":                  embeddingModel(0.015, 32768, "NVIDIA multilingual Nemotron 1B BF16 embedding model."),
	"nvidia/Nemotron-3-Embed-1B-NVFP4":                 embeddingModel(0.010, 32768, "NVIDIA multilingual Nemotron 1B NVFP4 embedding model."),
	"nvidia/Nemotron-3-Embed-8B":                       embeddingModel(0.035, 32768, "NVIDIA multilingual Nemotron 8B embedding model."),
	"sentence-transformers/all-MiniLM-L12-v2":          embeddingModel(0.005, 512, "Sentence Transformers MiniLM L12 embedding model."),
	"sentence-transformers/all-MiniLM-L6-v2":           embeddingModel(0.005, 512, "Sentence Transformers MiniLM L6 embedding model."),
	"sentence-transformers/all-mpnet-base-v2":          embeddingModel(0.005, 512, "Sentence Transformers MPNet embedding model."),
	"sentence-transformers/multi-qa-mpnet-base-dot-v1": embeddingModel(0.005, 512, "MPNet semantic-search embedding model."),
	"sentence-transformers/paraphrase-MiniLM-L6-v2":    embeddingModel(0.005, 512, "MiniLM paraphrase embedding model."),

	"Qwen/Qwen3-Reranker-0.6B": rerankModel(0.010, 32768, "Compact Qwen3 cross-encoder reranker."),
	"Qwen/Qwen3-Reranker-4B":   rerankModel(0.025, 32768, "Mid-size Qwen3 cross-encoder reranker."),
	"Qwen/Qwen3-Reranker-8B":   rerankModel(0.050, 32768, "Large Qwen3 cross-encoder reranker for RAG pipelines."),

	"Qwen/Qwen3-ASR-0.6B":                                 minutePricedAudioModel(0.00020, "Compact multilingual Qwen3 speech recognition model."),
	"Qwen/Qwen3-ASR-1.7B":                                 minutePricedAudioModel(0.00045, "Flagship multilingual Qwen3 speech recognition model."),
	"mistralai/Voxtral-Mini-3B-2507":                      minutePricedAudioModel(0.00100, "Voxtral Mini transcription and speech translation model."),
	"mistralai/Voxtral-Small-24B-2507":                    minutePricedAudioModel(0.00300, "Voxtral Small transcription and speech translation model."),
	"nvidia/Nemotron-3.5-ASR-Streaming-Multilingual-0.6b": minutePricedAudioModel(0.00020, "Low-latency multilingual Nemotron ASR model."),
	"openai/whisper-large-v3":                             minutePricedAudioModel(0.00045, "Whisper large v3 transcription and translation model."),
	"openai/whisper-large-v3-turbo":                       minutePricedAudioModel(0.00020, "Faster Whisper large v3 transcription and translation model."),

	"Qwen/Qwen3-TTS":                     characterPricedAudioModel(20.0, "Qwen3 text-to-speech model."),
	"Qwen/Qwen3-TTS-VoiceDesign":         characterPricedAudioModel(20.0, "Qwen3 TTS model with prompt-driven voice design."),
	"Audio8/Audio8-TTS-Preview-0.6b":     characterPricedAudioModel(5.0, "Audio8 compact preview text-to-speech model."),
	"ResembleAI/chatterbox-multilingual": characterPricedAudioModel(1.0, "Multilingual Chatterbox text-to-speech model."),
	"ResembleAI/chatterbox-turbo":        characterPricedAudioModel(1.0, "Low-latency Chatterbox text-to-speech model."),
	"bosonai/HiggsAudioV2.5":             characterPricedAudioModel(20.0, "HiggsAudio V2.5 natural text-to-speech model."),
	"canopylabs/orpheus-3b-0.1-ft":       characterPricedAudioModel(7.0, "Orpheus empathetic text-to-speech model."),
	"hexgrad/Kokoro-82M":                 characterPricedAudioModel(0.62, "Compact and efficient Kokoro text-to-speech model."),
	"inworld-ai/realtime-tts-1.5-max":    characterPricedAudioModel(50.0, "High-quality Inworld real-time text-to-speech model."),
	"inworld-ai/realtime-tts-1.5-mini":   characterPricedAudioModel(25.0, "Low-latency Inworld real-time text-to-speech model."),
	"inworld-ai/realtime-tts-2":          characterPricedAudioModel(35.0, "Steerable Inworld real-time text-to-speech model."),
	"sesame/csm-1b":                      characterPricedAudioModel(7.0, "Sesame conversational speech generation model."),

	"black-forest-labs/FLUX-2-klein-4b": imageModel(0.014, "FLUX.2 Klein 4B text-to-image and image editing model."),
	"black-forest-labs/FLUX-2-klein-9b": imageModel(0.015, "FLUX.2 Klein 9B text-to-image and image editing model."),
}

// textModel builds token-priced text or vision model metadata.
func textModel(inputUSD, outputUSD, cachedInputUSD float64, contextLength int32, vision bool, reasoning bool, description string) adaptor.ModelConfig {
	completionRatio := 1.0
	if inputUSD > 0 {
		completionRatio = outputUSD / inputUSD
	}

	inputModalities := deepInfraTextInput
	if vision {
		inputModalities = deepInfraVisionInput
	}
	features := []string{"tools", "json_mode"}
	if reasoning {
		features = append(features, "reasoning")
	}

	config := adaptor.ModelConfig{
		Ratio:                       inputUSD * billingratio.MilliTokensUsd,
		CompletionRatio:             completionRatio,
		ContextLength:               contextLength,
		InputModalities:             append([]string(nil), inputModalities...),
		OutputModalities:            append([]string(nil), deepInfraTextOutput...),
		SupportedFeatures:           features,
		SupportedSamplingParameters: append([]string(nil), deepInfraChatSamplingParameters...),
		Description:                 description,
	}
	if cachedInputUSD > 0 {
		config.CachedInputRatio = cachedInputUSD * billingratio.MilliTokensUsd
	}
	return config
}

// embeddingModel builds text-token-priced embedding metadata.
func embeddingModel(inputUSD float64, contextLength int32, description string) adaptor.ModelConfig {
	ratio := inputUSD * billingratio.MilliTokensUsd
	return adaptor.ModelConfig{
		Ratio:           ratio,
		CompletionRatio: 1.0,
		ContextLength:   contextLength,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: ratio,
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"embedding"},
		Description:      description,
	}
}

// rerankModel builds token-priced reranking metadata.
func rerankModel(inputUSD float64, contextLength int32, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:                       inputUSD * billingratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               contextLength,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedSamplingParameters: []string{"top_n"},
		Description:                 description,
	}
}

// minutePricedAudioModel converts a provider per-minute ASR price to the
// audio-token ratio used by one-api's transcription and translation controller.
func minutePricedAudioModel(inputUSDPerMinute float64, description string) adaptor.ModelConfig {
	const promptTokensPerSecond = 10.0

	ratio := inputUSDPerMinute * float64(billingratio.QuotaPerUsd) / (60.0 * promptTokensPerSecond)
	return adaptor.ModelConfig{
		Ratio:           ratio,
		CompletionRatio: 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptTokensPerSecond: promptTokensPerSecond,
			UsdPerSecond:          inputUSDPerMinute / 60.0,
		},
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Description:      description,
	}
}

// characterPricedAudioModel builds metadata for speech models billed per input character.
func characterPricedAudioModel(inputUSDPerMillionCharacters float64, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:            inputUSDPerMillionCharacters * billingratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		Description:      description,
	}
}

// imageModel builds area-scaled per-image pricing for OpenAI-compatible image requests.
func imageModel(basePriceUSD float64, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: basePriceUSD,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"512x512":   0.25,
				"768x768":   0.5625,
				"1024x1024": 1.0,
				"1024x1536": 1.5,
				"1536x1024": 1.5,
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      description,
	}
}
