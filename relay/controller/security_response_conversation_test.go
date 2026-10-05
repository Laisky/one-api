package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// conversationPriorMarker is text that exists only inside the seeded gateway
// conversation, so seeing it on the wire proves the conversation was hydrated.
const conversationPriorMarker = "prior conversation marker 6f1d"

// conversationUpstreamCall is one request the fake provider received.
type conversationUpstreamCall struct {
	path string
	body string
}

// conversationUpstream is a fake OpenAI provider that records every call and
// answers Chat Completions with an assistant message and native Responses with a
// completed response object, so either relay route can finish successfully.
type conversationUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []conversationUpstreamCall
}

// newConversationUpstream starts the recording provider and returns it.
func newConversationUpstream(t *testing.T) *conversationUpstream {
	t.Helper()
	upstream := &conversationUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read forwarded fixture body: %v", err)
		}
		upstream.mu.Lock()
		upstream.calls = append(upstream.calls, conversationUpstreamCall{path: r.URL.Path, body: string(body)})
		upstream.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		reply := `{"id":"resp_native_conversation","object":"response","status":"completed","output":[{"type":"message","id":"msg_native","role":"assistant","status":"completed","content":[{"type":"output_text","text":"native reply"}]}],"usage":{"input_tokens":6,"output_tokens":4,"total_tokens":10}}`
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			reply = `{"id":"chatcmpl-conversation","object":"chat.completion","created":1741036800,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"fallback reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":6,"completion_tokens":4,"total_tokens":10}}`
		}
		if _, err := w.Write([]byte(reply)); err != nil {
			t.Errorf("write fixture reply: %v", err)
		}
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

// recorded returns a snapshot of every provider call received so far.
func (u *conversationUpstream) recorded() []conversationUpstreamCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]conversationUpstreamCall(nil), u.calls...)
}

// runNativeConversationRelay drives RelayResponseAPIHelper for payload on the
// native OpenAI Responses channel backed by upstream, drains detached billing,
// and returns the relay error.
func runNativeConversationRelay(t *testing.T, upstream *conversationUpstream, payload string) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	c := setupResponseStateBillingContext(t, httptest.NewRecorder(), payload)
	c.Set(ctxkey.Channel, channeltype.OpenAI)
	c.Set(ctxkey.ChannelId, fallbackOpenAIChannelID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackOpenAIChannelID, Type: channeltype.OpenAI})
	// The api.openai.com suffix makes the channel a native Responses upstream.
	c.Set(ctxkey.BaseURL, upstream.server.URL+"/api.openai.com")
	c.Set(ctxkey.RequestId, fmt.Sprintf("req_conv_%s", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())))
	err := RelayResponseAPIHelper(c)
	drainSecurityConversationBilling(t)
	return err
}

