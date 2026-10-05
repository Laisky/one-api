package cohere

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestCohereRerankSearchBudget covers billing chunk boundaries, model-specific
// query caps, caller-scalar ownership, and checked aggregate arithmetic.
func TestCohereRerankSearchBudget(t *testing.T) {
	for _, tc := range []struct {
		name, model         string
		docs, cap           int
		configured          int32
		want                int64
		invalid, structured bool
	}{
		{name: "default", docs: 3, want: 1},
		{name: "last_single_search", docs: 7, want: 1},
		{name: "next_search", docs: 8, want: 2},
		{name: "all_documents", docs: 100, want: 13},
		{name: "document_limit", docs: 10000, want: 1300},
		{name: "chunk_boundary", docs: 100, cap: 448, want: 5},
		{name: "next_chunk", docs: 100, cap: 449, want: 6},
		{name: "v4", model: "rerank-v4.0-pro", docs: 100, want: 41},
		{name: "known_model_cannot_shrink_query_bound", docs: 100, configured: 2, want: 13},
		{name: "custom_declared_contract", model: "tenant-rerank", docs: 100, configured: 32768, want: 41},
		{name: "custom_unknown_contract", model: "tenant-rerank", docs: 100, invalid: true},
		{name: "negative_context", model: "tenant-rerank", docs: 100, configured: -1, invalid: true},
		{name: "empty_documents", invalid: true},
		{name: "too_many_documents", docs: 10001, invalid: true},
		{name: "zero_cap", docs: 1, cap: -1, invalid: true},
		{name: "negative_cap", docs: 1, cap: -2, invalid: true},
		{name: "token_addition_overflow", docs: 1, cap: math.MaxInt, invalid: true},
		{name: "aggregate_overflow", docs: 10000, cap: math.MaxInt / 2, invalid: true},
		{name: "structured_input", docs: 1, structured: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := tc.model
			if name == "" {
				name = "rerank-v3.5"
			}
			docs := make([]string, tc.docs)
			for i := range docs {
				docs[i] = "document"
			}
			top := 1
			request := &model.RerankRequest{Model: name, Query: "query", Documents: docs, TopN: &top}
			if tc.cap != 0 {
				value := tc.cap
				if value == -1 {
					value = 0
				}
				request.MaxTokensPerDoc = &value
			}
			if tc.structured {
				request.NativeQuery = json.RawMessage(`{"text":"opaque"}`)
			}
			original := request.MaxTokensPerDoc
			units, err := QuoteRerankSearchUnits(request, tc.configured)
			if tc.invalid {
				require.Error(t, err)
				require.Zero(t, units)
				require.Same(t, original, request.MaxTokensPerDoc)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, units)
			require.NotNil(t, request.MaxTokensPerDoc)
			cap := tc.cap
			if cap == 0 {
				cap = 4096
			}
			require.Equal(t, cap, *request.MaxTokensPerDoc)
			if original != nil {
				require.NotSame(t, original, request.MaxTokensPerDoc)
				require.Equal(t, cap, *original)
			}
			converted, err := ConvertRerankRequest(*request)
			require.NoError(t, err)
			require.Equal(t, request.MaxTokensPerDoc, converted.MaxTokensPerDoc)
			request.TopN = nil
			again, err := QuoteRerankSearchUnits(request, tc.configured)
			require.NoError(t, err)
			require.Equal(t, units, again, "top_n changes results, not submitted work")
		})
	}
	units, err := QuoteRerankSearchUnits(nil, 0)
	require.Error(t, err)
	require.Zero(t, units)
}
