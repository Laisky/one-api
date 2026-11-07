package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/common/claudeconformance"
	"github.com/Laisky/one-api/relay/channeltype"
)

// claudeUsageReceipt is one upstream usage receipt in OpenAI-family semantics:
// prompt counts every input token, including the cached reads and the cache
// writes it also reports separately.
type claudeUsageReceipt struct {
	name                                string
	prompt, cached, write5m, completion int
}

// cost is the settled quota under claudeUsagePricing: uncached input at 1,
// cache reads at 0.25, 5m cache writes at 1.25, and output at 1 per token.
func (r claudeUsageReceipt) cost() int64 {
	uncached := r.prompt - r.cached - r.write5m
	return int64(uncached) + int64(r.cached)/4 + int64(r.write5m)*5/4 + int64(r.completion)
}

// claudeUsagePricing prices cache reads and writes distinctly from fresh input
// so a lost or double-counted cache bucket moves the settled cost.
var claudeUsagePricing = model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, CachedInputRatio: 0.25, CacheWrite5mRatio: 1.25}

// claudeUsageReceipts lists the receipts driven through every upstream. The
// light receipts keep the cached prefix below the uncached remainder: billing
// re-derived from their Anthropic-semantics body would then be read as OpenAI
// semantics and under-charge, so they pin billing to the server-only receipt.
var claudeUsageReceipts = []claudeUsageReceipt{
	{name: "uncached", prompt: 1000, completion: 40},
	{name: "cached", prompt: 1000, cached: 800, completion: 40},
	{name: "cached_write", prompt: 1000, cached: 600, write5m: 200, completion: 40},
	{name: "light_cache", prompt: 1000, cached: 200, completion: 40},
	{name: "light_cache_write", prompt: 1000, cached: 200, write5m: 100, completion: 40},
}

// claudeUsageUpstream is one non-Anthropic upstream family serving /v1/messages.
type claudeUsageUpstream struct {
	name        string
	channelType int
	actualModel string
	pathMarker  string
	// writes reports whether the upstream wire format can carry cache writes.
	writes bool
}

// claudeUsageUpstreams covers the OpenAI adaptor over Chat (OpenAI-compatible
// channel) and Responses (OpenAI channel), the shared openai_compatible
// converter (Groq), and the Gemini adaptor.
func claudeUsageUpstreams() []claudeUsageUpstream {
	return []claudeUsageUpstream{
		{name: "openai_chat", channelType: channeltype.OpenAICompatible, actualModel: "gpt-4o-mini", pathMarker: "/chat/completions", writes: true},
		{name: "openai_responses", channelType: channeltype.OpenAI, actualModel: "gpt-4o", pathMarker: "/responses", writes: true},
		{name: "openai_compatible", channelType: channeltype.Groq, actualModel: "llama-3.3-70b-versatile", pathMarker: "/chat/completions", writes: true},
		{name: "gemini", channelType: channeltype.Gemini, actualModel: "gemini-2.5-flash", pathMarker: "enerateContent"},
	}
}

// claudeUsageChatUsage renders the Chat Completions usage object of r.
func claudeUsageChatUsage(r claudeUsageReceipt) string {
	usage := fmt.Sprintf(`{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}`, r.prompt, r.completion, r.prompt+r.completion, r.cached)
	if r.write5m > 0 {
		usage += fmt.Sprintf(`,"cache_write_5m_tokens":%d`, r.write5m)
	}
	return usage + "}"
}

// claudeUsageResponsesObject renders a terminal Responses object carrying r.
func claudeUsageResponsesObject(r claudeUsageReceipt) string {
	usage := fmt.Sprintf(`{"input_tokens":%d,"input_tokens_details":{"cached_tokens":%d},"output_tokens":%d,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":%d`, r.prompt, r.cached, r.completion, r.prompt+r.completion)
	if r.write5m > 0 {
		usage += fmt.Sprintf(`,"cache_write_5m_tokens":%d`, r.write5m)
	}
	return `{"id":"resp_usage","object":"response","created_at":1,"status":"completed","incomplete_details":null,"model":"gpt-4o","output":` +
		conformanceResponsesOutput(conformanceText) + `,"usage":` + usage + "}}"
}

// claudeUsageGeminiMetadata renders the Gemini usageMetadata receipt of r.
func claudeUsageGeminiMetadata(r claudeUsageReceipt) string {
	return fmt.Sprintf(`"usageMetadata":{"promptTokenCount":%d,"candidatesTokenCount":%d,"totalTokenCount":%d,"cachedContentTokenCount":%d}`, r.prompt, r.completion, r.prompt+r.completion, r.cached)
}

