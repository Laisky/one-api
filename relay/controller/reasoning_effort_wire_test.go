package controller

import (
	"encoding/json"
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
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// effortSurface names one client protocol entry point of the relay.
type effortSurface string

const (
	effortChat      effortSurface = "/v1/chat/completions"
	effortResponses effortSurface = "/v1/responses"
	effortClaude    effortSurface = "/v1/messages"
)

// effortChatReply is a minimal chat completion returned by the fake upstream.
const effortChatReply = `{"id":"chatcmpl-effort","object":"chat.completion","created":1741036800,"model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`

// effortResponsesReply is a minimal native Responses API object returned by
// the fake upstream for providers that serve /v1/responses natively.
const effortResponsesReply = `{"id":"resp_effort","object":"response","created_at":1741036800,"status":"completed","model":"fixture","output":[{"type":"message","id":"msg_effort","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`

// effortPayload builds the client request body for surface. Parameters:
// modelID is the catalog ID and effortJSON is the raw JSON value of the
// caller's effort control. It returns the request body string.
func effortPayload(surface effortSurface, modelID, effortJSON string) string {
	switch surface {
	case effortResponses:
		return `{"model":"` + modelID + `","stream":false,"max_output_tokens":64,"input":"hello","reasoning":{"effort":` + effortJSON + `}}`
	case effortClaude:
		return `{"model":"` + modelID + `","stream":false,"max_tokens":64,"messages":[{"role":"user","content":"hello"}],"output_config":{"effort":` + effortJSON + `}}`
	default:
		return `{"model":"` + modelID + `","stream":false,"max_tokens":64,"messages":[{"role":"user","content":"hello"}],"reasoning_effort":` + effortJSON + `}`
	}
}

// relayEffortWire drives one request through the real relay entry point for
// surface against a fake upstream of channel. Parameters: t owns assertions,
// channel is the provider channel type, surface selects the client protocol,
// modelID is the upstream model, payload is the client body, reply is the
// upstream response body and wantPath is the expected upstream path. It
// returns the single upstream request body decoded as raw root fields.
func relayEffortWire(t *testing.T, channel int, surface effortSurface, modelID, payload, reply, wantPath string) map[string]json.RawMessage {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })

	var mu sync.Mutex
	var bodies [][]byte
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body read failed", http.StatusBadRequest)
			return
		}
		mu.Lock()
		bodies = append(bodies, raw)
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(upstream.Close)
	oldClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = oldClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, string(surface), strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer fixture-key")
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Channel, channel)
	c.Set(ctxkey.ChannelId, fallbackNVIDIAChannelID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackNVIDIAChannelID, Type: channel})
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, modelID)
	c.Set(ctxkey.BaseURL, upstream.URL+"/v1")
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_effort_wire")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: 1_000_000})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	var failure *relaymodel.ErrorWithStatusCode
	switch surface {
	case effortResponses:
		failure = RelayResponseAPIHelper(c)
	case effortClaude:
		failure = RelayClaudeMessagesHelper(c)
	default:
		failure = RelayTextHelper(c)
	}
	drainResponseFallbackBilling(t)
	require.Nil(t, failure, "relay must succeed: %v", failure)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, bodies, 1, "exactly one upstream request")
	require.Equal(t, wantPath, paths[0])
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bodies[0], &root))
	require.JSONEq(t, `"`+modelID+`"`, string(root["model"]))
	return root
}

// relayNVIDIAEffort drives surface to the NVIDIA channel, whose every surface
// is lowered to the chat completions endpoint. Parameters mirror
// relayEffortWire. It returns the upstream request root fields.
func relayNVIDIAEffort(t *testing.T, surface effortSurface, modelID, payload string) map[string]json.RawMessage {
	t.Helper()
	return relayEffortWire(t, channeltype.NVIDIA, surface, modelID, payload, effortChatReply, "/v1/chat/completions")
}

