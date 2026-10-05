package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// claudeToolQuotaWire captures a bounded fake-provider request without assertions in its goroutine.
type claudeToolQuotaWire struct {
	Body []byte
	Path string
	Err  error
}

// TestClaudeToolResultQuotaHTTP checks real native and converted admission against independently counted tool text.
func TestClaudeToolResultQuotaHTTP(t *testing.T) {
	for _, channel := range []struct {
		name string
		kind int
	}{
		{"native", channeltype.Anthropic}, {"converted", channeltype.OpenAICompatible},
	} {
		for _, structured := range []bool{false, true} {
			for _, funded := range []bool{false, true} {
				name := channel.name
				if structured {
					name += "/blocks"
				} else {
					name += "/string"
				}
				if funded {
					name += "/funded"
				} else {
					name += "/underfunded"
				}
				t.Run(name, func(t *testing.T) {
					const actualModel = "claude-3-5-haiku-20241022"
					const lowBalance = int64(1000)
					text := strings.Repeat("large synthetic tool output ", 44000)
					// This oracle tokenizes only the text that the provider receives; it never calls the Claude estimator.
					textTokens := openai.CountTokenText(text, actualModel)
					require.Greater(t, textTokens, int(lowBalance))
					balance := lowBalance
					if funded {
						balance = int64(textTokens*2 + 1000)
					}
					xaiVideoSetup(t, balance, false)
					canonicalAdmissionConfiguration(t)
					var calls atomic.Int32
					seen := make(chan claudeToolQuotaWire, 1)
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
						seen <- claudeToolQuotaWire{Body: body, Path: r.URL.Path, Err: err}
						w.Header().Set("Content-Type", "application/json")
						if channel.kind == channeltype.Anthropic {
							_, _ = io.WriteString(w, claudeNonStreamUpstreamBody)
						} else {
							_, _ = io.WriteString(w, `{"id":"synthetic-tool-quota","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
						}
					}))
					t.Cleanup(server.Close)
					oldClient := client.HTTPClient
					client.HTTPClient = server.Client()
					client.HTTPClient.Timeout = 5 * time.Second
					t.Cleanup(func() { client.HTTPClient = oldClient })
					var result any = text
					if structured {
						result = []any{map[string]any{"type": "text", "text": text}}
					}
					payload := map[string]any{
						"model": "alias", "max_tokens": 1,
						"messages": []any{
							map[string]any{"role": "user", "content": "Run the synthetic lookup."},
							map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "synthetic-call", "name": "lookup", "input": map[string]any{"query": "fixture"}}}},
							map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": result}}},
						},
						"tools": []any{map[string]any{"name": "lookup", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}}},
					}
					body, err := json.Marshal(payload)
					require.NoError(t, err)
					require.Greater(t, len(body), 1<<20)
					c, _, id := protocolContext(t, channel.kind, actualModel, "/v1/messages", string(body), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
					apiErr := RelayClaudeMessagesHelper(c)
					drainCriticalTasks(t)
					if calls.Load() > 0 {
						observed := <-seen
						require.NoError(t, observed.Err)
						require.Contains(t, string(observed.Body), text)
						t.Logf("PROVIDER_TOOL_RESULT_DISPATCH path=%s body_bytes=%d text_tokens=%d balance=%d", observed.Path, len(observed.Body), textTokens, balance)
					}
					if funded {
						require.EqualValues(t, 1, calls.Load(), "valid funded tool history must remain supported")
						require.Nil(t, apiErr)
						return
					}
					// Test dispatch first so the unchanged implementation demonstrates actual unpriced provider work.
					require.Zero(t, calls.Load(), "underfunded tool results must be rejected before provider dispatch")
					require.NotNil(t, apiErr)
					require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
					require.Equal(t, balance, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance, token.RemainQuota)
					var rows int64
					require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
					require.Zero(t, rows)
				})
			}
		}
	}
	testClaudeToolResultBoundaries(t)
}