// claudeUsageUpstreamBody returns the upstream answer "Hello world" carrying r
// in the wire format selected by the request path and stream mode.
func claudeUsageUpstreamBody(path string, stream bool, r claudeUsageReceipt) string {
	switch {
	case strings.Contains(path, "enerateContent"):
		if !stream {
			return `{"candidates":[{"content":{"parts":[{"text":"Hello world"}],"role":"model"},"finishReason":"STOP","index":0}],` + claudeUsageGeminiMetadata(r) + `,"modelVersion":"gemini-2.5-flash","responseId":"gem-usage"}`
		}
		return `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"index":0}],` + claudeUsageGeminiMetadata(r) + `,"modelVersion":"gemini-2.5-flash","responseId":"gem-usage"}` + "\r\n\r\n" +
			`data: {"candidates":[{"content":{"parts":[{"text":" world"}],"role":"model"},"index":0,"finishReason":"STOP"}],` + claudeUsageGeminiMetadata(r) + `,"modelVersion":"gemini-2.5-flash","responseId":"gem-usage"}` + "\r\n\r\n"
	case strings.Contains(path, "/responses"):
		if !stream {
			return claudeUsageResponsesObject(r)
		}
		var sb strings.Builder
		inProgress := `"response":{"id":"resp_usage","object":"response","created_at":1,"status":"in_progress","model":"gpt-4o","output":[],"usage":null}`
		sb.WriteString(conformanceResponsesEvent("response.created", `"sequence_number":0,`+inProgress))
		conformanceResponsesText(&sb, "0")
		sb.WriteString(conformanceResponsesEvent("response.completed", `"response":`+claudeUsageResponsesObject(r)))
		return sb.String()
	default:
		if !stream {
			return `{"id":"chatcmpl-usage","object":"chat.completion","created":1,"model":"upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hello world"},"finish_reason":"stop"}],"usage":` + claudeUsageChatUsage(r) + "}"
		}
		return conformanceChatChunk(`{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}`) +
			conformanceChatChunk(`{"index":0,"delta":{"content":" world"},"finish_reason":null}`) +
			conformanceChatChunk(`{"index":0,"delta":{},"finish_reason":"stop"}`) +
			`data: {"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1,"model":"upstream-model","choices":[],"usage":` + claudeUsageChatUsage(r) + "}\n\n" +
			"data: [DONE]\n\n"
	}
}

// claudeUsageLedger is the durable accounting of one settled request.
type claudeUsageLedger struct {
	body                          string
	cost, userDebit, tokenUsed    int64
	tokenRemain, logQuota, logOut int64
	logIn                         int64
}

