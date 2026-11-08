package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// countTokensClaudeModel is the only model the fixture token and channel admit.
	countTokensClaudeModel = "claude-sonnet-4-5"
	// countTokensUpstreamInputTokens is what the fake Anthropic count_tokens endpoint answers.
	countTokensUpstreamInputTokens = 4242
	// countTokensClaudeCodeBody is the shape Claude Code sends to count_tokens:
	// a Messages body without max_tokens.
	countTokensClaudeCodeBody = `{"model":"` + countTokensClaudeModel + `","system":[{"type":"text","text":"You are Claude Code."}],"messages":[{"role":"user","content":[{"type":"text","text":"hello there"},{"type":"tool_result","tool_use_id":"toolu_1","content":"file contents"}]}],"tools":[{"name":"Read","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]}`
	// countTokensWithMaxTokensBody is a count_tokens-style body that also carries
	// max_tokens, which is enough for the generation handler to accept it.
	countTokensWithMaxTokensBody = `{"model":"` + countTokensClaudeModel + `","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"hello there"},{"type":"tool_result","tool_use_id":"toolu_1","content":"file contents"}]}]}`
)

// anthropicUpstreamCall records one request observed by the fake Anthropic upstream.
type anthropicUpstreamCall struct {
	path    string
	headers http.Header
	body    []byte
}

// anthropicUpstreamCapture records every request the fake Anthropic upstream observed.
type anthropicUpstreamCapture struct {
	mu    sync.Mutex
	calls []anthropicUpstreamCall
}

// snapshot returns a copy of the recorded upstream calls in arrival order.
func (u *anthropicUpstreamCapture) snapshot() []anthropicUpstreamCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]anthropicUpstreamCall(nil), u.calls...)
}

// paths returns the request path of every recorded upstream call.
func (u *anthropicUpstreamCapture) paths() []string {
	var out []string
	for _, call := range u.snapshot() {
		out = append(out, call.path)
	}
	return out
}

