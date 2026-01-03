package validator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestValidateRerankRequest(t *testing.T) {
	t.Parallel()
	req := &model.RerankRequest{
		Model:     "cohere-rerank",
		Query:     "hello",
		Documents: []string{"doc"},
	}
	err := ValidateRerankRequest(req)
	require.NoError(t, err, "expected valid request")

	bad := &model.RerankRequest{Model: "cohere"}
	err = ValidateRerankRequest(bad)
	require.Error(t, err, "expected error for missing query")

	bad = &model.RerankRequest{Model: "cohere", Query: "hello"}
	err = ValidateRerankRequest(bad)
	require.Error(t, err, "expected error for missing documents")

	topN := 0
	bad = &model.RerankRequest{
		Model:     "cohere",
		Query:     "hello",
		Documents: []string{"doc"},
		TopN:      &topN,
	}
	err = ValidateRerankRequest(bad)
	require.Error(t, err, "expected error for invalid top_n")
}

func TestValidateUnknownParametersForRerank(t *testing.T) {
	t.Parallel()
	// Valid rerank payload should not be considered unknown
	valid := []byte(`{"model":"rerank-v3.5","query":"What is X?","documents":["a","b"],"top_n":2}`)
	err := ValidateUnknownParameters(valid)
	require.NoError(t, err, "expected no unknown-parameter error for valid rerank payload")

	// Payload with an unexpected field should be silently ignored (not cause error)
	// Unknown parameters are logged at DEBUG level but don't reject the request
	invalid := []byte(`{"model":"rerank-v3.5","query":"x","documents":["a"],"unexpected_field":123}`)
	err = ValidateUnknownParameters(invalid)
	require.NoError(t, err, "expected no error for payload with unexpected_field (unknown params should be ignored)")
}
