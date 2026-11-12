package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestClaudeNativeTextPresenceRoundTrip preserves metadata-only and explicit empty native text oneofs through both Claude response modes.
func TestClaudeNativeTextPresenceRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, part string
			present    bool
		}{
			{"metadata_only", `{"thoughtSignature":"c2ln"}`, false},
			{"explicit_empty", `{"text":"","thoughtSignature":"c2ln"}`, true},
		} {
			t.Run(fmt.Sprint(stream)+"/"+tc.name, func(t *testing.T) {
				c, _ := signatureTestContext()
				fixture := `{"candidates":[{"content":{"role":"model","parts":[` + tc.part + `]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
				upstream := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
				info := &meta.Meta{ActualModelName: "gemini-3.8-flash", PromptTokens: 2}
				var converted *http.Response
				if stream {
					result, failure := (&Adaptor{}).convertStreamingToClaudeResponse(c, upstream, []byte("data: "+fixture+"\n\n"), info)
					require.Nil(t, failure)
					converted = result
				} else {
					result, failure := (&Adaptor{}).convertNonStreamingToClaudeResponse(c, upstream, []byte(fixture), info)
					require.Nil(t, failure)
					converted = result
				}
				body, err := io.ReadAll(converted.Body)
				require.NoError(t, err)
				require.NoError(t, converted.Body.Close())
				var blocks []any
				if !stream {
					var response struct {
						Content []any `json:"content"`
					}
					require.NoError(t, json.Unmarshal(body, &response))
					blocks = response.Content
				} else {
					for _, line := range strings.Split(string(body), "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						var event struct {
							Type  string         `json:"type"`
							Block map[string]any `json:"content_block"`
						}
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
						if event.Type == "content_block_start" {
							blocks = append(blocks, event.Block)
						}
					}
				}
				require.Len(t, blocks, 1)
				block := blocks[0].(map[string]any)
				require.Equal(t, "text", block["type"])
				require.Equal(t, "", block["text"])
				chat, err := ConvertClaudeToNativeChat(c, &model.ClaudeRequest{Model: "gemini-3.8-flash", MaxTokens: 16, Messages: []model.ClaudeMessage{{Role: "assistant", Content: blocks}}})
				require.NoError(t, err)
				native, err := ConvertNativeRequestChecked(c, *chat)
				require.NoError(t, err)
				require.Len(t, native.Contents, 1)
				require.Len(t, native.Contents[0].Parts, 1)
				raw, err := json.Marshal(native.Contents[0].Parts[0])
				require.NoError(t, err)
				require.JSONEq(t, tc.part, string(raw), "native oneof presence and opaque signature must survive Claude replay")
				extra := block["extra_content"].(map[string]any)
				if tc.present {
					require.NotContains(t, extra, "oneapi")
				} else {
					require.Equal(t, map[string]any{"gemini_text_present": false}, extra["oneapi"], "metadata-only native Part must not gain the text oneof")
				}

			})
		}
	}
}
