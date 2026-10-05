package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageJSONRawSnapshot preserves the exact application JSON bytes for done-only fallback usage.
func TestResponseUsageJSONRawSnapshot(t *testing.T) {
	payloads := []struct{ name, raw string }{
		{"large_exponent", `{"n":1e400,"answer":"` + strings.Repeat("synthetic raw accounting evidence ", 100) + `"}`},
		{"inner_whitespace", `{"answer":` + strings.Repeat(" ", 2000) + `"synthetic"}`},
		{"quoted_scalar", `"synthetic\nquoted \"answer\""`},
		{"literal_null", "null"},
	}
	shapes := []struct{ name, before, after string }{
		{"root_json", `{"type":"response.output_json.done","output_index":0,"json":`, "}"},
		{"part_json", `{"type":"response.output_json.done","output_index":0,"part":{"type":"output_json","json":`, "}}"},
		{"output_json_block", `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_json","json":`, "}] }]}}"},
	}
	for _, shape := range shapes {
		for _, payload := range payloads {
			for _, measured := range []bool{false, true} {
				name := shape.name + "/" + payload.name
				if measured {
					name += "/measured"
				}
				t.Run(name, func(t *testing.T) {
					wire := "data: " + shape.before + payload.raw + shape.after + "\n\n"
					if measured {
						wire += `data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}` + "\n\n"
					}
					usage := ParseResponseUsage([]byte(wire+"data: [DONE]\n\n"), 7, func(s string) int { return len(s) })
					expected := len(payload.raw)
					if measured {
						expected = 3
					}
					require.Equal(t, expected, usage.CompletionTokens)

				})
			}
		}
	}
}