// requireNVIDIAWireEffort asserts the effort NVIDIA receives. Parameters: root
// is the upstream body, want is the expected reasoning_effort ("" means the
// field must be absent). No surface may leak a Responses-style reasoning
// object carrying an effort, because NVIDIA's documented control is the
// top-level reasoning_effort field.
func requireNVIDIAWireEffort(t *testing.T, root map[string]json.RawMessage, want string) {
	t.Helper()
	if want == "" {
		require.NotContains(t, root, "reasoning_effort", "catalog-unlisted effort must be dropped so NVIDIA applies its default")
	} else {
		require.Contains(t, root, "reasoning_effort")
		require.JSONEq(t, `"`+want+`"`, string(root["reasoning_effort"]))
	}
	if raw, ok := root["reasoning"]; ok {
		var reasoning map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &reasoning))
		require.NotContains(t, reasoning, "effort", "reasoning.effort must not bypass the catalog filter: %s", raw)
	}
}

// TestNVIDIAReasoningEffortWire_ChatAndResponses reproduces the P2 finding:
// for a model that publishes a reasoning_effort vocabulary, NVIDIA must
// receive only a listed value (normalized to lowercase) and otherwise no
// effort at all, on both the Chat Completions and Responses surfaces.
func TestNVIDIAReasoningEffortWire_ChatAndResponses(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		effort string
		want   string
	}{
		// Controls: listed values are forwarded unchanged.
		{name: "listed high", model: "mistralai/mistral-large-4-1t-a52b", effort: "high", want: "high"},
		{name: "listed none", model: "mistralai/mistral-large-4-1t-a52b", effort: "none", want: "none"},
		{name: "listed max", model: "z-ai/glm-5.3", effort: "max", want: "max"},
		// Control: models without a published vocabulary keep dropping the field.
		{name: "no vocabulary", model: "nvidia/nemotron-3.5-bolt-e4b", effort: "high", want: ""},
		// Bug class: globally valid but catalog-unlisted values.
		{name: "unlisted max", model: "mistralai/mistral-large-4-1t-a52b", effort: "max", want: ""},
		{name: "unlisted medium", model: "z-ai/glm-5.3", effort: "medium", want: ""},
		{name: "unlisted low", model: "mistralai/mistral-large-4-1t-a52b", effort: "low", want: ""},
		// Bug class: case/whitespace variants of a listed value.
		{name: "case variant", model: "mistralai/mistral-large-4-1t-a52b", effort: " HIGH ", want: "high"},
	}
	for _, surface := range []effortSurface{effortChat, effortResponses} {
		for _, tc := range cases {
			t.Run(string(surface)+"/"+tc.name, func(t *testing.T) {
				effortJSON, err := json.Marshal(tc.effort)
				require.NoError(t, err)
				root := relayNVIDIAEffort(t, surface, tc.model, effortPayload(surface, tc.model, string(effortJSON)))
				requireNVIDIAWireEffort(t, root, tc.want)
			})
		}
	}
}

// TestNVIDIAReasoningEffortWire_InvalidValue proves a value outside every
// vocabulary ("extreme") never reaches NVIDIA from any surface: it is dropped
// before the upstream request.
func TestNVIDIAReasoningEffortWire_InvalidValue(t *testing.T) {
	const id = "mistralai/mistral-large-4-1t-a52b"
	for _, surface := range []effortSurface{effortChat, effortResponses} {
		t.Run(string(surface), func(t *testing.T) {
			root := relayNVIDIAEffort(t, surface, id, effortPayload(surface, id, `"extreme"`))
			requireNVIDIAWireEffort(t, root, "")
		})
	}
}

