package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// These end-to-end tests drive the real HTTP entry points (RelayResponseAPIHelper
// and the GET/DELETE action helpers) against a fake upstream with the gateway
// Response-State feature enabled, asserting client-observable state behavior:
// selector validation, hydration of prior context, and the get/delete lifecycle.
//
// They must NOT run in parallel: state.SetForTest toggles a process-global feature
// flag and installs a process-global store, so concurrent state tests would race.

// stateE2EUpstream captures every chat-completion request body a fake upstream
// receives and records whether it was ever contacted. It returns a fixed
// assistant reply so the fallback renders (and commits) a deterministic response.
type stateE2EUpstream struct {
	mu        sync.Mutex
	called    bool
	bodies    [][]byte
	assistant string
}

func (u *stateE2EUpstream) lastBody(t *testing.T) []byte {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	require.NotEmpty(t, u.bodies, "expected the fake upstream to be contacted")
	return u.bodies[len(u.bodies)-1]
}

func (u *stateE2EUpstream) wasCalled() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.called
}

// newStateE2EUpstream starts a fake OpenAI-compatible chat-completions upstream
// and swaps client.HTTPClient to route to it for the duration of the test.
func newStateE2EUpstream(t *testing.T, assistant string) (*stateE2EUpstream, *httptest.Server) {
	t.Helper()
	up := &stateE2EUpstream{assistant: assistant}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "failed to read upstream body")
		up.mu.Lock()
		up.called = true
		up.bodies = append(up.bodies, body)
		up.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
		  "id": "chatcmpl-e2e",
		  "object": "chat.completion",
		  "created": 1741036800,
		  "model": "gpt-4o-mini",
		  "choices": [{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],
		  "usage": {"prompt_tokens": 6, "completion_tokens": 4, "total_tokens": 10}
		}`, up.assistant)
	}))
	t.Cleanup(server.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })
	return up, server
}

// applyStateE2EEnv toggles Redis off and disables consume-logging for a hermetic,
// deterministic state e2e run, restoring both on cleanup.
func applyStateE2EEnv(t *testing.T) {
	t.Helper()
	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })
}

// runStateE2EPostTurn drives one successful non-streaming Responses fallback turn
// through RelayResponseAPIHelper against upstreamURL, using a unique request id so
// the gateway state commit is not idempotency-deduped against a sibling turn. It
// returns the decoded Responses response (whose Id is the resolvable gateway id)
// after draining the async billing task.
func runStateE2EPostTurn(t *testing.T, upstreamURL, requestID, payload string) *openai.ResponseAPIResponse {
	t.Helper()
	// Reset the persisted user quota so consecutive turns start from the same
	// baseline; billing amounts are asserted elsewhere.
	resetFallbackUserQuota(t, 1_000_000)

	recorder := httptest.NewRecorder()
	c := setupResponseStateBillingContext(t, recorder, payload)
	c.Set(ctxkey.BaseURL, upstreamURL)
	c.Set(ctxkey.RequestId, requestID)

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "successful state fallback turn must not error")
	require.Equal(t, http.StatusOK, recorder.Code, "expected 200 for successful turn")

	var resp openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), "failed to decode Responses body")

	// Drain the async post-billing goroutine so a later fixture reset / DB access
	// does not race the spawned billing task.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx), "failed to drain billing task")
	return &resp
}

// TestResponseStateE2E_DualSelectorsRejected proves that a request carrying BOTH a
// conversation and a previous_response_id selector is rejected at the one-api
// boundary with HTTP 400 and the documented invalid_state_selector code before any
// upstream call (A01/B08/E01). Validation catches the mutually-exclusive selectors
// (before the hydrator) but surfaces the state error code via a sentinel so the
// client-observable contract matches Section 6.
func TestResponseStateE2E_DualSelectorsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	enableStateForTest(t)
	applyStateE2EEnv(t)

	up, _ := newStateE2EUpstream(t, "should never be produced")

	recorder := httptest.NewRecorder()
	payload := `{"model":"gpt-4o-mini","stream":false,"conversation":"conv_e2e","previous_response_id":"resp_e2e"}`
	c := setupResponseStateBillingContext(t, recorder, payload)
	c.Set(ctxkey.RequestId, "req_e2e_dual")

	apiErr := RelayResponseAPIHelper(c)
	require.NotNil(t, apiErr, "dual selectors must be rejected")
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode, "dual selectors must be a 400")
	require.Contains(t, apiErr.Message, "mutually exclusive", "expected mutually-exclusive rejection message")
	require.Equal(t, "invalid_state_selector", apiErr.Code,
		"dual selectors must surface the documented invalid_state_selector code (Section 6, E01)")
	require.False(t, up.wasCalled(), "no upstream call may occur for a locally-rejected request")
}

// TestResponseStateE2E_StateOnlyRequestAccepted proves that after a committed
// turn, a follow-up request carrying ONLY previous_response_id (no input field) is
// accepted and the upstream receives the hydrated prior context (A03/B09). This is
// the state-only-request contract exercised end-to-end.
func TestResponseStateE2E_StateOnlyRequestAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	enableStateForTest(t)
	applyStateE2EEnv(t)

	up, server := newStateE2EUpstream(t, "ok-state-only")

	const priorUserText = "state-only prior context marker"

	// Turn 1: commit a gateway response node.
	first := runStateE2EPostTurn(t, server.URL, "req_e2e_stateonly_1",
		`{"model":"gpt-4o-mini","stream":false,"input":"`+priorUserText+`"}`)
	require.True(t, state.LooksLikeGatewayResponseID(first.Id),
		"turn 1 must return a resolvable gateway id, got %q", first.Id)

	// Turn 2: previous_response_id only, NO input at all.
	second := runStateE2EPostTurn(t, server.URL, "req_e2e_stateonly_2",
		`{"model":"gpt-4o-mini","stream":false,"previous_response_id":"`+first.Id+`"}`)
	require.Equal(t, "completed", second.Status, "state-only follow-up must complete")

	// The upstream for turn 2 must have received the hydrated prior context: the
	// only source of the prior user text is state hydration (turn 2 sent no input).
	var chatReq relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal(up.lastBody(t), &chatReq), "failed to decode turn-2 upstream body")
	require.NotEmpty(t, chatReq.Messages, "turn 2 must carry hydrated messages")

	var sawPriorUser bool
	for _, m := range chatReq.Messages {
		if strings.Contains(m.StringContent(), priorUserText) {
			sawPriorUser = true
		}
	}
	require.True(t, sawPriorUser,
		"turn 2 upstream request must contain the hydrated prior user text %q; messages=%+v", priorUserText, chatReq.Messages)
}

// TestResponseStateE2E_ChainHydration proves that a chained turn hydrates BOTH the
// prior user message and the prior assistant reply into the upstream request
// (C01/M04), so the effective transcript contains at least three messages: prior
// user, prior assistant, and the new user input.
func TestResponseStateE2E_ChainHydration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	enableStateForTest(t)
	applyStateE2EEnv(t)

	const priorUserText = "remember the code is 4242"
	const priorAssistant = "ok"
	up, server := newStateE2EUpstream(t, priorAssistant)

	// Turn 1: "remember the code is 4242" -> assistant "ok".
	first := runStateE2EPostTurn(t, server.URL, "req_e2e_chain_1",
		`{"model":"gpt-4o-mini","stream":false,"input":"`+priorUserText+`"}`)
	require.True(t, state.LooksLikeGatewayResponseID(first.Id), "turn 1 must return a gateway id, got %q", first.Id)

	// Turn 2: continue from turn 1, asking for the code.
	const followUp = "what is the code?"
	second := runStateE2EPostTurn(t, server.URL, "req_e2e_chain_2",
		`{"model":"gpt-4o-mini","stream":false,"previous_response_id":"`+first.Id+`","input":"`+followUp+`"}`)
	require.Equal(t, "completed", second.Status)

	var chatReq relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal(up.lastBody(t), &chatReq), "failed to decode turn-2 upstream body")
	require.GreaterOrEqual(t, len(chatReq.Messages), 3,
		"chained turn must hydrate prior user + prior assistant + new input; messages=%+v", chatReq.Messages)

	var sawPriorUser, sawPriorAssistant, sawFollowUp bool
	for _, m := range chatReq.Messages {
		content := m.StringContent()
		if m.Role == "user" && strings.Contains(content, priorUserText) {
			sawPriorUser = true
		}
		if m.Role == "assistant" && strings.Contains(content, priorAssistant) {
			sawPriorAssistant = true
		}
		if m.Role == "user" && strings.Contains(content, followUp) {
			sawFollowUp = true
		}
	}
	require.True(t, sawPriorUser, "hydrated transcript must include the prior user message; messages=%+v", chatReq.Messages)
	require.True(t, sawPriorAssistant, "hydrated transcript must include the prior assistant reply; messages=%+v", chatReq.Messages)
	require.True(t, sawFollowUp, "hydrated transcript must include the new user input; messages=%+v", chatReq.Messages)
}

// TestResponseStateE2E_GetDeleteRoundtrip proves the retrieve/delete lifecycle for
// a gateway-committed response (C10/C11): GET returns the stored output, DELETE
// tombstones it, and a subsequent GET is not-found, all through the exported
// action entry points under the same owner scope.
func TestResponseStateE2E_GetDeleteRoundtrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	enableStateForTest(t)
	applyStateE2EEnv(t)

	const storedAssistant = "stored answer 4242"
	_, server := newStateE2EUpstream(t, storedAssistant)

	// Commit a gateway response node via a real fallback turn.
	committed := runStateE2EPostTurn(t, server.URL, "req_e2e_lifecycle_1",
		`{"model":"gpt-4o-mini","stream":false,"input":"roundtrip stored output"}`)
	require.True(t, state.LooksLikeGatewayResponseID(committed.Id), "commit must return a gateway id, got %q", committed.Id)

	// GET the committed id -> 200 with the stored output.
	wGet := httptest.NewRecorder()
	cGet := newStateActionContext(t, wGet, http.MethodGet, committed.Id, "req_e2e_lifecycle_get1")
	getErr := RelayResponseAPIGetHelper(cGet)
	require.Nil(t, getErr, "GET of a committed gateway response must succeed")
	require.Equal(t, http.StatusOK, wGet.Code, "expected 200 on GET")

	var got openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &got), "failed to decode GET body")
	require.Equal(t, committed.Id, got.Id, "GET must echo the stored gateway id")
	require.NotEmpty(t, got.Output, "GET must return the stored output items")
	require.NotEmpty(t, got.Output[0].Content, "stored output item must carry content")
	require.Equal(t, storedAssistant, got.Output[0].Content[0].Text, "GET must return the stored assistant text")

	// DELETE the committed id -> 200 deleted.
	wDel := httptest.NewRecorder()
	cDel := newStateActionContext(t, wDel, http.MethodDelete, committed.Id, "req_e2e_lifecycle_del")
	delErr := RelayResponseAPIDeleteHelper(cDel)
	require.Nil(t, delErr, "DELETE of a committed gateway response must succeed")
	require.Equal(t, http.StatusOK, wDel.Code, "expected 200 on DELETE")

	var deleted map[string]any
	require.NoError(t, json.Unmarshal(wDel.Body.Bytes(), &deleted), "failed to decode DELETE body")
	require.Equal(t, true, deleted["deleted"], "DELETE must report deleted:true")

	// GET again -> not-found (feature enabled, legacy passthrough off).
	wGet2 := httptest.NewRecorder()
	cGet2 := newStateActionContext(t, wGet2, http.MethodGet, committed.Id, "req_e2e_lifecycle_get2")
	getErr2 := RelayResponseAPIGetHelper(cGet2)
	require.NotNil(t, getErr2, "GET after DELETE must be an error")
	require.Equal(t, http.StatusNotFound, getErr2.StatusCode, "GET after DELETE must be 404")
}

// newStateActionContext builds a minimal owner-scoped gin.Context for the GET and
// DELETE action helpers: it carries the fixture owner (so the state lookup resolves
// under the same scope the commit used) and the :response_id routing param.
func newStateActionContext(t *testing.T, recorder *httptest.ResponseRecorder, method, responseID, requestID string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/v1/responses/"+responseID, nil)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.RequestId, requestID)
	c.Params = gin.Params{{Key: "response_id", Value: responseID}}
	return c
}