// newFakeAnthropicUpstream starts a TLS fixture that behaves like the Anthropic
// API: /v1/messages runs a (paid) generation and /v1/messages/count_tokens is
// the free counter. countTokens overrides the count_tokens reply when non-nil.
func newFakeAnthropicUpstream(t *testing.T, countTokens func(w http.ResponseWriter)) (*httptest.Server, *anthropicUpstreamCapture) {
	t.Helper()
	capture := &anthropicUpstreamCapture{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		capture.mu.Lock()
		capture.calls = append(capture.calls, anthropicUpstreamCall{path: r.URL.Path, headers: r.Header.Clone(), body: raw})
		capture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/messages/count_tokens":
			if countTokens != nil {
				countTokens(w)
				return
			}
			w.Header().Set("request-id", "req_fixture_count")
			_, _ = io.WriteString(w, `{"input_tokens":4242}`)
		case "/v1/messages":
			_, _ = io.WriteString(w, `{"id":"msg_fixture","type":"message","role":"assistant","model":"`+countTokensClaudeModel+`","content":[{"type":"text","text":"generated"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1000,"output_tokens":500}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"not_found_error","message":"not found"}}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, capture
}

// countTokensLedger returns the fixture user's quota, the token's remaining
// quota, the number of consume logs and the number of request-cost rows.
func countTokensLedger(t *testing.T) (userQuota int64, tokenQuota int64, consumeLogs int64, requestCosts int64) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Where("id = ?", passthroughKeysUserID).Take(&user).Error)
	var token model.Token
	require.NoError(t, model.DB.Where("id = ?", passthroughKeysTokenID).Take(&token).Error)
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", passthroughKeysUserID, model.LogTypeConsume).Count(&consumeLogs).Error)
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("user_id = ?", passthroughKeysUserID).Count(&requestCosts).Error)
	return user.Quota, token.RemainQuota, consumeLogs, requestCosts
}

// postClaudeJSON sends one authenticated Claude-style request through the engine.
func postClaudeJSON(t *testing.T, engine *gin.Engine, key, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("x-api-key", "sk-"+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	drainCriticalTasksForRouter(t)
	return w
}

// TestClaudeCountTokensAliasNeverRunsGeneration drives count_tokens requests
// through every Claude Code path alias of the shipped router. Each must reach
// the free upstream counter and never the paid /v1/messages generation, and
// must leave the user, token and request-cost ledgers untouched.
func TestClaudeCountTokensAliasNeverRunsGeneration(t *testing.T) {
	for _, path := range []string{
		"/v1/messages/count_tokens",
		"/v1/v1/messages/count_tokens",
		"/openai/v1/messages/count_tokens",
		"/openai/v1/v1/messages/count_tokens",
		"/api/v1/v1/messages/count_tokens",
	} {
		for name, body := range map[string]string{
			"claude_code_body":     countTokensClaudeCodeBody,
			"body_with_max_tokens": countTokensWithMaxTokensBody,
		} {
			t.Run(strings.TrimPrefix(strings.ReplaceAll(path, "/", "_"), "_")+"/"+name, func(t *testing.T) {
				upstream, capture := newFakeAnthropicUpstream(t, nil)
				engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
				client.HTTPClient = upstream.Client()
				userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

				w := postClaudeJSON(t, engine, key, path+"?beta=true", body, nil)

				userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
				require.NotContains(t, capture.paths(), "/v1/messages",
					"a count_tokens request must never dispatch a paid generation (status %d, user quota -%d, token quota -%d, consume logs +%d, request costs +%d, body %s)",
					w.Code, userBefore-userAfter, tokenBefore-tokenAfter, logsAfter-logsBefore, costsAfter-costsBefore, w.Body.String())
				require.Equal(t, userBefore, userAfter, "count_tokens must not move the user quota")
				require.Equal(t, tokenBefore, tokenAfter, "count_tokens must not move the token quota")
				require.Equal(t, logsBefore, logsAfter, "count_tokens must not write consume logs")
				require.Equal(t, costsBefore, costsAfter, "count_tokens must not record a request cost")
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Equal(t, []string{"/v1/messages/count_tokens"}, capture.paths())
				var payload map[string]int
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
				require.Equal(t, countTokensUpstreamInputTokens, payload["input_tokens"])
			})
		}
	}
}

// TestClaudeCountTokensFormatMismatchNeverRunsGeneration proves the format
// auto-detector cannot transparently re-dispatch a count_tokens request whose
// body looks like a Responses API request onto a paid generation endpoint.
func TestClaudeCountTokensFormatMismatchNeverRunsGeneration(t *testing.T) {
	upstream, capture := newFakeAnthropicUpstream(t, nil)
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
	client.HTTPClient = upstream.Client()
	userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

	w := postClaudeJSON(t, engine, key, "/v1/messages/count_tokens", `{"model":"`+countTokensClaudeModel+`","input":"hello","max_output_tokens":16}`, nil)

	userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
	require.NotContains(t, capture.paths(), "/v1/messages",
		"a count_tokens request must never dispatch a paid generation (status %d, user quota -%d, token quota -%d, consume logs +%d, request costs +%d)",
		w.Code, userBefore-userAfter, tokenBefore-tokenAfter, logsAfter-logsBefore, costsAfter-costsBefore)
	require.Equal(t, userBefore, userAfter)
	require.Equal(t, tokenBefore, tokenAfter)
	require.Equal(t, logsBefore, logsAfter)
	require.Equal(t, costsBefore, costsAfter)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	requireClaudeErrorEnvelope(t, w, "invalid_request_error")
}

// requireClaudeErrorEnvelope asserts an Anthropic-format error body of errType.
func requireClaudeErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder, errType string) {
	t.Helper()
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	var envelope struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
	require.Equal(t, "error", envelope.Type, w.Body.String())
	require.Equal(t, errType, envelope.Error.Type, w.Body.String())
	require.NotEmpty(t, envelope.Error.Message)
}

// requireNoUserKeyUpstream asserts the caller's gateway key never reached the upstream.
func requireNoUserKeyUpstream(t *testing.T, calls []anthropicUpstreamCall, key string) {
	t.Helper()
	for _, call := range calls {
		for name, values := range call.headers {
			for _, value := range values {
				require.NotContains(t, value, key, "the gateway key leaked upstream in header %s", name)
			}
		}
	}
}

// TestClaudeCountTokensAnthropicPassthrough proves a native Anthropic channel
// answers from its own count_tokens endpoint with the channel credential, the
// caller's anthropic-version / anthropic-beta headers and the mapped model,
// and that the free call leaves every ledger untouched.
func TestClaudeCountTokensAnthropicPassthrough(t *testing.T) {
	const mapped = "claude-sonnet-4-5-20250929"
	upstream, capture := newFakeAnthropicUpstream(t, nil)
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
	client.HTTPClient = upstream.Client()
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 929293).
		Update("model_mapping", `{"`+countTokensClaudeModel+`":"`+mapped+`"}`).Error)
	userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

	w := postClaudeJSON(t, engine, key, "/v1/messages/count_tokens?beta=true", countTokensClaudeCodeBody,
		map[string]string{"anthropic-beta": "token-counting-2024-11-01,context-1m-2025-08-07"})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"input_tokens":4242}`, w.Body.String())
	require.Equal(t, "req_fixture_count", w.Header().Get("request-id"))
	calls := capture.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, "/v1/messages/count_tokens", calls[0].path)
	require.Equal(t, "fixture-only", calls[0].headers.Get("x-api-key"), "the channel credential must authenticate upstream")
	require.Empty(t, calls[0].headers.Get("Authorization"))
	require.Equal(t, "2023-06-01", calls[0].headers.Get("anthropic-version"))
	beta := calls[0].headers.Get("anthropic-beta")
	require.Contains(t, beta, "token-counting-2024-11-01")
	require.Contains(t, beta, "context-1m-2025-08-07")
	require.Equal(t, "application/json", calls[0].headers.Get("Content-Type"))
	requireNoUserKeyUpstream(t, calls, key)
	var forwarded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(calls[0].body, &forwarded))
	require.JSONEq(t, `"`+mapped+`"`, string(forwarded["model"]), "the upstream must count for the mapped model")
	require.NotContains(t, forwarded, "max_tokens")
	require.Contains(t, forwarded, "messages")
	require.Contains(t, forwarded, "tools")
	require.Contains(t, forwarded, "system")

	userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
	require.Equal(t, userBefore, userAfter)
	require.Equal(t, tokenBefore, tokenAfter)
	require.Equal(t, logsBefore, logsAfter)
	require.Equal(t, costsBefore, costsAfter)
}

