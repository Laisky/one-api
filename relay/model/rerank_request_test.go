package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRerankRequestNormalizeFromInput(t *testing.T) {
	req := &RerankRequest{
		Model:     "rerank-test",
		Input:     "  example query  ",
		Documents: []string{"doc1", "doc2"},
	}

	require.NoError(t, req.Normalize(), "expected normalize to succeed")
	require.Equal(t, "example query", req.Query, "expected trimmed query")
}

func TestRerankRequestNormalizeRequiredFields(t *testing.T) {
	req := &RerankRequest{Model: "rerank-test"}
	require.Error(t, req.Normalize(), "expected error when query missing")
}

func TestRerankRequestClone(t *testing.T) {
	docs := []string{"a", "b"}
	req := &RerankRequest{
		Model:     "rerank-test",
		Query:     "foo",
		Documents: docs,
	}

	clone := req.Clone()
	require.NotSame(t, req, clone, "expected clone to create new instance")
	clone.Documents[0] = "mutated"
	require.NotEqual(t, "mutated", req.Documents[0], "expected clone to deep copy documents slice")
}
