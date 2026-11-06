package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// codexFallbackTools is a synthetic Codex 0.166 tool list: a plain function, a
// lark-grammar custom tool, the collaboration namespace, an app-connector
// namespace whose child starts with "_", and client-executed tool search.
const codexFallbackTools = `[
	{"type":"function","name":"exec_command","description":"Run a shell command","strict":false,"parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"],"additionalProperties":false}},
	{"type":"custom","name":"apply_patch","description":"Use apply_patch to edit files. This is a FREEFORM tool.","format":{"type":"grammar","syntax":"lark","definition":"start: begin_patch hunk+ end_patch\nbegin_patch: \"*** Begin Patch\" LF"}},
	{"type":"namespace","name":"collaboration","description":"Multi-agent collaboration","tools":[
		{"type":"function","name":"spawn_agent","description":"Spawn a worker agent","parameters":{"type":"object","properties":{"task":{"type":"string"}},"required":["task"]}},
		{"type":"function","name":"wait_agent","description":"Wait for a worker agent","parameters":{"type":"object","properties":{"id":{"type":"string"}}}}
	]},
	{"type":"namespace","name":"mcp__codex_apps__github","description":"GitHub connector","tools":[
		{"type":"function","name":"_create_issue","description":"Create an issue","parameters":{"type":"object","properties":{"title":{"type":"string"}}}}
	]},
	{"type":"tool_search","execution":"client","description":"# Tool discovery\nSearch deferred tools.","parameters":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"number"}},"required":["query"],"additionalProperties":false}}
]`

// codexFallbackInput is synthetic Codex history covering client tool search,
// a loaded MCP namespace call, a custom tool call, the multi-agent turn shape
// [call A, call B, assistant text, output A, output B], an agent message and a
// configuration update. The search limit keeps a literal beyond float64 range.
const codexFallbackInput = `[
	{"type":"message","role":"user","content":[{"type":"input_text","text":"Read the docs and patch the README."}]},
	{"type":"tool_search_call","id":"tsc_1","call_id":"call_s1","execution":"client","status":"completed","arguments":{"query":"docs","limit":8,"cursor":9007199254740993}},
	{"type":"tool_search_output","id":"tso_1","call_id":"call_s1","status":"completed","execution":"client","tools":[{"type":"namespace","name":"mcp__docs","description":"Docs server","tools":[{"type":"function","name":"read_doc","description":"Read one document","defer_loading":true,"parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}]}]},
	{"type":"function_call","id":"call_x","call_id":"call_x","namespace":"mcp__docs","name":"read_doc","arguments":"{\"path\":\"a\"}"},
	{"type":"function_call_output","id":"fco_1","call_id":"call_x","output":"doc text"},
	{"type":"custom_tool_call","id":"call_p","call_id":"call_p","name":"apply_patch","input":"*** Begin Patch\n*** Add File: a.txt\n+a\n*** End Patch\n"},
	{"type":"custom_tool_call_output","call_id":"call_p","output":"Success"},
	{"type":"function_call","id":"call_a","call_id":"call_a","namespace":"collaboration","name":"spawn_agent","arguments":"{\"task\":\"review\"}"},
	{"type":"function_call","id":"call_b","call_id":"call_b","namespace":"collaboration","name":"wait_agent","arguments":"{\"id\":\"w1\"}"},
	{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Spawning a reviewer."}]},
	{"type":"function_call_output","call_id":"call_a","output":"spawned w1"},
	{"type":"function_call_output","call_id":"call_b","output":"w1 finished"},
	{"type":"agent_message","author":"/root","recipient":"/root/worker","content":[{"type":"input_text","text":"hi"}]},
	{"type":"configuration_update","reasoning":{"effort":"high"}}
]`

// codexFallbackPatch is the raw custom-tool input the fake upstream returns.
const codexFallbackPatch = "*** Begin Patch\n*** Add File: b.txt\n+b\n*** End Patch\n"

// codexFallbackFlatTools lists the flattened upstream tool names in catalog order.
var codexFallbackFlatTools = []string{
	"exec_command", "apply_patch", "collaboration__spawn_agent", "collaboration__wait_agent",
	"mcp__codex_apps__github_create_issue", "tool_search", "mcp__docs__read_doc",
}

// codexFallbackUpstreamCall is one tool call the fake upstream emits: its
// provider call ID, flattened name and complete JSON argument text.
type codexFallbackUpstreamCall struct {
	id, name, arguments string
}