// TestClaudeCountTokensAnthropicUpstreamFailures covers the native counter's
// failure handling: request errors are returned verbatim, while channel-level
// failures fall back to the local estimate. No case may dispatch a generation
// or touch the ledger.
func TestClaudeCountTokensAnthropicUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantBody   string
	}{
		{"server_error_falls_back", http.StatusInternalServerError, `{"type":"error","error":{"type":"api_error","message":"boom"}}`, http.StatusOK, ""},
		{"overloaded_falls_back", 529, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`, http.StatusOK, ""},
		{"rate_limited_falls_back", http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, http.StatusOK, ""},
		{"unauthorized_channel_falls_back", http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, http.StatusOK, ""},
		{"compatible_proxy_without_endpoint_falls_back", http.StatusNotFound, `404 page not found`, http.StatusOK, ""},
		{"malformed_success_falls_back", http.StatusOK, `{"tokens":1}`, http.StatusOK, ""},
		{"request_error_is_returned", http.StatusBadRequest, `{"type":"error","error":{"type":"invalid_request_error","message":"tools.0: server tools are not supported"}}`, http.StatusBadRequest, `{"type":"error","error":{"type":"invalid_request_error","message":"tools.0: server tools are not supported"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newFakeAnthropicUpstream(t, func(w http.ResponseWriter) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
			client.HTTPClient = upstream.Client()
			userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

			w := postClaudeJSON(t, engine, key, "/v1/messages/count_tokens", countTokensClaudeCodeBody, nil)

			require.Equal(t, []string{"/v1/messages/count_tokens"}, capture.paths())
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, w.Body.String())
			} else {
				var payload map[string]int
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
				require.Positive(t, payload["input_tokens"])
				require.NotEqual(t, countTokensUpstreamInputTokens, payload["input_tokens"])
			}
			userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
			require.Equal(t, userBefore, userAfter)
			require.Equal(t, tokenBefore, tokenAfter)
			require.Equal(t, logsBefore, logsAfter)
			require.Equal(t, costsBefore, costsAfter)
		})
	}
}

