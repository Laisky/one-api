package cohere

import (
	"math"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

const (
	rerankDefaultDocumentTokens int64 = 4096
	rerankMaximumDocuments            = 10000
	rerankBillingChunkTokens    int64 = 500
	rerankDocumentsPerSearch    int64 = 100
)

// QuoteRerankSearchUnits normalizes the outbound document cap and returns a
// conservative search-unit allowance, not a measurement of provider usage.
// Cohere prices groups of 100 documents after 500-token billing chunking,
// including the query. Its v2 endpoint truncates each document to the explicit
// cap, and supported models truncate queries to half their context window.
// Four special-token slots cover both supported model families. We reserve the
// full caps rather than trust a different model's tokenizer or text byte size.
// https://cohere.com/pricing
// https://docs.cohere.com/v2/reference/rerank
// https://docs.cohere.com/docs/reranking-best-practices
// The operator-supplied context is used only for an otherwise unknown model;
// it must describe the same Cohere truncation contract before enabling that model.
func QuoteRerankSearchUnits(request *model.RerankRequest, configuredContext int32) (int64, error) {
	if request == nil || request.HasStructuredInput() || len(request.Documents) == 0 || len(request.Documents) > rerankMaximumDocuments {
		return 0, errors.New("Cohere rerank requires between 1 and 10000 text documents")
	}
	contextTokens := int64(configuredContext)
	if cfg, ok := ModelRatios[request.Model]; ok && cfg.ContextLength > 0 {
		contextTokens = int64(cfg.ContextLength)
	}
	if contextTokens < 2 {
		return 0, errors.New("Cohere rerank model requires an explicit context and truncation contract")
	}
	documentTokens := rerankDefaultDocumentTokens
	if request.MaxTokensPerDoc != nil {
		documentTokens = int64(*request.MaxTokensPerDoc)
	}
	if documentTokens <= 0 {
		return 0, errors.New("Cohere max_tokens_per_doc must be positive")
	}
	overhead := contextTokens/2 + 4
	if documentTokens > math.MaxInt64-overhead {
		return 0, errors.New("Cohere rerank document token allowance overflows")
	}
	combined := documentTokens + overhead
	chunks := combined / rerankBillingChunkTokens
	if combined%rerankBillingChunkTokens != 0 {
		chunks++
	}
	documents := int64(len(request.Documents))
	if chunks > math.MaxInt64/documents {
		return 0, errors.New("Cohere rerank aggregate chunk allowance overflows")
	}
	chunks *= documents
	units := chunks / rerankDocumentsPerSearch
	if chunks%rerankDocumentsPerSearch != 0 {
		units++
	}
	// Own the normalized scalar so cloning or retries cannot mutate caller state.
	normalizedCap := int(documentTokens)
	request.MaxTokensPerDoc = &normalizedCap
	return units, nil
}