// drainSecurityConversationBilling waits for detached post-billing tasks so the
// ledger assertions observe the settled balance.
func drainSecurityConversationBilling(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// conversationSelector renders a conversation selector in the requested wire form.
func conversationSelector(form, id string) string {
	if form == "object" {
		return `{"id":"` + id + `"}`
	}
	return `"` + id + `"`
}

// seedSecurityConversation stores a conversation for owner holding one prior
// user message that carries conversationPriorMarker, and returns its ID.
func seedSecurityConversation(t *testing.T, store state.ResponseStateStore, owner state.OwnerScope) string {
	t.Helper()
	id, err := state.NewConversationID()
	require.NoError(t, err)
	_, err = store.CreateConversation(context.Background(), &state.ConversationStateRecord{
		GatewayConversationID: id,
		Owner:                 owner,
		Items:                 []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"`+conversationPriorMarker+`"}]}`)},
	}, "")
	require.NoError(t, err)
	return id
}

// unavailableConversationStore fails every conversation read with a storage outage.
type unavailableConversationStore struct{ state.ResponseStateStore }

// GetConversation returns an unavailable-store error for any owner and identifier.
func (s unavailableConversationStore) GetConversation(context.Context, state.OwnerScope, string) (*state.ConversationStateRecord, error) {
	return nil, errors.WithStack(state.ErrStoreUnavailable)
}

// TestSecurityResponseNativeConversationOwnership proves a native Responses
// channel never forwards a raw conversation selector: the caller's own gateway
// conversation is served from the owner-scoped store through the hydrating
// fallback, and every unowned, unknown, deleted, or provider-created ID fails
// closed with conversation_not_found before reservation or dispatch.
func TestSecurityResponseNativeConversationOwnership(t *testing.T) {
	scenarios := []string{"owner", "foreign-user", "foreign-token", "provider-style", "unknown-gateway", "deleted", "allowlist-excluded", "unavailable"}
	for _, form := range []string{"string", "object"} {
		for _, scenario := range scenarios {
			t.Run(form+"/"+scenario, func(t *testing.T) {
				securityAdmissionSetup(t, 1_000_000)
				store := enableStateForTest(t)
				owner := state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID}

				var convID string
				switch scenario {
				case "foreign-user":
					convID = seedSecurityConversation(t, store, state.OwnerScope{UserID: fallbackUserID + 1, TokenID: fallbackTokenID})
				case "foreign-token":
					convID = seedSecurityConversation(t, store, state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID + 1})
				case "provider-style":
					// A provider-created conversation ID (OpenAI shape) that one-api never minted.
					convID = "conv_" + strings.Repeat("0123456789abcdef", 3)
				case "unknown-gateway":
					var err error
					convID, err = state.NewConversationID()
					require.NoError(t, err)
				case "deleted":
					convID = seedSecurityConversation(t, store, owner)
					require.NoError(t, store.DeleteConversation(context.Background(), owner, convID))
				default:
					convID = seedSecurityConversation(t, store, owner)
				}
				switch scenario {
				case "allowlist-excluded":
					// The owner's conversation exists, but neither the owner nor the
					// channel is in scope, so the fallback could not hydrate it.
					state.SetForTest(store, state.WithAllowlist("user:1"))
				case "unavailable":
					state.SetForTest(unavailableConversationStore{store})
				}

				upstream := newConversationUpstream(t)
				before := readSecurityLedger(t)
				payload := `{"model":"gpt-4o-mini","stream":false,"input":"follow-up turn","conversation":` + conversationSelector(form, convID) + `}`
				apiErr := runNativeConversationRelay(t, upstream, payload)
				calls := upstream.recorded()
				for _, call := range calls {
					t.Logf("provider received %s with conversation id on wire: %v", call.path, strings.Contains(call.body, convID))
				}

				if scenario == "owner" {
					require.Nil(t, apiErr, "the owner's gateway conversation must be served")
					require.Len(t, calls, 1, "exactly one provider call")
					require.True(t, strings.HasSuffix(calls[0].path, "/v1/chat/completions"),
						"a gateway conversation has no provider handle, so it must divert to the hydrating fallback; got %s", calls[0].path)
					require.NotContains(t, calls[0].body, convID, "the gateway conversation ID must never reach the provider")
					require.Contains(t, calls[0].body, conversationPriorMarker, "the stored conversation items must be hydrated")
					require.Contains(t, calls[0].body, "follow-up turn")
					conv, err := store.GetConversation(context.Background(), owner, convID)
					require.NoError(t, err)
					require.Greater(t, len(conv.Items), 1, "the served turn must be appended to the owner's conversation")
					return
				}

				require.NotNil(t, apiErr, "an unproven conversation must fail closed")
				require.Empty(t, calls, "conversation authorization must complete before provider dispatch")
				require.Equal(t, before, readSecurityLedger(t), "a rejected conversation must not touch the ledger")
				if scenario == "unavailable" {
					require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
					require.Equal(t, codeStateStoreUnavailable, apiErr.Code)
					return
				}
				require.Equal(t, http.StatusNotFound, apiErr.StatusCode)
				require.Equal(t, codeConversationNotFound, apiErr.Code)
			})
		}
	}
}

// TestSecurityResponseNativeConversationStateDisabled proves that without the
// gateway state layer a native channel rejects every conversation selector
// instead of forwarding an ID whose owner one-api cannot prove, mirroring the
// previous_response_id policy, while selector-free requests keep working.
func TestSecurityResponseNativeConversationStateDisabled(t *testing.T) {
	for name, selector := range map[string]string{
		"provider-string": `"conv_` + strings.Repeat("0123456789abcdef", 3) + `"`,
		"provider-object": `{"id":"conv_` + strings.Repeat("0123456789abcdef", 3) + `"}`,
		"gateway-shaped":  `"conv_` + strings.Repeat("ab", 16) + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			securityAdmissionSetup(t, 1_000_000)
			state.SetForTest(nil)
			upstream := newConversationUpstream(t)
			before := readSecurityLedger(t)
			apiErr := runNativeConversationRelay(t, upstream, `{"model":"gpt-4o-mini","stream":false,"input":"hello","conversation":`+selector+`}`)
			require.NotNil(t, apiErr, "an unowned conversation must never be forwarded")
			require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
			require.Equal(t, codeStateStoreUnavailable, apiErr.Code)
			require.Empty(t, upstream.recorded())
			require.Equal(t, before, readSecurityLedger(t))
		})
	}

	t.Run("no-selector-control", func(t *testing.T) {
		securityAdmissionSetup(t, 1_000_000)
		state.SetForTest(nil)
		upstream := newConversationUpstream(t)
		require.Nil(t, runNativeConversationRelay(t, upstream, `{"model":"gpt-4o-mini","stream":false,"input":"hello"}`))
		calls := upstream.recorded()
		require.Len(t, calls, 1)
		require.True(t, strings.HasSuffix(calls[0].path, "/v1/responses"), "a selector-free request stays on the native route")
	})
}

