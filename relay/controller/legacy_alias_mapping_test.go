package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// legacyAliasObservation captures one provider request on the fixture server so
// assertions run on the test goroutine rather than the HTTP handler goroutine.
type legacyAliasObservation struct {
	Path string
	Body map[string]any
	Err  error
}

// legacyAliasUpstream starts a TLS provider fixture that records every request
// and answers with reply. Parameters: t owns cleanup, contentType labels the
// reply and reply is the raw provider body. Returns: the base URL, the bounded
// observation channel and the request counter.
func legacyAliasUpstream(t *testing.T, contentType, reply string) (string, <-chan legacyAliasObservation, *atomic.Int32) {
	t.Helper()
	observations := make(chan legacyAliasObservation, 4)
	calls := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		select {
		case observations <- legacyAliasObservation{Path: r.URL.Path, Body: body, Err: err}:
		default:
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(server.Close)
	previous := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	return server.URL, observations, calls
}

// legacyAliasConsumeLogs loads the finalized consume rows for one request.
// Parameters: t owns assertions and requestID selects the rows. Returns: the rows.
func legacyAliasConsumeLogs(t *testing.T, requestID string) []model.Log {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	return logs
}

// legacyAliasMistralBody returns the client request for one Mistral entrypoint.
// Parameters: protocol is chat, responses or messages and effort is an optional
// caller reasoning effort. Returns: the request path and JSON body.
func legacyAliasMistralBody(protocol, effort string) (string, string) {
	switch protocol {
	case "responses":
		reasoning := ""
		if effort != "" {
			reasoning = `,"reasoning":{"effort":"` + effort + `"}`
		}
		return "/v1/responses", `{"model":"alias","input":"Say hello.","max_output_tokens":64` + reasoning + `}`
	case "messages":
		return "/v1/messages", `{"model":"alias","messages":[{"role":"user","content":"Say hello."}],"max_tokens":64}`
	default:
		field := ""
		if effort != "" {
			field = `,"reasoning_effort":"` + effort + `"`
		}
		return "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"Say hello."}],"max_tokens":64` + field + `}`
	}
}

// mistralLegacyAliasReceipt is a fixed upstream usage receipt: 10,000 input and
// 10,000 output tokens. Large 4 bills (0.70+2.10)/100 USD = 14,000 quota; the
// former Large 3 alias tariff (0.50+1.50)/100 USD would bill 10,000 quota.
const mistralLegacyAliasReceipt = `{"id":"cmpl-fixture","object":"chat.completion","created":1,"model":"mistral-large-latest","choices":[{"index":0,"message":{"role":"assistant","content":"Hello there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10000,"completion_tokens":10000,"total_tokens":20000}}`

// TestLegacyAliasMistralLargeLatestEffortAndTariff verifies that an existing
// channel mapping to the mutable mistral-large-latest alias keeps working after
// the alias moved to Large 4: unsupported efforts (none, xhigh, minimal) are
// dropped instead of forwarded to an upstream that rejects them, supported
// efforts are case-normalized and forwarded, and the ledger charges Large 4
// prices. Models that document none keep forwarding it. It returns nothing.
func TestLegacyAliasMistralLargeLatestEffortAndTariff(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, actual, effort, query, want string
		charge                                      int64
	}{
		{name: "chat_none_dropped", protocol: "chat", actual: "mistral-large-latest", effort: "none", charge: 14000},
		{name: "chat_unknown_dropped", protocol: "chat", actual: "mistral-large-latest", effort: "xhigh", charge: 14000},
		{name: "chat_low_forwarded", protocol: "chat", actual: "mistral-large-latest", effort: "low", want: "low", charge: 14000},
		{name: "chat_case_normalized", protocol: "chat", actual: "mistral-large-latest", effort: "HIGH", want: "high", charge: 14000},
		{name: "chat_omitted", protocol: "chat", actual: "mistral-large-latest", charge: 14000},
		{name: "chat_thinking_query", protocol: "chat", actual: "mistral-large-latest", query: "?thinking=true", want: "high", charge: 14000},
		{name: "responses_none_dropped", protocol: "responses", actual: "mistral-large-latest", effort: "none", charge: 14000},
		{name: "responses_minimal_dropped", protocol: "responses", actual: "mistral-large-latest", effort: "minimal", charge: 14000},
		{name: "responses_low_forwarded", protocol: "responses", actual: "mistral-large-latest", effort: "low", want: "low", charge: 14000},
		{name: "responses_case_normalized", protocol: "responses", actual: "mistral-large-latest", effort: "HIGH", want: "high", charge: 14000},
		{name: "messages_tariff", protocol: "messages", actual: "mistral-large-latest", charge: 14000},
		// Controls: Small 4 and Medium 3.5 document none, so it is still forwarded.
		{name: "control_small_none_forwarded", protocol: "chat", actual: "mistral-small-latest", effort: "none", want: "none", charge: 3750},
		{name: "control_medium_responses_none", protocol: "responses", actual: "mistral-medium-latest", effort: "none", want: "none", charge: 45000},
		{name: "medium_unsupported_dropped", protocol: "chat", actual: "mistral-medium-latest", effort: "medium", charge: 45000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(1_000_000)
			xaiVideoSetup(t, balance, false)
			base, observations, calls := legacyAliasUpstream(t, "application/json", mistralLegacyAliasReceipt)
			path, body := legacyAliasMistralBody(tc.protocol, tc.effort)
			c, w, id := protocolContext(t, channeltype.Mistral, tc.actual, path+tc.query, body, base, balance, 1, false, nil)

			apiErr := relayLyriaProtocol(c, tc.protocol)
			drainCriticalTasks(t)
			require.Nil(t, apiErr, "the mapped request must succeed")
			require.EqualValues(t, 1, calls.Load(), "exactly one provider request")

			observed := <-observations
			require.NoError(t, observed.Err)
			require.Equal(t, "/v1/chat/completions", observed.Path)
			require.Equal(t, tc.actual, observed.Body["model"], "the channel mapping target reaches the provider")
			require.NotContains(t, observed.Body, "reasoning", "Responses-style controls are translated, never forwarded")
			if tc.want == "" {
				require.NotContains(t, observed.Body, "reasoning_effort", "unsupported or omitted effort must not reach the provider")
			} else {
				require.Equal(t, tc.want, observed.Body["reasoning_effort"])
			}
			require.Contains(t, w.Body.String(), "Hello there", "the provider answer is delivered")

			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			require.Equal(t, tc.charge, requestCostQuota(t, id))
			logs := legacyAliasConsumeLogs(t, id)
			require.Len(t, logs, 1)
			require.EqualValues(t, tc.charge, logs[0].Quota)
			require.Equal(t, tc.actual, logs[0].ModelName, "billing uses the mapped model")
		})
	}
}