// TestClaudeCountTokensOpenAICompatibleLocalEstimate proves a non-Anthropic
// channel is answered locally: no upstream call at all, a positive estimate
// that grows with the prompt, and an untouched ledger.
func TestClaudeCountTokensOpenAICompatibleLocalEstimate(t *testing.T) {
	upstream, capture := newExtraBodyGateway(t)
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, countTokensClaudeModel, true)
	client.HTTPClient = upstream.Client()
	userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

	short := postClaudeJSON(t, engine, key, "/v1/messages/count_tokens", countTokensClaudeCodeBody, nil)
	longBody := strings.Replace(countTokensClaudeCodeBody, "hello there", strings.Repeat("hello there ", 200), 1)
	long := postClaudeJSON(t, engine, key, "/v1/v1/messages/count_tokens", longBody, nil)

	bodies, _, _ := capture.snapshot()
	require.Empty(t, bodies, "a local estimate must not call the upstream")
	require.Equal(t, http.StatusOK, short.Code, short.Body.String())
	require.Equal(t, http.StatusOK, long.Code, long.Body.String())
	var shortPayload, longPayload map[string]int
	require.NoError(t, json.Unmarshal(short.Body.Bytes(), &shortPayload))
	require.NoError(t, json.Unmarshal(long.Body.Bytes(), &longPayload))
	require.Positive(t, shortPayload["input_tokens"])
	require.Greater(t, longPayload["input_tokens"], shortPayload["input_tokens"]+300)
	userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
	require.Equal(t, userBefore, userAfter)
	require.Equal(t, tokenBefore, tokenAfter)
	require.Equal(t, logsBefore, logsAfter)
	require.Equal(t, costsBefore, costsAfter)
}

