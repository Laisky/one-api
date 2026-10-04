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

// TestSecurityClaudeJSONReviewBoundaryConverter checks actual delivered text and independent token oracles for reviewer JSON shapes.
func TestSecurityClaudeJSONReviewBoundaryConverter(t *testing.T) {
	text := strings.Repeat("synthetic accounting evidence ", 100)
	payload := `{"answer":"` + text + `"}`
	app := `{"text":"x","answer":"` + text + `"}`
	q, err := json.Marshal(payload)
	require.NoError(t, err)
	whitespace := `{"answer":` + strings.Repeat(" ", 2000) + `"synthetic"}`
	cases := []struct {
		name, wire, expected string
		exact                bool
	}{
		{"plain_object_whitespace", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + whitespace + `}` + "\n\n", whitespace, false},
		{"late_identity_alias", "data: " + `{"type":"response.output_json.delta","item_id":"a","delta":"{\"a\":"}` + "\n\n" + "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":"1}"}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"a","output_index":0,"json":{"a":1}}` + "\n\n", `{"a":1}`, true},
		{"plain_object_delta", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + payload + `}` + "\n\n", payload, false},
		{"part_application_text", "data: " + `{"type":"response.output_json.done","output_index":0,"part":{"type":"output_json","json":` + app + `}}` + "\n\n", app, false},
		{"distinct_items", "data: " + `{"type":"response.output_json.delta","item_id":"a","output_index":0,"delta":"{\"a\":1}"}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"b","output_index":1,"json":{"b":2}}` + "\n\n", `{"a":1}{"b":2}`, false},
		{"same_item_snapshot_control", "data: " + `{"type":"response.output_json.delta","item_id":"a","output_index":0,"delta":{"partial_json":` + string(q) + `}}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"a","output_index":0,"json":` + payload + `}` + "\n\n", payload, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			usage, apiErr := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.wire + "data: [DONE]\n\n"))}, 7, "gpt-4")
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
			if tc.exact {
				require.Contains(t, delivered.String(), tc.expected)
				require.Equal(t, CountTokenText(tc.expected, "gpt-4"), usage.CompletionTokens)
			} else {
				require.Equal(t, tc.expected, delivered.String())
				require.GreaterOrEqual(t, usage.CompletionTokens, CountTokenText(delivered.String(), "gpt-4"))
			}
		})
	}
}
