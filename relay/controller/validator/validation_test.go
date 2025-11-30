package validator

import (
	"testing"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestValidateRerankRequest(t *testing.T) {
	req := &model.RerankRequest{
		Model:     "cohere-rerank",
		Query:     "hello",
		Documents: []string{"doc"},
	}
	if err := ValidateRerankRequest(req); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	bad := &model.RerankRequest{Model: "cohere"}
	if err := ValidateRerankRequest(bad); err == nil {
		t.Fatalf("expected error for missing query")
	}

	bad = &model.RerankRequest{Model: "cohere", Query: "hello"}
	if err := ValidateRerankRequest(bad); err == nil {
		t.Fatalf("expected error for missing documents")
	}

	topN := 0
	bad = &model.RerankRequest{
		Model:     "cohere",
		Query:     "hello",
		Documents: []string{"doc"},
		TopN:      &topN,
	}
	if err := ValidateRerankRequest(bad); err == nil {
		t.Fatalf("expected error for invalid top_n")
	}
}

func TestValidateUnknownParametersForRerank(t *testing.T) {
	// Valid rerank payload should not be considered unknown
	valid := []byte(`{"model":"rerank-v3.5","query":"What is X?","documents":["a","b"],"top_n":2}`)
	if err := ValidateUnknownParameters(valid); err != nil {
		t.Fatalf("expected no unknown-parameter error for valid rerank payload, got: %v", err)
	}

	// Payload with an unexpected field should be silently ignored (not cause error)
	// Unknown parameters are logged at DEBUG level but don't reject the request
	invalid := []byte(`{"model":"rerank-v3.5","query":"x","documents":["a"],"unexpected_field":123}`)
	if err := ValidateUnknownParameters(invalid); err != nil {
		t.Fatalf("expected no error for payload with unexpected_field (unknown params should be ignored), got: %v", err)
	}
}