// TestSecurityResponseNativeConversationResolver pins the resolver contract at
// its own boundary: an invalid owner, unowned IDs (which must not rely on the
// fallback hydrator's second lookup), empty selectors, and the owner's divert.
func TestSecurityResponseNativeConversationResolver(t *testing.T) {
	store := enableStateForTest(t)
	convID := seedSecurityConversation(t, store, testOwner())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	invalid := testMeta()
	invalid.UserId = 0
	divert, apiErr := resolveNativeConversation(c, invalid, &openai.ResponseAPIRequest{Conversation: &openai.ResponseAPIConversation{Id: convID}})
	require.False(t, divert)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	require.Equal(t, codeConversationNotFound, apiErr.Code)

	for _, req := range []*openai.ResponseAPIRequest{
		nil,
		{},
		{Conversation: &openai.ResponseAPIConversation{}},
		{Conversation: &openai.ResponseAPIConversation{Id: "  "}},
	} {
		divert, apiErr := resolveNativeConversation(c, testMeta(), req)
		require.False(t, divert)
		require.Nil(t, apiErr, "an empty selector names nothing to authorize")
	}

	foreignID := seedSecurityConversation(t, store, state.OwnerScope{UserID: 2, TokenID: 2})
	for _, id := range []string{foreignID, "conv_" + strings.Repeat("0123456789abcdef", 3)} {
		divert, apiErr = resolveNativeConversation(c, testMeta(), &openai.ResponseAPIRequest{Conversation: &openai.ResponseAPIConversation{Id: id}})
		require.False(t, divert, "the resolver itself must not divert an unowned conversation")
		require.NotNil(t, apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode)
		require.Equal(t, codeConversationNotFound, apiErr.Code)
	}

	divert, apiErr = resolveNativeConversation(c, testMeta(), &openai.ResponseAPIRequest{Conversation: &openai.ResponseAPIConversation{Id: convID}})
	require.Nil(t, apiErr)
	require.True(t, divert, "an owned gateway conversation diverts to the hydrating fallback")
}

// TestSecurityResponseNativeConversationQueryBoundRequest proves a non-JSON
// Content-Type cannot bind a conversation-free typed request (so resolution sees
// no selector) while the raw JSON body, which the native path forwards, still
// names another owner's conversation: the wire builder never forwards one.
func TestSecurityResponseNativeConversationQueryBoundRequest(t *testing.T) {
	securityAdmissionSetup(t, 1_000_000)
	store := enableStateForTest(t)
	foreign := seedSecurityConversation(t, store, state.OwnerScope{UserID: fallbackUserID + 1, TokenID: fallbackTokenID})
	upstream := newSecurityBackgroundUpstream(t, `{"id":"resp_query_bound","object":"response","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`)
	// Version (not Id) binds a prompt without also binding a conversation Id.
	apiErr := runNativeResponseRelayAs(t, upstream, `{"model":"gpt-4o-mini","input":"hello","conversation":"`+foreign+`"}`, "text/plain", "Model=gpt-4o-mini&Version=1")
	require.Nil(t, apiErr, "the typed request names no conversation, so the native call proceeds")
	require.Len(t, upstream.forwarded(), 1)
	for _, body := range upstream.forwarded() {
		require.NotContains(t, body, foreign, "a raw-body conversation must never reach the provider")
		require.NotContains(t, body, `"conversation"`)
	}
}

// TestSecurityResponseNativePreviousResponseQueryBoundRequest proves the same
// typed/raw split cannot smuggle an unowned previous_response_id past
// resolveNativePreviousResponse: the typed request (bound from the query) names
// no parent, so the raw body's parent must not reach the provider either.
func TestSecurityResponseNativePreviousResponseQueryBoundRequest(t *testing.T) {
	securityAdmissionSetup(t, 1_000_000)
	enableStateForTest(t)
	upstream := newSecurityBackgroundUpstream(t, `{"id":"resp_query_bound","object":"response","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`)
	apiErr := runNativeResponseRelayAs(t, upstream, `{"model":"gpt-4o-mini","input":"hello","previous_response_id":"resp_foreign_provider_handle"}`, "text/plain", "Model=gpt-4o-mini&Version=1")
	require.Nil(t, apiErr, "the typed request names no parent, so the native call proceeds")
	require.Len(t, upstream.forwarded(), 1, "the body check below must observe a dispatched request")
	for _, body := range upstream.forwarded() {
		require.NotContains(t, body, "resp_foreign_provider_handle", "an unresolved raw-body parent must never reach the provider")
	}
}