// codexFallbackUpstreamCalls returns the fake upstream's tool calls for one
// channel; Claude tool IDs use the toolu_ prefix. The search limit is 8.0 so
// rendering must normalize it to the integer literal Codex parses.
func codexFallbackUpstreamCalls(channel int) []codexFallbackUpstreamCall {
	prefix := "call_"
	if channel == channeltype.Anthropic {
		prefix = "toolu_"
	}
	return []codexFallbackUpstreamCall{
		{prefix + "doc", "mcp__docs__read_doc", `{"path":"b"}`},
		{prefix + "spawn", "collaboration__spawn_agent", `{"task":"fix"}`},
		{prefix + "issue", "mcp__codex_apps__github_create_issue", `{"title":"bug"}`},
		{prefix + "exec", "exec_command", `{"cmd":"ls"}`},
		{prefix + "patch", "apply_patch", mustCodexJSON(map[string]string{"input": codexFallbackPatch})},
		{prefix + "search", "tool_search", `{"query":"calendar","limit":8.0}`},
	}
}

// codexFallbackRenderedCallID returns the Responses call ID rendered for a
// provider tool call ID, mirroring ensureResponseAPICallID.
func codexFallbackRenderedCallID(id string) string {
	return ensureResponseAPICallID(id)
}

// mustCodexJSON encodes value for fixtures; encoding a string map cannot fail.
func mustCodexJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// codexFallbackObservation records what the loopback provider received and the
// quota held at dispatch, without calling testing.Fatal off the test goroutine.
type codexFallbackObservation struct {
	Path        string
	Body        json.RawMessage
	User, Token int64
	Err         error
}

// codexFallbackRequestBody builds the client Responses request. Parameters:
// stream selects SSE. Returns the JSON body.
func codexFallbackRequestBody(stream bool) string {
	return fmt.Sprintf(`{"model":"alias","stream":%t,"max_output_tokens":64,"tools":%s,"input":%s}`, stream, codexFallbackTools, codexFallbackInput)
}

// codexFallbackProvider starts a TLS loopback upstream that records the request
// and answers with the channel's wire format carrying codexFallbackUpstreamCalls
// and usage of 11 prompt and 7 completion tokens. Parameters: t owns cleanup,
// channel picks Chat or Claude framing and stream selects SSE. Returns the
// server, the observation channel and the dispatch counter.
func codexFallbackProvider(t *testing.T, channel int, stream bool) (*httptest.Server, chan codexFallbackObservation, *atomic.Int32) {
	t.Helper()
	observed := make(chan codexFallbackObservation, 1)
	calls := &atomic.Int32{}
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		got := codexFallbackObservation{Path: r.URL.Path}
		got.Body, got.Err = io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
		var user model.User
		var token model.Token
		if got.Err == nil {
			got.Err = model.DB.First(&user, fallbackUserID).Error
		}
		if got.Err == nil {
			got.Err = model.DB.First(&token, fallbackTokenID).Error
		}
		got.User, got.Token = user.Quota, token.RemainQuota
		select {
		case observed <- got:
		default:
		}
		writeCodexFallbackReply(w, channel, stream)
	}))
	t.Cleanup(provider.Close)
	previous := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	return provider, observed, calls
}

