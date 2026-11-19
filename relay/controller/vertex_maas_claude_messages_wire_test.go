package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// vertexMaaSClaudeFixture describes a Vertex managed open model served over
// the OpenAI-compatible MaaS chat endpoint. Parameters: name labels the case
// and modelName is the Vertex model ID. Returns: the fixture.
func vertexMaaSClaudeFixture(name, modelName string) legacyClaudeFixture {
	const chatPath = "/endpoints/openapi/chat/completions"
	return legacyClaudeFixture{
		name: name, channel: channeltype.VertextAI, model: modelName, key: "vertex-fixture-key",
		config:   model.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject},
		chatPath: chatPath, receiptUsage: true,
		setup: func(t *testing.T) {
			key := fmt.Sprintf("vertexai-token-%d", fallbackChannelID)
			vertexai.Cache.Set(key, providerMatrixVertexToken, time.Minute)
			t.Cleanup(func() { vertexai.Cache.Delete(key) })
		},
		reply: func(stream bool, r *http.Request) legacyWireReply {
			if !strings.HasSuffix(r.URL.Path, chatPath) {
				return legacyWireReply{Status: http.StatusNotFound, ContentType: "application/json", Body: `{"error":"unexpected path"}`}
			}
			if stream {
				return legacyWireReply{ContentType: "text/event-stream", Body: legacyOpenAIChatSSE(modelName)}
			}
			return legacyWireReply{ContentType: "application/json", Body: legacyOpenAIChatJSON(modelName)}
		},
		wire: func(t *testing.T, stream bool, body map[string]any) {
			require.Equal(t, modelName, body["model"])
			require.EqualValues(t, 64, body["max_tokens"], "Vertex MaaS takes max_tokens: %v", body)
			require.NotContains(t, body, "max_completion_tokens")
			require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["messages"],
				"an absent Claude system prompt must not become an empty system message: %v", body)
		},
	}
}

// TestVertexMaaSClaudeMessagesWireContract drives Claude Messages requests to
// Vertex managed open models (GPT-OSS, Qwen) through RelayClaudeMessagesHelper.
// The MaaS endpoint answers in the OpenAI Chat wire format, so the Claude
// client must receive that answer converted to Claude Messages, and the Chat
// Completions control must bill the same receipt.
func TestVertexMaaSClaudeMessagesWireContract(t *testing.T) {
	for _, fx := range []legacyClaudeFixture{
		vertexMaaSClaudeFixture("vertex_gptoss", "openai/gpt-oss-20b-maas"),
		vertexMaaSClaudeFixture("vertex_qwen", "qwen/qwen3-next-80b-a3b-instruct-maas"),
	} {
		runLegacyClaudeContract(t, fx)
	}
}

// TestVertexMaaSClaudeToolHistoryWire sends a Claude Messages tool round trip
// (a structured system prompt, a tool declared without input_schema, an
// assistant tool_use and the user's tool_result) to Vertex GPT-OSS. The
// provider must receive the equivalent Chat Completions history rather than a
// request with the tool turns dropped, and the schema-less tool must not crash
// the conversion.
func TestVertexMaaSClaudeToolHistoryWire(t *testing.T) {
	const modelName = "openai/gpt-oss-20b-maas"
	fx := vertexMaaSClaudeFixture("vertex_gptoss_tools", modelName)
	const balance = int64(1_000_000)
	xaiVideoSetup(t, balance, false)
	fx.setup(t)
	upstream := newLegacyWireUpstream(t, func(r *http.Request) legacyWireReply { return fx.reply(false, r) })
	body := `{"model":"` + modelName + `","max_tokens":64,"system":[{"type":"text","text":"Be brief"}],` +
		`"tools":[{"name":"lookup","description":"Look up a term"}],` +
		`"messages":[{"role":"user","content":"Find x"},` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"x"}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"found"}]}]}`
	tariff := legacyClaudeTariff
	c, writer, requestID := protocolContext(t, fx.channel, modelName, "/v1/messages", body, upstream.server.URL, balance, 1, false, &tariff)
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.RequestModel, modelName)
	c.Set(ctxkey.Config, fx.config)
	clearNativeSettledLedger(t, requestID)

	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr, "the tool round trip must be relayed: %+v", apiErr)
	requireClaudeMessageJSON(t, writer)
	requests := upstream.recorded()
	require.Len(t, requests, 1)

	var wire struct {
		Messages []map[string]any `json:"messages"`
		Tools    []map[string]any `json:"tools"`
	}
	require.NoError(t, json.Unmarshal([]byte(requests[0].Body), &wire))
	require.Len(t, wire.Messages, 4, "system, user, assistant tool call and tool result must all reach the provider: %s", requests[0].Body)
	require.Equal(t, map[string]any{"role": "system", "content": "Be brief"}, wire.Messages[0])
	require.Equal(t, "assistant", wire.Messages[2]["role"])
	calls, _ := wire.Messages[2]["tool_calls"].([]any)
	require.Len(t, calls, 1, "the assistant tool_use must become a tool call: %s", requests[0].Body)
	call, _ := calls[0].(map[string]any)
	require.Equal(t, "toolu_1", call["id"])
	function, _ := call["function"].(map[string]any)
	require.Equal(t, "lookup", function["name"])
	require.JSONEq(t, `{"q":"x"}`, function["arguments"].(string))
	require.Equal(t, "tool", wire.Messages[3]["role"])
	require.Equal(t, "toolu_1", wire.Messages[3]["tool_call_id"])
	require.Equal(t, "found", wire.Messages[3]["content"])
	require.Len(t, wire.Tools, 1)
	declared, _ := wire.Tools[0]["function"].(map[string]any)
	require.Equal(t, "lookup", declared["name"])
	require.Equal(t, map[string]any{}, declared["parameters"], "a tool without input_schema takes no parameters")
}
