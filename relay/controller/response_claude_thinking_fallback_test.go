package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeThinkingTurnOneSSE is a Claude Opus stream whose thinking uses the
// default omitted display: an empty-text block whose only state is sig1.
var claudeThinkingTurnOneSSE = []string{
	`{"type":"message_start","message":{"id":"msg_turn1","type":"message","role":"assistant","model":"claude-opus-5-5","usage":{"input_tokens":11,"output_tokens":0}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig1"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"shell","input":{}}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":\"ls\"}"}}`,
	`{"type":"content_block_stop","index":1}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":21}}`,
	`{"type":"message_stop"}`,
}

// claudeThinkingTurnOneJSON is the non-streaming equivalent of turn one.
const claudeThinkingTurnOneJSON = `{"id":"msg_turn1","type":"message","role":"assistant","model":"claude-opus-5-5",` +
	`"content":[{"type":"thinking","thinking":"","signature":"sig1"},{"type":"tool_use","id":"toolu_1","name":"shell","input":{"cmd":"ls"}}],` +
	`"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":11,"output_tokens":21}}`

// claudeThinkingTurnTwoSSE finishes the turn with plain text.
var claudeThinkingTurnTwoSSE = []string{
	`{"type":"message_start","message":{"id":"msg_turn2","type":"message","role":"assistant","model":"claude-opus-5-5","usage":{"input_tokens":30,"output_tokens":0}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
	`{"type":"message_stop"}`,
}

// claudeThinkingTurnTwoJSON is the non-streaming equivalent of turn two.
const claudeThinkingTurnTwoJSON = `{"id":"msg_turn2","type":"message","role":"assistant","model":"claude-opus-5-5",` +
	`"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":30,"output_tokens":2}}`

// fakeClaudeThinkingUpstream records request bodies and answers the two turns.
type fakeClaudeThinkingUpstream struct {
	mu     sync.Mutex
	bodies [][]byte
}

// ServeHTTP answers call N with turn N, streaming when the request asks for it.
func (u *fakeClaudeThinkingUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	u.bodies = append(u.bodies, body)
	turn := len(u.bodies)
	u.mu.Unlock()
	var probe struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	if !probe.Stream {
		w.Header().Set("Content-Type", "application/json")
		if turn == 1 {
			_, _ = io.WriteString(w, claudeThinkingTurnOneJSON)
		} else {
			_, _ = io.WriteString(w, claudeThinkingTurnTwoJSON)
		}
		return
	}
	events := claudeThinkingTurnOneSSE
	if turn > 1 {
		events = claudeThinkingTurnTwoSSE
	}
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	for _, event := range events {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// runClaudeThinkingFallbackTurn sends one Responses request through the
// Anthropic fallback channel and returns the recorded downstream response.
func runClaudeThinkingFallbackTurn(t *testing.T, upstreamURL string, channelType, channelID int, payload []byte, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fallback-key")
	c.Request = req
	gmw.SetLogger(c, logger.Logger)
	var requestModel struct {
		Model string `json:"model"`
	}
	require.NoError(t, json.Unmarshal(payload, &requestModel))
	c.Set(ctxkey.Channel, channelType)
	c.Set(ctxkey.ChannelId, channelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, requestModel.Model)
	c.Set(ctxkey.BaseURL, upstreamURL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, requestID)
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Quota: 1_000_000})
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: channelID, Type: channelType})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	drainResponseFallbackBilling(t)
	require.Nil(t, apiErr, "fallback turn failed: %s", recorder.Body.String())
	require.Equal(t, http.StatusOK, recorder.Code)
	requireResponsesRecorderConformance(t, recorder)
	return recorder
}

// prepareClaudeThinkingFallbackTest installs fixtures, disables Redis and
// consume logs, and routes the shared HTTP client to upstream.
func prepareClaudeThinkingFallbackTest(t *testing.T, upstream *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })
	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })
	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })
}

// codexReplayItems returns turn-one output items in the order Codex records
// them: response.output_item.done order for SSE, response.output for JSON.
func codexReplayItems(t *testing.T, recorder *httptest.ResponseRecorder, stream bool) []map[string]any {
	t.Helper()
	var items []map[string]any
	if !stream {
		var response struct {
			Output []map[string]any `json:"output"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Output
	}
	for _, event := range parseSSEEvents(recorder.Body.String()) {
		if event.event != "response.output_item.done" {
			continue
		}
		var decoded struct {
			Item map[string]any `json:"item"`
		}
		require.NoError(t, json.Unmarshal([]byte(event.data), &decoded))
		items = append(items, decoded.Item)
	}
	return items
}

