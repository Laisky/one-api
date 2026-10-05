package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
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
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeToolBoundaryPNG creates a deterministic bounded valid PNG with enough encoded text to distinguish native image pricing.
func claudeToolBoundaryPNG(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	var state uint32 = 7
	for i := range img.Pix {
		state = state*1664525 + 1013904223
		img.Pix[i] = byte(state >> 24)
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, img))
	require.Less(t, encoded.Len(), 50_000)
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

// testClaudeToolResultBoundaries exercises documented non-text blocks and provider-specific tool-result representation through real HTTP dispatch.
func testClaudeToolResultBoundaries(t *testing.T) {
	const actualModel = "claude-3-5-haiku-20241022"
	const lowBalance = int64(1000)
	text := strings.Repeat("bounded synthetic document or search result ", 2500)
	pngData := claudeToolBoundaryPNG(t)
	for _, shape := range []struct {
		name   string
		block  map[string]any
		needle string
	}{
		{"document", map[string]any{"type": "document", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": text}}, text},
		{"search_result", map[string]any{"type": "search_result", "source": "kb://synthetic-result", "title": "Synthetic result", "content": []any{map[string]any{"type": "text", "text": text}}}, text},
		{"search_metadata", map[string]any{"type": "search_result", "source": "kb://synthetic-result", "title": text, "content": []any{map[string]any{"type": "text", "text": "short result"}}}, text},
		{"image", map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": pngData}}, pngData},
	} {
		for _, channel := range []struct {
			name string
			kind int
		}{{"native", channeltype.Anthropic}, {"converted", channeltype.OpenAICompatible}} {
			for _, funded := range []bool{false, true} {
				name := "boundary/" + shape.name + "/" + channel.name
				if funded {
					name += "/funded"
				} else {
					name += "/limited"
				}
				t.Run(name, func(t *testing.T) {
					encodedBlock, err := json.Marshal(shape.block)
					require.NoError(t, err)
					wireTextTokens := openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "tool", Content: string(encodedBlock)}}, actualModel)
					require.Greater(t, wireTextTokens, int(lowBalance))
					// The native image remains an image; the converted provider receives its JSON/base64 as tool text.
					if shape.name == "image" {
						imageTokens, imageErr := openai.CountImageTokens("data:image/png;base64,"+pngData, "high", actualModel)
						require.NoError(t, imageErr)
						require.Less(t, imageTokens, int(lowBalance/2))
					}
					balance := lowBalance
					if funded {
						balance = int64(wireTextTokens*3 + 1000)
					}
					xaiVideoSetup(t, balance, false)
					canonicalAdmissionConfiguration(t)
					var calls atomic.Int32
					seen := make(chan claudeToolQuotaWire, 1)
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						body, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
						select {
						case seen <- claudeToolQuotaWire{Body: body, Path: r.URL.Path, Err: readErr}:
						default:
						}
						w.Header().Set("Content-Type", "application/json")
						if channel.kind == channeltype.Anthropic {
							_, _ = io.WriteString(w, claudeNonStreamUpstreamBody)
						} else {
							_, _ = io.WriteString(w, `{"id":"synthetic-boundary","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
						}
					}))
					t.Cleanup(server.Close)
					oldClient := client.HTTPClient
					client.HTTPClient = server.Client()
					client.HTTPClient.Timeout = 5 * time.Second
					t.Cleanup(func() { client.HTTPClient = oldClient })
					payload := map[string]any{"model": "alias", "max_tokens": 1,
						"messages": []any{
							map[string]any{"role": "user", "content": "Run the synthetic lookup."},
							map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "synthetic-call", "name": "lookup", "input": map[string]any{"query": "fixture"}}}},
							map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": []any{shape.block}}}},
						},
						"tools": []any{map[string]any{"name": "lookup", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}}},
					}
					body, err := json.Marshal(payload)
					require.NoError(t, err)
					require.Less(t, len(body), 1<<20)
					c, _, id := protocolContext(t, channel.kind, actualModel, "/v1/messages", string(body), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
					apiErr := RelayClaudeMessagesHelper(c)
					drainCriticalTasks(t)
					if calls.Load() > 0 {
						observed := <-seen
						require.NoError(t, observed.Err)
						var wire map[string]any
						require.NoError(t, json.Unmarshal(observed.Body, &wire))
						if channel.kind == channeltype.OpenAICompatible {
							var chat relaymodel.GeneralOpenAIRequest
							require.NoError(t, json.Unmarshal(observed.Body, &chat))
							var toolText string
							for _, message := range chat.Messages {
								if message.Role == "tool" {
									toolText = message.StringContent()
								}
							}
							require.Equal(t, string(encodedBlock), toolText, "independent expected representation must match the actual provider payload")
							require.Contains(t, toolText, shape.needle)
							capturedTokens := openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "tool", Content: toolText}}, actualModel)
							t.Logf("BOUNDARY_DISPATCH shape=%s path=%s captured_tool_tokens=%d balance=%d", shape.name, observed.Path, capturedTokens, balance)
						} else {
							messages := wire["messages"].([]any)
							content := messages[2].(map[string]any)["content"].([]any)
							result := content[0].(map[string]any)["content"].([]any)
							require.Equal(t, shape.block, result[0], "native structured content must be preserved")
							t.Logf("BOUNDARY_DISPATCH shape=%s path=%s preserved_block_bytes=%d balance=%d", shape.name, observed.Path, len(encodedBlock), balance)
						}
					}
					allow := funded || (shape.name == "image" && channel.kind == channeltype.Anthropic)
					if allow {
						require.EqualValues(t, 1, calls.Load())
						require.Nil(t, apiErr)
						return
					}
					// Only real provider dispatch confirms an admission bug; estimator-only disagreement is insufficient.
					require.Zero(t, calls.Load(), "underfunded provider-visible tool content must be rejected before dispatch")
					require.NotNil(t, apiErr)
					require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
					assertClaudeToolBoundaryLedger(t, balance, id)
				})
			}
		}
	}
}

// assertClaudeToolBoundaryLedger checks that rejected work changes no durable user, token, or request-cost balance.
func assertClaudeToolBoundaryLedger(t *testing.T, balance int64, id string) {
	t.Helper()
	require.Equal(t, balance, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance, token.RemainQuota)
	var rows int64
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
	require.Zero(t, rows)
}