// cohereLegacyNorthReply is a native Cohere v1 chat reply with 10,000 input and
// 5,000 output tokens.
const cohereLegacyNorthReply = `{"response_id":"resp-fixture","text":"Bonjour","generation_id":"gen-fixture","finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":10000,"output_tokens":5000}}}`

// cohereLegacyNorthStream is a native Cohere v1 newline-delimited chat stream
// with the same usage receipt as cohereLegacyNorthReply.
const cohereLegacyNorthStream = `{"is_finished":false,"event_type":"stream-start","generation_id":"gen-fixture"}
{"is_finished":false,"event_type":"text-generation","text":"Bonjour"}
{"is_finished":true,"event_type":"stream-end","finish_reason":"COMPLETE","response":{"response_id":"resp-fixture","text":"Bonjour","generation_id":"gen-fixture","finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":10000,"output_tokens":5000}}}}
`

// TestLegacyAliasCohereNorthMappingsReachCanonicalIDs verifies that channel
// mappings (and direct requests) that still name the historical hyphenated North
// spellings send Cohere's published dotted model IDs upstream on every chat
// entrypoint, deliver the answer, and bill under the mapped name at the catalog
// price. Canonical IDs and other Cohere models are controls. It returns nothing.
func TestLegacyAliasCohereNorthMappingsReachCanonicalIDs(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, mapped, requested, wire string
		stream                                  bool
		charge                                  int64
	}{
		{name: "mini_code_legacy_mapping", protocol: "chat", mapped: "north-mini-code-1-0", requested: "alias", wire: "north-mini-code-1.0"},
		{name: "translate_legacy_mapping", protocol: "chat", mapped: "north-small-translate-1-0", requested: "alias", wire: "north-small-translate-1.0"},
		{name: "mini_code_legacy_stream", protocol: "chat", mapped: "north-mini-code-1-0", requested: "alias", wire: "north-mini-code-1.0", stream: true},
		{name: "translate_legacy_responses", protocol: "responses", mapped: "north-small-translate-1-0", requested: "alias", wire: "north-small-translate-1.0"},
		{name: "translate_legacy_messages", protocol: "messages", mapped: "north-small-translate-1-0", requested: "alias", wire: "north-small-translate-1.0"},
		{name: "mini_code_direct_legacy", protocol: "chat", mapped: "north-mini-code-1-0", requested: "north-mini-code-1-0", wire: "north-mini-code-1.0"},
		{name: "translate_direct_legacy", protocol: "chat", mapped: "north-small-translate-1-0", requested: "north-small-translate-1-0", wire: "north-small-translate-1.0"},
		{name: "control_canonical_mapping", protocol: "chat", mapped: "north-mini-code-1.0", requested: "alias", wire: "north-mini-code-1.0"},
		{name: "control_canonical_direct", protocol: "chat", mapped: "north-small-translate-1.0", requested: "north-small-translate-1.0", wire: "north-small-translate-1.0"},
		// A priced control proves a zero charge is the catalog price, not a skipped ledger.
		{name: "control_priced_command", protocol: "chat", mapped: "command-r-08-2024", requested: "alias", wire: "command-r-08-2024", charge: 2250},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(1_000_000)
			xaiVideoSetup(t, balance, false)
			reply, contentType := cohereLegacyNorthReply, "application/json"
			if tc.stream {
				reply, contentType = cohereLegacyNorthStream, "application/stream+json"
			}
			base, observations, calls := legacyAliasUpstream(t, contentType, reply)
			path, body := legacyAliasMistralBody(tc.protocol, "")
			body = `{"model":"` + tc.requested + `"` + body[len(`{"model":"alias"`):]
			if tc.stream {
				body = body[:len(body)-1] + `,"stream":true}`
			}
			c, w, id := protocolContext(t, channeltype.Cohere, tc.mapped, path, body, base, balance, 1, false, nil)
			if tc.requested != "alias" {
				c.Set(ctxkey.RequestModel, tc.requested)
				c.Set(ctxkey.ModelMapping, map[string]string{})
			}

			apiErr := relayLyriaProtocol(c, tc.protocol)
			drainCriticalTasks(t)
			require.Nil(t, apiErr, "the mapped request must succeed")
			require.EqualValues(t, 1, calls.Load(), "exactly one provider request")

			observed := <-observations
			require.NoError(t, observed.Err)
			require.Equal(t, "/v1/chat", observed.Path)
			require.Equal(t, tc.wire, observed.Body["model"], "the provider receives its published model ID")
			require.Equal(t, "Say hello.", observed.Body["message"])
			require.Contains(t, w.Body.String(), "Bonjour", "the provider answer is delivered")

			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			if tc.charge > 0 {
				require.Equal(t, tc.charge, requestCostQuota(t, id))
			} else {
				require.Zero(t, requestCostQuota(t, id), "a zero settlement must still finalize the request-cost row")
			}
			logs := legacyAliasConsumeLogs(t, id)
			require.Len(t, logs, 1)
			require.EqualValues(t, tc.charge, logs[0].Quota)
			require.Equal(t, tc.mapped, logs[0].ModelName, "billing keeps the administrator-mapped name")
		})
	}
}