// driveClaudeUsageRequest sends one request (Claude Messages or native Chat)
// through the real relay helper against an httptest upstream that answers with
// r, then reads the request cost, consume log, and user/token balances.
func driveClaudeUsageRequest(t *testing.T, up claudeUsageUpstream, r claudeUsageReceipt, stream, native bool) claudeUsageLedger {
	t.Helper()
	const balance int64 = 10_000_000
	xaiVideoSetup(t, balance, false)
	paths := make(chan string, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case paths <- req.URL.Path:
		default:
		}
		_, _ = io.Copy(io.Discard, req.Body)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = io.WriteString(w, claudeUsageUpstreamBody(req.URL.Path, stream, r))
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	path, body := "/v1/messages", claudeConformanceRequest(stream)
	if native {
		path = "/v1/chat/completions"
		encoded, err := json.Marshal(map[string]any{"model": "alias", "stream": stream, "messages": []any{map[string]any{"role": "user", "content": "What is the weather in Paris?"}}})
		require.NoError(t, err)
		body = string(encoded)
	}
	c, w, id := protocolContext(t, up.channelType, up.actualModel, path, body, upstream.URL, balance, 1, false, &claudeUsagePricing)
	// Request IDs derive from the test name, so -count reruns share them on the
	// shared SQLite fixture; start every drive from an empty request ledger.
	require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Delete(&model.Log{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", id).Delete(&model.UserRequestCost{}).Error)
	if native {
		require.Nil(t, RelayTextHelper(c))
	} else {
		require.Nil(t, RelayClaudeMessagesHelper(c))
	}
	drainCriticalTasks(t)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Contains(t, <-paths, up.pathMarker, "request must reach the intended upstream format")

	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
	require.Len(t, logs, 1, "one reconciled consume row owns the final debit")
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
	return claudeUsageLedger{
		body: w.Body.String(), cost: requestCostQuota(t, id), userDebit: balance - reloadUserQuota(t),
		tokenUsed: token.UsedQuota, tokenRemain: balance - token.RemainQuota,
		logQuota: int64(logs[0].Quota), logOut: int64(logs[0].CompletionTokens), logIn: int64(logs[0].PromptTokens),
	}
}

// claudeUsageClientView returns the usage a Claude SDK client assembles from
// the downstream payload: the Message usage, or message_start merged with the
// cumulative message_delta usage for a stream.
func claudeUsageClientView(t *testing.T, body string, stream bool) map[string]any {
	t.Helper()
	report := claudeconformance.ValidateMessage([]byte(body))
	if stream {
		report = claudeconformance.ValidateStream([]byte(body))
	}
	require.Empty(t, report.Violations, "wire violations:\n%s\nbody:\n%s", formatViolations(report.Violations), body)
	require.NotNil(t, report.Message.Usage)
	return report.Message.Usage
}

// claudeUsageCounter reads one numeric usage field, treating absence as zero.
func claudeUsageCounter(usage map[string]any, key string) int {
	value, _ := usage[key].(float64)
	return int(value)
}

// TestClaudeMessagesConvertedUsageSemantics drives stream and non-stream
// /v1/messages requests against non-Anthropic upstreams and requires (1) the
// client-facing usage to follow Anthropic semantics, so input_tokens plus the
// cache buckets equals the upstream prompt count instead of double-counting the
// cached prefix, and (2) the settled cost, consume log, and user and token
// ledgers to equal the native /v1/chat/completions billing of the same receipt.
func TestClaudeMessagesConvertedUsageSemantics(t *testing.T) {
	for _, up := range claudeUsageUpstreams() {
		for _, stream := range []bool{true, false} {
			mode := "nonstream"
			if stream {
				mode = "stream"
			}
			for _, receipt := range claudeUsageReceipts {
				if receipt.write5m > 0 && !up.writes {
					continue
				}
				t.Run(up.name+"/"+mode+"/"+receipt.name, func(t *testing.T) {
					messages := driveClaudeUsageRequest(t, up, receipt, stream, false)
					native := driveClaudeUsageRequest(t, up, receipt, stream, true)
					usage := claudeUsageClientView(t, messages.body, stream)
					input, read := claudeUsageCounter(usage, "input_tokens"), claudeUsageCounter(usage, "cache_read_input_tokens")
					creation := claudeUsageCounter(usage, "cache_creation_input_tokens")
					t.Logf("CLAUDE_USAGE_BILLING upstream=%s mode=%s receipt=%s messages_cost=%d native_cost=%d want=%d input=%d cache_read=%d cache_creation=%d output=%d",
						up.name, mode, receipt.name, messages.cost, native.cost, receipt.cost(), input, read, creation, claudeUsageCounter(usage, "output_tokens"))

					require.Equal(t, receipt.prompt, input+read+creation, "Claude clients size the context window from input_tokens plus both cache buckets")
					require.Equal(t, receipt.cached, read, "cache_read_input_tokens must equal the upstream cached count")
					require.Equal(t, receipt.write5m, creation, "cache_creation_input_tokens must equal the upstream cache writes")
					require.Equal(t, receipt.completion, claudeUsageCounter(usage, "output_tokens"))
					if receipt.write5m > 0 {
						creationSplit, ok := usage["cache_creation"].(map[string]any)
						require.True(t, ok, "cache writes must carry the TTL split object")
						require.EqualValues(t, receipt.write5m, creationSplit["ephemeral_5m_input_tokens"])
					}

					require.Equal(t, native.cost, messages.cost, "Claude Messages billing must equal native Chat billing for the same receipt")
					require.Equal(t, receipt.cost(), messages.cost)
					for _, ledger := range []claudeUsageLedger{messages, native} {
						require.Equal(t, ledger.cost, ledger.userDebit, "user quota debit")
						require.Equal(t, ledger.cost, ledger.tokenUsed, "token used quota")
						require.Equal(t, ledger.cost, ledger.tokenRemain, "token remaining quota debit")
						require.Equal(t, ledger.cost, ledger.logQuota, "consume log quota")
						require.EqualValues(t, receipt.completion, ledger.logOut, "consume log completion tokens")
						require.EqualValues(t, receipt.prompt, ledger.logIn, "consume log prompt tokens count the cached prefix once")
					}
				})
			}
		}
	}
}
