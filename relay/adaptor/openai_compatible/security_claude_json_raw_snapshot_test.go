package openai_compatible

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSecurityClaudeJSONRawSnapshotConverter compares actual delivered JSON with an independent tokenizer oracle.
func TestSecurityClaudeJSONRawSnapshotConverter(t *testing.T) {
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
					w := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(w)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					usage, apiErr := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire + "data: [DONE]\n\n"))}, 7, "gpt-4")
					require.Nil(t, apiErr)
					var delivered strings.Builder
					for _, line := range strings.Split(w.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						var e struct {
							Type  string `json:"type"`
							Delta struct {
								Text string `json:"text"`
							} `json:"delta"`
						}
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e))
						if e.Type == "content_block_delta" {
							delivered.WriteString(e.Delta.Text)
						}
					}
					require.Equal(t, payload.raw, delivered.String())
					expected := CountTokenText(delivered.String(), "gpt-4")
					if measured {
						expected = 3
					}
					require.Equal(t, expected, usage.CompletionTokens)

				})
			}
		}
	}
}