// writeCodexFallbackReply writes the fake upstream reply. Parameters: w is the
// response writer, channel picks Chat or Claude framing, stream selects SSE.
func writeCodexFallbackReply(w http.ResponseWriter, channel int, stream bool) {
	upstream := codexFallbackUpstreamCalls(channel)
	if channel == channeltype.Anthropic {
		if !stream {
			blocks := []string{`{"type":"text","text":"Working."}`}
			for _, call := range upstream {
				blocks = append(blocks, fmt.Sprintf(`{"type":"tool_use","id":%q,"name":%q,"input":%s}`, call.id, call.name, call.arguments))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":"msg_codex","type":"message","role":"assistant","model":"codex-fallback-fixture","content":[%s],"stop_reason":"tool_use","usage":{"input_tokens":11,"output_tokens":7}}`, strings.Join(blocks, ","))
			return
		}
		events := []string{
			`{"type":"message_start","message":{"id":"msg_codex","type":"message","role":"assistant","model":"codex-fallback-fixture","content":[],"usage":{"input_tokens":11,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Working."}}`,
			`{"type":"content_block_stop","index":0}`,
		}
		for i, call := range upstream {
			index := i + 1
			half := len(call.arguments) / 2
			events = append(events,
				fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, index, call.id, call.name),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, index, mustCodexJSON(call.arguments[:half])),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, index, mustCodexJSON(call.arguments[half:])),
				fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, index),
			)
		}
		events = append(events,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
			`{"type":"message_stop"}`,
		)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			var envelope struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(event), &envelope)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", envelope.Type, event)
		}
		return
	}
	if !stream {
		calls := make([]string, 0, len(upstream))
		for _, call := range upstream {
			calls = append(calls, fmt.Sprintf(`{"id":%q,"type":"function","function":{"name":%q,"arguments":%s}}`, call.id, call.name, mustCodexJSON(call.arguments)))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"chatcmpl_codex","object":"chat.completion","created":1762387200,"model":"codex-fallback-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"Working.","tool_calls":[%s]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, strings.Join(calls, ","))
		return
	}
	chunk := func(delta, finish, usage string) string {
		return fmt.Sprintf(`{"id":"chatcmpl_codex","object":"chat.completion.chunk","created":1762387200,"model":"codex-fallback-fixture","choices":[{"index":0,"delta":%s,"finish_reason":%s}]%s}`, delta, finish, usage)
	}
	chunks := []string{chunk(`{"role":"assistant","content":"Working."}`, "null", "")}
	for i, call := range upstream {
		half := len(call.arguments) / 2
		chunks = append(chunks,
			chunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"id":%q,"type":"function","function":{"name":%q,"arguments":""}}]}`, i, call.id, call.name), "null", ""),
			chunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"function":{"arguments":%s}}]}`, i, mustCodexJSON(call.arguments[:half])), "null", ""),
			chunk(fmt.Sprintf(`{"tool_calls":[{"index":%d,"function":{"arguments":%s}}]}`, i, mustCodexJSON(call.arguments[half:])), "null", ""),
		)
	}
	chunks = append(chunks, chunk(`{}`, `"tool_calls"`, `,"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}`))
	w.Header().Set("Content-Type", "text/event-stream")
	for _, data := range chunks {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

// codexFallbackRun relays the synthetic Codex request through the fallback
// route of channel against a fresh loopback provider. Parameters: t owns the
// fixtures, channel is OpenAICompatible or Anthropic, stream selects SSE and
// balance seeds user and token quota. Returns the handled context, recorder,
// request ID, the provider observation and the dispatch counter.
func codexFallbackRun(t *testing.T, channel int, stream bool, balance int64) (*gin.Context, *httptest.ResponseRecorder, string, codexFallbackObservation, *atomic.Int32) {
	t.Helper()
	xaiVideoSetup(t, balance, false)
	provider, observed, calls := codexFallbackProvider(t, channel, stream)
	c, recorder, requestID := protocolContext(t, channel, "codex-fallback-fixture", "/v1/responses", codexFallbackRequestBody(stream), provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
	responseNamespaceCleanLedger(t, requestID)
	require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "the channel must take the Chat/Claude fallback")
	apiErr := RelayResponseAPIHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr, "fallback must emulate Codex tools instead of rejecting them")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, calls.Load())
	got := <-observed
	require.NoError(t, got.Err)
	require.True(t, metalib.GetByContext(c).ResponseAPIFallback)
	return c, recorder, requestID, got, calls
}

// requireFallbackTokenSettlement asserts the ordinary token-only settlement of
// one fallback request priced at 1 quota per prompt token and 2 per completion
// token: 11 prompt + 7*2 completion = 25, with no hosted-tool fee. Parameters:
// t owns assertions, c is the handled request, requestID selects the ledger
// rows, balance is the seeded quota and user/token are the balances the
// provider observed at dispatch, which must already exclude the reservation.
func requireFallbackTokenSettlement(t *testing.T, c *gin.Context, requestID string, balance, user, token int64) {
	t.Helper()
	held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
	require.Positive(t, held, "fallback must reserve quota before dispatch")
	require.Equal(t, balance-held, user)
	require.Equal(t, balance-held, token)
	require.EqualValues(t, 25, requestCostQuota(t, requestID))
	require.EqualValues(t, 25, consumeLogQuota(t, requestID))
	require.Equal(t, balance-25, reloadUserQuota(t))
	var settled model.Token
	require.NoError(t, model.DB.First(&settled, fallbackTokenID).Error)
	require.Equal(t, balance-25, settled.RemainQuota)
	require.EqualValues(t, 25, settled.UsedQuota)
}