// TestNVIDIAReasoningEffortWire_ChatReasoningObject covers Chat clients that
// send the Responses-style reasoning object: its effort is promoted into
// NVIDIA's reasoning_effort only when listed, an explicit reasoning_effort
// wins, and the reasoning object itself never reaches NVIDIA.
func TestNVIDIAReasoningEffortWire_ChatReasoningObject(t *testing.T) {
	const id = "mistralai/mistral-large-4-1t-a52b"
	for _, tc := range []struct {
		name    string
		control string
		want    string
	}{
		{name: "listed object effort", control: `"reasoning":{"effort":"HIGH","summary":"auto"}`, want: "high"},
		{name: "unlisted object effort", control: `"reasoning":{"effort":"max"}`, want: ""},
		{name: "explicit field wins", control: `"reasoning_effort":"none","reasoning":{"effort":"high"}`, want: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"model":"` + id + `","stream":false,"max_tokens":64,"messages":[{"role":"user","content":"hello"}],` + tc.control + `}`
			root := relayNVIDIAEffort(t, effortChat, id, payload)
			requireNVIDIAWireEffort(t, root, tc.want)
			require.NotContains(t, root, "reasoning", "NVIDIA chat schema has no reasoning object")
		})
	}
}

// TestNVIDIAReasoningEffortWire_ClaudeMessages pins that the Claude Messages
// surface carries no reasoning_effort to NVIDIA: the shared converter does not
// lower output_config.effort for OpenAI-compatible destinations, so neither a
// listed nor an unlisted Anthropic effort can bypass the catalog filter.
func TestNVIDIAReasoningEffortWire_ClaudeMessages(t *testing.T) {
	const id = "mistralai/mistral-large-4-1t-a52b"
	for _, effort := range []string{"high", "max"} {
		t.Run(effort, func(t *testing.T) {
			root := relayNVIDIAEffort(t, effortClaude, id, effortPayload(effortClaude, id, `"`+effort+`"`))
			requireNVIDIAWireEffort(t, root, "")
			require.NotContains(t, root, "output_config", "Claude-only controls must not reach NVIDIA")
		})
	}
}

// TestXAIReasoningEffortWire pins the shared catalog policy on both xAI
// surfaces that carry an effort: Chat (adaptor ConvertRequest) and native
// Responses (normalizeResponseProviderReasoning). A listed value is forwarded
// in its canonical lowercase spelling, an unlisted one is omitted, and
// unknown models keep their explicit native Responses effort untouched.
func TestXAIReasoningEffortWire(t *testing.T) {
	for _, tc := range []struct {
		name    string
		surface effortSurface
		model   string
		effort  string
		want    string
	}{
		{name: "chat listed", surface: effortChat, model: "grok-3-mini", effort: "medium", want: "medium"},
		{name: "chat case variant", surface: effortChat, model: "grok-3-mini", effort: " HIGH ", want: "high"},
		{name: "chat unlisted", surface: effortChat, model: "grok-4.20-multi-agent-0309", effort: "none", want: ""},
		{name: "chat invalid", surface: effortChat, model: "grok-3-mini", effort: "extreme", want: ""},
		{name: "chat unknown model", surface: effortChat, model: "grok-future", effort: "high", want: ""},
		{name: "responses listed", surface: effortResponses, model: "grok-3-mini", effort: "medium", want: "medium"},
		{name: "responses case variant", surface: effortResponses, model: "grok-3-mini", effort: " HIGH ", want: "high"},
		{name: "responses unlisted", surface: effortResponses, model: "grok-4.20-multi-agent-0309", effort: "none", want: ""},
		{name: "responses invalid", surface: effortResponses, model: "grok-3-mini", effort: "extreme", want: ""},
		{name: "responses unknown model keeps explicit effort", surface: effortResponses, model: "grok-future", effort: " HIGH ", want: " HIGH "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			effortJSON, err := json.Marshal(tc.effort)
			require.NoError(t, err)
			reply, path := effortChatReply, "/v1/chat/completions"
			if tc.surface == effortResponses {
				reply, path = effortResponsesReply, "/v1/responses"
			}
			root := relayEffortWire(t, channeltype.XAI, tc.surface, tc.model, effortPayload(tc.surface, tc.model, string(effortJSON)), reply, path)
			if tc.surface == effortChat {
				if tc.want == "" {
					require.NotContains(t, root, "reasoning_effort")
					return
				}
				require.JSONEq(t, string(mustJSONString(t, tc.want)), string(root["reasoning_effort"]))
				return
			}
			var reasoning map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(root["reasoning"], &reasoning))
			if tc.want == "" {
				require.NotContains(t, reasoning, "effort")
				return
			}
			require.JSONEq(t, string(mustJSONString(t, tc.want)), string(reasoning["effort"]))
		})
	}
}

// mustJSONString encodes value as a JSON string. Parameters: t owns the
// assertion and value is the text. It returns the encoded bytes.
func mustJSONString(t *testing.T, value string) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}