// TestResponseFallbackClaudeThinkingTwoTurnReplay drives a Codex-style tool
// loop through the Claude fallback: turn one returns omitted-display thinking
// (empty text, signature sig1) plus a tool_use; turn two is built from the
// turn-one output items exactly as Codex replays them and must send the
// thinking block, signature sig1 and empty text intact, before the tool_use.
func TestResponseFallbackClaudeThinkingTwoTurnReplay(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			fake := &fakeClaudeThinkingUpstream{}
			upstream := httptest.NewServer(fake)
			t.Cleanup(upstream.Close)
			prepareClaudeThinkingFallbackTest(t, upstream)

			user := map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "list files"}}}
			tools := []any{map[string]any{"type": "function", "name": "shell", "parameters": map[string]any{"type": "object", "properties": map[string]any{"cmd": map[string]any{"type": "string"}}}}}
			turnOne, err := json.Marshal(map[string]any{
				"model": "claude-opus-5-5", "stream": stream, "store": false,
				"include": []string{"reasoning.encrypted_content"},
				"tools":   tools, "input": []any{user},
			})
			require.NoError(t, err)
			first := runClaudeThinkingFallbackTurn(t, upstream.URL, channeltype.Anthropic, fallbackAnthropicChannelID, turnOne, fmt.Sprintf("req_claude_thinking_1_%v", stream))

			items := codexReplayItems(t, first, stream)
			var reasoning, call map[string]any
			for _, item := range items {
				switch item["type"] {
				case "reasoning":
					reasoning = item
				case "function_call":
					call = item
				}
			}
			require.NotNil(t, reasoning, "omitted-display thinking must still produce a reasoning item")
			require.NotNil(t, call)
			require.Equal(t, []any{}, reasoning["summary"])
			envelope, _ := reasoning["encrypted_content"].(string)
			block, err := relaymodel.DecodeClaudeThinkingEnvelope(envelope)
			require.NoError(t, err)
			require.Equal(t, relaymodel.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"}, block)
			if stream {
				require.Equal(t, "reasoning", items[0]["type"], "Codex must record the thinking before the call")
			}

			replay := []any{user}
			for _, item := range items {
				replay = append(replay, item)
			}
			replay = append(replay, map[string]any{"type": "function_call_output", "call_id": call["call_id"], "output": "file.txt"})
			turnTwo, err := json.Marshal(map[string]any{
				"model": "claude-opus-5-5", "stream": stream, "store": false,
				"include": []string{"reasoning.encrypted_content"},
				"tools":   tools, "input": replay,
			})
			require.NoError(t, err)
			second := runClaudeThinkingFallbackTurn(t, upstream.URL, channeltype.Anthropic, fallbackAnthropicChannelID, turnTwo, fmt.Sprintf("req_claude_thinking_2_%v", stream))
			require.Contains(t, second.Body.String(), "done")

			fake.mu.Lock()
			bodies := append([][]byte(nil), fake.bodies...)
			fake.mu.Unlock()
			require.Len(t, bodies, 2)
			var upstreamRequest struct {
				Messages []struct {
					Role    string            `json:"role"`
					Content []json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(bodies[1], &upstreamRequest))
			require.Len(t, upstreamRequest.Messages, 3, "user, assistant turn, tool result: %s", bodies[1])
			assistant := upstreamRequest.Messages[1]
			require.Equal(t, "assistant", assistant.Role)
			require.Len(t, assistant.Content, 2, "thinking then tool_use: %s", bodies[1])
			require.JSONEq(t, `{"type":"thinking","thinking":"","signature":"sig1"}`, string(assistant.Content[0]))
			var toolUse struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}
			require.NoError(t, json.Unmarshal(assistant.Content[1], &toolUse))
			require.Equal(t, "tool_use", toolUse.Type)
			var toolResult struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
			}
			require.Equal(t, "user", upstreamRequest.Messages[2].Role)
			require.NoError(t, json.Unmarshal(upstreamRequest.Messages[2].Content[0], &toolResult))
			require.Equal(t, "tool_result", toolResult.Type)
			require.Equal(t, toolUse.ID, toolResult.ToolUseID)
			require.NotContains(t, string(bodies[1]), "oneapi_claude_thinking_blocks")
			require.NotContains(t, string(bodies[1]), "<think>")
		})
	}
}

// TestResponseFallbackClaudeEnvelopeNeverReachesChatUpstream verifies a
// replayed Claude envelope is not forwarded to an OpenAI-compatible upstream.
func TestResponseFallbackClaudeEnvelopeNeverReachesChatUpstream(t *testing.T) {
	var captured []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
	}))
	t.Cleanup(upstream.Close)
	prepareClaudeThinkingFallbackTest(t, upstream)

	envelope, err := relaymodel.EncodeClaudeThinkingEnvelope(relaymodel.ClaudeThinkingBlock{Type: "thinking", Thinking: "secret plan", Signature: "sig1"})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"model": "gpt-4o-mini", "store": false,
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "q"},
			map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}, "encrypted_content": envelope},
			map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "a"}}},
			map[string]any{"type": "message", "role": "user", "content": "again"},
		},
	})
	require.NoError(t, err)
	recorder := runClaudeThinkingFallbackTurn(t, upstream.URL, channeltype.OpenAICompatible, fallbackCompatibleChannelID, payload, "req_claude_envelope_chat")
	var response openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotEmpty(t, captured)
	require.NotContains(t, string(captured), "oneapi_claude_thinking_blocks")
	require.NotContains(t, string(captured), "sig1")
	require.NotContains(t, string(captured), relaymodel.ClaudeThinkingEnvelopePrefix)
}