// TestClaudeCountTokensRejectsInvalidRequests proves malformed, oversized and
// unsupported count_tokens requests get Anthropic-format 4xx errors without any
// upstream call or ledger movement.
func TestClaudeCountTokensRejectsInvalidRequests(t *testing.T) {
	oversized := `{"model":"` + countTokensClaudeModel + `","messages":[{"role":"user","content":"` +
		strings.Repeat("a", 32<<20) + `"}]}`
	for _, tc := range []struct {
		name        string
		channelType int
		body        string
		wantStatus  int
		wantType    string
	}{
		{"invalid_json", channeltype.Anthropic, `{"model":`, http.StatusBadRequest, "invalid_request_error"},
		{"missing_model", channeltype.Anthropic, `{"messages":[{"role":"user","content":"hi"}]}`, http.StatusBadRequest, "invalid_request_error"},
		{"empty_messages", channeltype.Anthropic, `{"model":"` + countTokensClaudeModel + `","messages":[]}`, http.StatusBadRequest, "invalid_request_error"},
		{"oversized_body", channeltype.Anthropic, oversized, http.StatusRequestEntityTooLarge, "request_too_large"},
		{"url_image_local", channeltype.OpenAICompatible, `{"model":"` + countTokensClaudeModel + `","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]}]}`, http.StatusBadRequest, "invalid_request_error"},
		{"server_tool_local", channeltype.OpenAICompatible, `{"model":"` + countTokensClaudeModel + `","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, http.StatusBadRequest, "invalid_request_error"},
		{"mcp_connector_local", channeltype.OpenAICompatible, `{"model":"` + countTokensClaudeModel + `","messages":[{"role":"user","content":"hi"}],"mcp_servers":[{"type":"url","url":"https://mcp.example.invalid","name":"x"}]}`, http.StatusBadRequest, "invalid_request_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newFakeAnthropicUpstream(t, nil)
			engine, key := ambiguousKeysRouter(t, upstream.URL, tc.channelType, countTokensClaudeModel, true)
			client.HTTPClient = upstream.Client()
			userBefore, tokenBefore, logsBefore, costsBefore := countTokensLedger(t)

			w := postClaudeJSON(t, engine, key, "/v1/messages/count_tokens", tc.body, nil)

			require.Empty(t, capture.paths(), "a rejected request must not reach the upstream")
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			requireClaudeErrorEnvelope(t, w, tc.wantType)
			userAfter, tokenAfter, logsAfter, costsAfter := countTokensLedger(t)
			require.Equal(t, userBefore, userAfter)
			require.Equal(t, tokenBefore, tokenAfter)
			require.Equal(t, logsBefore, logsAfter)
			require.Equal(t, costsBefore, costsAfter)
		})
	}
}

// TestClaudeCountTokensAuthAndModelPermission proves count_tokens enforces the
// same token authentication and model allow-list as /v1/messages.
func TestClaudeCountTokensAuthAndModelPermission(t *testing.T) {
	upstream, capture := newFakeAnthropicUpstream(t, nil)
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
	client.HTTPClient = upstream.Client()

	unauthenticated := postClaudeJSON(t, engine, "not-a-real-key", "/v1/messages/count_tokens", countTokensClaudeCodeBody, nil)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, unauthenticated.Body.String())

	forbidden := postClaudeJSON(t, engine, key, "/v1/messages/count_tokens",
		strings.Replace(countTokensClaudeCodeBody, countTokensClaudeModel, "claude-opus-4-1", 1), nil)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	require.Empty(t, capture.paths())
}

// TestClaudeMessagesGenerationUnchanged guards that the canonical Messages
// endpoint and its exact Claude Code aliases still run (and bill) generations.
func TestClaudeMessagesGenerationUnchanged(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/v1/messages", "/openai/v1/messages", "/api/v1/v1/messages"} {
		t.Run(strings.TrimPrefix(strings.ReplaceAll(path, "/", "_"), "_"), func(t *testing.T) {
			upstream, capture := newFakeAnthropicUpstream(t, nil)
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
			client.HTTPClient = upstream.Client()
			userBefore, _, logsBefore, _ := countTokensLedger(t)

			w := postClaudeJSON(t, engine, key, path+"?beta=true", countTokensWithMaxTokensBody, nil)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, []string{"/v1/messages"}, capture.paths())
			require.Contains(t, w.Body.String(), `"generated"`)
			userAfter, _, logsAfter, _ := countTokensLedger(t)
			require.Less(t, userAfter, userBefore, "a generation must still be billed")
			require.Equal(t, logsBefore+1, logsAfter)
		})
	}
}

// TestClaudeMessagesUnknownSubPathsNeverRunGeneration proves no other
// sub-resource of a Messages alias is rewritten onto the billed endpoint.
func TestClaudeMessagesUnknownSubPathsNeverRunGeneration(t *testing.T) {
	upstream, capture := newFakeAnthropicUpstream(t, nil)
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, countTokensClaudeModel, true)
	client.HTTPClient = upstream.Client()
	userBefore, tokenBefore, logsBefore, _ := countTokensLedger(t)

	for _, path := range []string{
		"/v1/v1/messages/batches", "/openai/v1/messages/count_tokens/extra",
		"/v1/v1/messagesx", "/v1/messages/batches", "/v1/messages/count_tokens/extra",
	} {
		w := postClaudeJSON(t, engine, key, path, countTokensWithMaxTokensBody, nil)
		require.Equal(t, http.StatusNotFound, w.Code, "%s: %s", path, w.Body.String())
		w = postClaudeJSON(t, engine, key, path, `{"model":"`+countTokensClaudeModel+`","input":"hello","max_output_tokens":16}`, nil)
		require.Equal(t, http.StatusNotFound, w.Code, "%s: %s", path, w.Body.String())
	}

	require.Empty(t, capture.paths())
	userAfter, tokenAfter, logsAfter, _ := countTokensLedger(t)
	require.Equal(t, userBefore, userAfter)
	require.Equal(t, tokenBefore, tokenAfter)
	require.Equal(t, logsBefore, logsAfter)
}

// TestClaudeCountTokensRouteUsesDedicatedHandler pins the route table: the
// count_tokens path is served by the free handler, never the generation relay.
func TestClaudeCountTokensRouteUsesDedicatedHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)
	found := false
	for _, route := range engine.Routes() {
		if !strings.Contains(route.Path, "count_tokens") {
			continue
		}
		found = true
		require.Equal(t, http.MethodPost, route.Method)
		require.Equal(t, "/v1/messages/count_tokens", route.Path)
		require.True(t, strings.HasSuffix(route.Handler, "relay/controller.RelayClaudeCountTokens"), route.Handler)
	}
	require.True(t, found, "the count_tokens route must be registered")
}
