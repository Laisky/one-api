package jina

import (
	"fmt"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains only Jina IDs accepted by the native inference schemas.
// Research date: 2026-09-17. Sources:
//   - https://jina.ai/models/
//   - https://api.jina.ai/openapi.json (2026.09.17.0130)
//   - https://api.jina.ai/v1/models (prices are USD per token, not per million)
//
// ReaderLM, jina-vlm, and jina-embedding-b-en-v1 appear in the catalog but are
// not accepted by these endpoint schemas. They are deliberately not advertised.
// Search models are token-priced, including rerank; none use PerCall billing.
var ModelRatios = buildModelRatios()

// buildModelRatios constructs catalog metadata without treating vector dimensions
// as generated token limits. It returns 25 unique native inference model IDs.
func buildModelRatios() map[string]adaptor.ModelConfig {
	models := make(map[string]adaptor.ModelConfig, 25)
	for _, spec := range []struct {
		name       string
		context    int32
		dimensions int
		usd        float64
		modalities []string
	}{
		{"jina-embeddings-v2-base-en", 8192, 768, 0.05, []string{"text"}},
		{"jina-embeddings-v2-base-zh", 8192, 768, 0.05, []string{"text"}},
		{"jina-embeddings-v2-base-de", 8192, 768, 0.05, []string{"text"}},
		{"jina-embeddings-v2-base-es", 8192, 768, 0.05, []string{"text"}},
		{"jina-embeddings-v2-base-code", 8192, 768, 0.05, []string{"text"}},
		{"jina-embeddings-v3", 8192, 1024, 0.05, []string{"text"}},
		{"jina-embeddings-v4", 32768, 2048, 0.05, []string{"text", "image", "file"}},
		{"jina-embeddings-v5-text-small", 32768, 1024, 0.05, []string{"text"}},
		{"jina-embeddings-v5-text-nano", 8192, 768, 0.02, []string{"text"}},
		{"jina-embeddings-v5-omni-small", 32768, 1024, 0.05, []string{"text", "image", "audio", "video", "file"}},
		{"jina-embeddings-v5-omni-nano", 8192, 768, 0.02, []string{"text", "image", "audio", "video", "file"}},
		{"jina-code-embeddings-0.5b", 32768, 896, 0.05, []string{"text"}},
		{"jina-code-embeddings-1.5b", 32768, 1536, 0.05, []string{"text"}},
		{"jina-clip-v1", 8192, 768, 0.05, []string{"text", "image"}},
		{"jina-clip-v2", 8192, 1024, 0.05, []string{"text", "image"}},
		{"jina-colbert-v1-en", 8192, 128, 0.05, []string{"text"}},
		{"jina-colbert-v2", 8192, 128, 0.05, []string{"text"}},
	} {
		admittedModalities := []string{"text"}
		for _, modality := range spec.modalities {
			if modality == "image" {
				admittedModalities = append(admittedModalities, modality)
			}
		}
		models[spec.name] = adaptor.ModelConfig{
			Ratio:            spec.usd * ratio.MilliTokensUsd,
			ContextLength:    spec.context,
			InputModalities:  admittedModalities,
			OutputModalities: []string{"embeddings"},
			HuggingFaceID:    "jinaai/" + spec.name,
			Description:      fmt.Sprintf("Jina embedding model; native vector width %d. Token-priced input; vector width is not a completion-token limit. Gateway admission currently permits supported text/image input only.", spec.dimensions),
		}
	}
	for _, spec := range []struct {
		name    string
		context int32
	}{
		{"jina-reranker-v1-tiny-en", 8192},
		{"jina-reranker-v1-turbo-en", 8192},
		{"jina-reranker-v1-base-en", 8192},
		{"jina-reranker-v2-base-multilingual", 1024},
		{"jina-reranker-m0", 10240},
		{"jina-reranker-v3", 134144},
		{"jina-reranker-v3.5", 131072},
	} {
		models[spec.name] = adaptor.ModelConfig{
			Ratio:            0.05 * ratio.MilliTokensUsd,
			ContextLength:    spec.context,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
			HuggingFaceID:    "jinaai/" + spec.name,
			Description:      "Jina reranking model; returns relevance scores through /v1/rerank. Billed by upstream input tokens, not by request count.",
		}
	}
	m0 := models["jina-reranker-m0"]
	m0.InputModalities = []string{"text", "image"}
	models["jina-reranker-m0"] = m0
	for _, name := range []string{"jina-colbert-v1-en", "jina-colbert-v2"} {
		cfg := models[name]
		cfg.Description = "Jina ColBERT: token-level 128-dimensional multi-vector embeddings and text reranking. Both endpoints use upstream token billing."
		models[name] = cfg
	}
	models["jina-ocr-v1"] = adaptor.ModelConfig{
		Ratio:             0.50 * ratio.MilliTokensUsd,
		CompletionRatio:   4,
		ContextLength:     32768,
		MaxOutputTokens:   8192,
		InputModalities:   []string{"text", "image"},
		OutputModalities:  []string{"text"},
		SupportedFeatures: []string{"streaming", "structured_outputs"},
		SupportedSamplingParameters: []string{
			"max_completion_tokens", "temperature", "top_p", "presence_penalty",
			"frequency_penalty", "stop", "seed", "logprobs", "top_logprobs", "logit_bias",
		},
		HuggingFaceID: "jinaai/jina-ocr-v1",
		Description:   "Document OCR via Chat Completions, with image input, streaming, and structured extraction. USD 0.50/M input and USD 2.00/M output tokens.",
	}
	return models
}
