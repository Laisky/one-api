package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestReview607MediaProtocolHTTP executes real controller/adaptor/quota paths
// with a captured HTTP upstream and SQLite ledgers, not just helper snapshots.
func TestReview607MediaProtocolHTTP(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "claude"} {
		for _, outcome := range []string{"unary", "stream", "insufficient", "count-error"} {
			t.Run(protocol+"/"+outcome, func(t *testing.T) {
				balance := int64(100000)
				if outcome == "insufficient" {
					balance = 10
				}
				xaiVideoSetup(t, balance, false)
				oldPre := config.PreConsumedQuota
				config.PreConsumedQuota = 0
				defer func() { config.PreConsumedQuota = oldPre }()
				type observation struct {
					path string
					body []byte
					err  error
				}
				seen := make(chan observation, 3)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					seen <- observation{r.URL.Path, body, err}
					if strings.HasSuffix(r.URL.Path, ":countTokens") {
						if outcome == "count-error" {
							w.WriteHeader(503)
							_, _ = io.WriteString(w, `{"error":{"message":"secret provider body"}}`)
							return
						}
						_, _ = io.WriteString(w, `{"totalTokens":100,"promptTokensDetails":[{"modality":"TEXT","tokenCount":2},{"modality":"VIDEO","tokenCount":50},{"modality":"DOCUMENT","tokenCount":48}]}`)
						return
					}
					receipt := `{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"answer"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":1,"totalTokenCount":101}}`
					if outcome == "stream" {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data: %s\n\n", receipt)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, receipt)
					}
				}))
				defer server.Close()
				oldClient := client.HTTPClient
				client.HTTPClient = server.Client()
				defer func() { client.HTTPClient = oldClient }()
				stream := outcome == "stream"
				content := `[{"type":"text","text":"before"},{"type":"video_url","video_url":{"url":"data:video/mp4;base64,YQ=="}},{"type":"file","file":{"file_data":"data:application/pdf;base64,JVBERg=="}},{"type":"text","text":"after"}]`
				path := "/v1/chat/completions"
				body := fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":%s}],"reasoning_effort":"high","max_tokens":8,"stream":%t}`, content, stream)
				if protocol == "responses" {
					path = "/v1/responses"
					content = `[{"type":"input_text","text":"before"},{"type":"input_video","video_url":"data:video/mp4;base64,YQ=="},{"type":"input_file","file_data":"data:application/pdf;base64,JVBERg=="},{"type":"input_text","text":"after"}]`
					body = fmt.Sprintf(`{"model":"alias","input":[{"role":"user","content":%s}],"reasoning":{"effort":"high"},"max_output_tokens":8,"stream":%t}`, content, stream)
				} else if protocol == "claude" {
					path = "/v1/messages"
					content = `[{"type":"text","text":"before"},{"type":"video","source":{"type":"base64","media_type":"video/mp4","data":"YQ=="}},{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERg=="}},{"type":"text","text":"after"}]`
					body = fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":%s}],"output_config":{"effort":"high"},"max_tokens":8,"stream":%t}`, content, stream)
				}
				c, w, _ := protocolContext(t, channeltype.Gemini, "gemini-3.1-flash-lite-image", path, body, server.URL, balance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				var apiErr *relaymodel.ErrorWithStatusCode
				switch protocol {
				case "chat":
					apiErr = RelayTextHelper(c)
				case "responses":
					apiErr = RelayResponseAPIHelper(c)
				case "claude":
					apiErr = RelayClaudeMessagesHelper(c)
				}
				drainCriticalTasks(t)
				if outcome == "insufficient" || outcome == "count-error" {
					require.NotNil(t, apiErr)
					require.Len(t, seen, 1, "only counting, never generation, before failed admission")
					require.True(t, strings.HasSuffix((<-seen).path, ":countTokens"))
					require.Equal(t, balance, reloadUserQuota(t))
					return
				}
				require.Nil(t, apiErr)
				require.Len(t, seen, 2)
				count, generate := <-seen, <-seen
				require.NoError(t, count.err)
				require.NoError(t, generate.err)
				require.True(t, strings.HasSuffix(count.path, ":countTokens"))
				require.False(t, strings.HasSuffix(generate.path, ":countTokens"))
				var payload gemini.ChatRequest
				require.NoError(t, json.Unmarshal(generate.body, &payload))
				require.Len(t, payload.Contents, 1)
				parts := payload.Contents[0].Parts
				require.Len(t, parts, 4)
				require.Equal(t, "before", parts[0].Text)
				require.Equal(t, "after", parts[3].Text)
				require.Equal(t, "video/mp4", parts[1].InlineData.MimeType)
				require.Equal(t, "application/pdf", parts[2].InlineData.MimeType)
				require.NotNil(t, payload.GenerationConfig.ThinkingConfig)
				require.Equal(t, "high", payload.GenerationConfig.ThinkingConfig.ThinkingLevel)
				require.Contains(t, w.Body.String(), "answer")
				require.Equal(t, balance-101, reloadUserQuota(t), "settle exact provider receipt once")
			})
		}
	}
}
