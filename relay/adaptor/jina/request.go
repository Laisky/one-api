package jina

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

var embeddingOptions = []string{
	"task", "embedding_type", "normalized", "truncate", "late_chunking",
	"return_multivector", "return_tokenized_input",
}

var rerankOptions = []string{"return_documents", "max_doc_length", "return_embeddings"}

// nativeOptions recovers allowlisted native fields from the reusable request body.
// Canonical routing fields (especially the mapped model) are never copied back.
// Top-level native fields take precedence over extra_body; explicit false survives.
func nativeOptions(c *gin.Context, extra map[string]any, allowed []string) (map[string]any, error) {
	out := make(map[string]any, len(allowed))
	for _, key := range allowed {
		if value, ok := extra[key]; ok {
			out[key] = value
		}
	}
	if c == nil || c.Request == nil {
		return out, nil
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "read jina native options")
	}
	if len(body) == 0 {
		return out, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.Wrap(err, "decode jina native options")
	}
	for _, key := range allowed {
		if value, ok := raw[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

// ConvertRequest preserves native embedding options and normalizes OCR token limits.
// It returns a new payload, leaving the caller's DTO unchanged for retries.
func (a *Adaptor) ConvertRequest(c *gin.Context, mode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("jina request is nil")
	}
	if mode == relaymode.Embeddings {
		out, err := nativeOptions(c, request.ExtraBody, embeddingOptions)
		if err != nil {
			return nil, err
		}
		out["model"] = request.Model
		out["input"] = request.Input
		if request.Dimensions != 0 {
			out["dimensions"] = request.Dimensions
		}
		if _, native := out["embedding_type"]; !native && request.EncodingFormat != "" {
			out["embedding_type"] = request.EncodingFormat
		}
		return out, nil
	}
	if mode != relaymode.ChatCompletions && mode != relaymode.ClaudeMessages {
		return nil, errors.Errorf("jina does not support request mode %d", mode)
	}
	out := *request
	if out.MaxCompletionTokens == nil && out.MaxTokens > 0 {
		limit := out.MaxTokens
		out.MaxCompletionTokens = &limit
	}
	if out.Model == "jina-ocr-v1" && out.MaxCompletionTokens == nil {
		limit := int(ModelRatios[out.Model].MaxOutputTokens)
		out.MaxCompletionTokens = &limit
	}
	out.MaxTokens = 0
	if out.Stream {
		// OCR billing needs upstream visual-token usage, not text-only estimation.
		out.StreamOptions = &model.StreamOptions{IncludeUsage: true}
	}
	return &out, nil
}

// ConvertRerankRequest preserves native result options and structured documents.
// Canonical model mapping, normalized text, and top_n remain authoritative.
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, request *model.RerankRequest) (any, error) {
	if request == nil {
		return nil, errors.New("jina rerank request is nil")
	}
	// Reuse the model-aware admission contract at the conversion boundary too.
	// Direct adaptor callers must not send image work to a text-only model.
	if _, err := QuoteRerank(request); err != nil {
		return nil, errors.Wrap(err, "validate Jina rerank model and input")
	}
	out, err := nativeOptions(c, nil, rerankOptions)
	if err != nil {
		return nil, err
	}
	out["model"] = request.Model
	out["query"] = request.Query
	out["documents"] = request.Documents
	if len(request.NativeQuery) > 0 {
		out["query"] = request.NativeQuery
	}
	if len(request.NativeDocuments) > 0 {
		out["documents"] = request.NativeDocuments
	}
	if request.TopN != nil {
		out["top_n"] = *request.TopN
	}
	if _, native := out["max_doc_length"]; !native && request.MaxTokensPerDoc != nil &&
		(request.Model == "jina-reranker-v3" || request.Model == "jina-reranker-v3.5") {
		out["max_doc_length"] = *request.MaxTokensPerDoc
	}
	return out, nil
}
