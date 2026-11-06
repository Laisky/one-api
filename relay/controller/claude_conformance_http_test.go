package controller

import (
	"encoding/json"
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

// claudeConformanceUpstream describes one non-Anthropic upstream family that a
// /v1/messages request can be routed to.
type claudeConformanceUpstream struct {
	name        string
	channelType int
	actualModel string
	pathMarker  string
	scenarios   []claudeConformanceScenario
	stream      func(claudeConformanceScenario) string
	nonStream   func(claudeConformanceScenario) string
	// streamCost and nonStreamCost pin the settled request cost so the wire
	// fixes are proven not to move billing. Responses streams settle from the
	// usage receipt of their terminal event, like every other upstream here.
	streamCost, nonStreamCost int64
}

// claudeConformanceUpstreams lists the converted upstream families under test.
// Gemini's native pipeline does not request thought parts, so it has no
// reasoning scenario.
func claudeConformanceUpstreams() []claudeConformanceUpstream {
	all := []claudeConformanceScenario{conformanceText, conformanceTools, conformanceLength, conformanceReasoning}
	return []claudeConformanceUpstream{
		{name: "openai_chat", channelType: channeltype.OpenAICompatible, actualModel: "gpt-4o-mini", pathMarker: "/v1/chat/completions", scenarios: all, stream: conformanceChatStream, nonStream: conformanceChatJSON, streamCost: 11, nonStreamCost: 11},
		{name: "openai_responses", channelType: channeltype.OpenAI, actualModel: "gpt-4o", pathMarker: "/v1/responses", scenarios: all, stream: conformanceResponsesStream, nonStream: conformanceResponsesObject, streamCost: 11, nonStreamCost: 11},
		{name: "gemini", channelType: channeltype.Gemini, actualModel: "gemini-2.5-flash", pathMarker: "enerateContent", scenarios: all[:3], stream: conformanceGeminiStream, nonStream: conformanceGeminiJSON, streamCost: 11, nonStreamCost: 11},
	}
}

// claudeConformanceRequest is the Claude Messages request every scenario sends;
// it declares both tools so tool names round-trip through the sanitizer.
func claudeConformanceRequest(stream bool) string {
	body, _ := json.Marshal(map[string]any{
		"model": "alias", "max_tokens": 64, "stream": stream,
		"messages": []any{map[string]any{"role": "user", "content": "What is the weather in Paris and the time in CET?"}},
		"tools": []any{
			map[string]any{"name": "get_weather", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}},
			map[string]any{"name": "get_time", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"tz": map[string]any{"type": "string"}}}},
		},
	})
	return string(body)
}

// claudeConformanceResult is one driven /v1/messages exchange.
type claudeConformanceResult struct {
	report claudeconformance.Report
	body   string
	cost   int64
}

// runClaudeConformance drives one real /v1/messages request through
// RelayClaudeMessagesHelper against an httptest upstream and validates the
// downstream wire bytes with the shared Anthropic conformance validator.
func runClaudeConformance(t *testing.T, up claudeConformanceUpstream, upstreamBody string, stream bool) claudeConformanceResult {
	t.Helper()
	const balance int64 = 10_000_000
	xaiVideoSetup(t, balance, false)
	paths := make(chan string, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case paths <- r.URL.Path:
		default:
		}
		_, _ = io.Copy(io.Discard, r.Body)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = io.WriteString(w, upstreamBody)
	}))
	t.Cleanup(upstream.Close)
	old := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = old })

	c, w, id := protocolContext(t, up.channelType, up.actualModel, "/v1/messages", claudeConformanceRequest(stream), upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	require.Nil(t, RelayClaudeMessagesHelper(c))
	drainCriticalTasks(t)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, <-paths, up.pathMarker, "request must reach the intended upstream format")

	body := w.Body.String()
	var report claudeconformance.Report
	if stream {
		require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		report = claudeconformance.ValidateStream([]byte(body))
	} else {
		require.Contains(t, w.Header().Get("Content-Type"), "application/json")
		report = claudeconformance.ValidateMessage([]byte(body))
	}
	return claudeConformanceResult{report: report, body: body, cost: requestCostQuota(t, id)}
}

// requireClaudeConformanceSemantics checks that the converted message carries
// the upstream's answer exactly once with the mapped stop_reason and the
// upstream's usage receipt.
func requireClaudeConformanceSemantics(t *testing.T, s claudeConformanceScenario, msg claudeconformance.Message) {
	t.Helper()
	switch s {
	case conformanceText:
		require.Equal(t, "Hello world", msg.Text())
		require.Equal(t, "end_turn", msg.StopReason)
		require.Empty(t, msg.ToolUses())
	case conformanceLength:
		require.Equal(t, "Hello world", msg.Text())
		require.Equal(t, "max_tokens", msg.StopReason)
	case conformanceReasoning:
		require.NotEmpty(t, msg.Content)
		require.Equal(t, "thinking", msg.Content[0].Type)
		require.Equal(t, "Thinking hard", msg.Content[0].Thinking)
		require.Equal(t, "Hello world", msg.Text())
		require.Equal(t, "end_turn", msg.StopReason)
	case conformanceTools:
		tools := msg.ToolUses()
		require.Len(t, tools, 2)
		require.Equal(t, "get_weather", tools[0].Name)
		require.JSONEq(t, `{"city":"Paris"}`, tools[0].Input)
		require.Equal(t, "get_time", tools[1].Name)
		require.JSONEq(t, `{"tz":"CET"}`, tools[1].Input)
		require.NotEqual(t, tools[0].ID, tools[1].ID)
		require.Equal(t, "tool_use", msg.StopReason)
		require.Empty(t, msg.Text())
	}
	require.NotNil(t, msg.Usage)
	require.EqualValues(t, 9, msg.Usage["input_tokens"])
	require.EqualValues(t, 2, msg.Usage["output_tokens"])
}

// TestClaudeConformanceConvertedUpstreams drives stream and non-stream
// /v1/messages requests against OpenAI Chat, OpenAI Responses, and Gemini
// upstreams and requires every downstream payload to satisfy the Anthropic
// Messages wire contract that Claude Code and the Anthropic SDKs enforce.
func TestClaudeConformanceConvertedUpstreams(t *testing.T) {
	for _, up := range claudeConformanceUpstreams() {
		for _, stream := range []bool{true, false} {
			mode := "nonstream"
			if stream {
				mode = "stream"
			}
			for _, s := range up.scenarios {
				t.Run(up.name+"/"+mode+"/"+string(s), func(t *testing.T) {
					upstreamBody := up.nonStream(s)
					if stream {
						upstreamBody = up.stream(s)
					}
					result := runClaudeConformance(t, up, upstreamBody, stream)
					t.Logf("CLAUDE_CONFORMANCE upstream=%s mode=%s scenario=%s cost=%d rules=%v", up.name, mode, s, result.cost, result.report.Rules())
					require.Empty(t, result.report.Violations, "wire violations:\n%s\nbody:\n%s", formatViolations(result.report.Violations), result.body)
					requireClaudeConformanceSemantics(t, s, result.report.Message)
					require.NotContains(t, result.body, "[DONE]")
					wantCost := up.nonStreamCost
					if stream {
						wantCost = up.streamCost
					}
					require.Equal(t, wantCost, result.cost, "converted presentation must not change billing")
				})
			}
		}
	}
}

// formatViolations renders violations one per line for failure messages.
func formatViolations(violations []claudeconformance.Violation) string {
	lines := make([]string, 0, len(violations))
	for _, v := range violations {
		lines = append(lines, "  - "+v.String())
	}
	return strings.Join(lines, "\n")
}
