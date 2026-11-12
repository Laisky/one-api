package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponsesVertexRejectsUnpairedToolOutputs verifies orphan results never dispatch on Vertex while other providers keep compatibility downgrades.
func TestResponsesVertexRejectsUnpairedToolOutputs(t *testing.T) {
	vertex := vertexMixedTextProvider(t)
	providers := []providerMatrixProvider{vertex}
	for _, p := range providerMatrixProviders() {
		if p.name == "openai_compatible" || p.name == "gemini" {
			providers = append(providers, p)
		}
	}
	cases := []struct{ name, input string }{
		{"orphan", `[{"type":"function_call_output","call_id":"wrong","output":"orphan-result"}]`},
		{"after_complete_turn", `[{"type":"function_call","call_id":"right","name":"one","arguments":"{}"},{"type":"function_call_output","call_id":"right","output":"matched-result"},{"type":"function_call_output","call_id":"wrong","output":"orphan-result"}]`},
	}
	for _, p := range providers {
		for _, tc := range cases {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				body := `{"model":"` + p.actual + `","input":` + tc.input + `}`
				result := runProviderMatrix(t, p, false, body, providerMatrixOptions{})
				if p.name == "vertex_gemini" {
					require.NotNil(t, result.apiErr, "native Vertex must reject unmatched result intent")
					require.Equal(t, 400, result.apiErr.StatusCode)
					require.Zero(t, result.calls, "invalid native history must fail before provider dispatch")
					require.False(t, result.observed)
				} else {
					requireProviderMatrixDispatched(t, result)
					require.True(t, strings.Contains(string(result.got.Body), "orphan-result"), "compatibility content remains visible to other providers")
				}
			})
		}
	}
}
