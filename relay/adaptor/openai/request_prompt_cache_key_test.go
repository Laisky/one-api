package openai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/channeltype"
	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/state"
)

// observedPromptCacheKeyLogger returns a DEBUG logger whose entries are
// captured. Parameters: t owns the assertion. It returns the logger and logs.
func observedPromptCacheKeyLogger(t *testing.T) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("prompt-cache-key-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	return lg, logs
}

// decodePromptCacheKey decodes a client prompt_cache_key JSON value.
func decodePromptCacheKey(t *testing.T, raw string) model.PromptCacheKey {
	t.Helper()
	var key model.PromptCacheKey
	require.NoError(t, json.Unmarshal([]byte(raw), &key))
	return key
}

// TestScopePromptCacheKeyForwardsOnlyToNativeOpenAI proves the destination
// policy: native OpenAI receives the tenant-scoped value, every other channel
// type and unauthenticated requests receive nothing, and no log line carries
// either the client or the forwarded value.
func TestScopePromptCacheKeyForwardsOnlyToNativeOpenAI(t *testing.T) {
	const clientKey = "tenant-shared-key"
	for _, tc := range []struct {
		name        string
		meta        *rmeta.Meta
		wantForward bool
	}{
		{name: "native openai", meta: &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 7}, wantForward: true},
		{name: "azure", meta: &rmeta.Meta{ChannelType: channeltype.Azure, UserId: 7}},
		{name: "openai compatible", meta: &rmeta.Meta{ChannelType: channeltype.OpenAICompatible, UserId: 7}},
		{name: "openrouter", meta: &rmeta.Meta{ChannelType: channeltype.OpenRouter, UserId: 7}},
		{name: "anonymous native openai", meta: &rmeta.Meta{ChannelType: channeltype.OpenAI}},
		{name: "missing meta", meta: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lg, logs := observedPromptCacheKeyLogger(t)
			key := model.NewPromptCacheKey(clientKey)
			require.Equal(t, tc.wantForward, ScopePromptCacheKey(lg, tc.meta, &key, "unit"))
			if tc.wantForward {
				require.Equal(t, model.NamespacePromptCacheKey(7, clientKey), key.Forwarded())
			} else {
				require.Empty(t, key.Forwarded())
			}
			require.Equal(t, 1, logs.Len(), "one DEBUG diagnostic per scoping decision")
			for _, entry := range logs.All() {
				require.Equal(t, zapcore.DebugLevel, entry.Level)
				encoded, err := json.Marshal(entry.ContextMap())
				require.NoError(t, err)
				require.NotContains(t, entry.Message+string(encoded), clientKey)
				require.NotContains(t, entry.Message+string(encoded), model.NamespacePromptCacheKey(7, clientKey))
			}
		})
	}
	require.False(t, ScopePromptCacheKey(nil, &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 7}, nil, "unit"))
}

// TestScopePromptCacheKeyRescopesWithheldKey proves a key scoped for native
// OpenAI is withheld when the same request is later prepared for another
// channel type (a provider retry), and re-scoped from the client value.
func TestScopePromptCacheKeyRescopesWithheldKey(t *testing.T) {
	key := model.NewPromptCacheKey("retry-key")
	require.True(t, ScopePromptCacheKey(nil, &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 3}, &key, "unit"))
	require.False(t, ScopePromptCacheKey(nil, &rmeta.Meta{ChannelType: channeltype.Azure, UserId: 3}, &key, "unit"))
	require.Empty(t, key.Forwarded())
	require.True(t, ScopePromptCacheKey(nil, &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 3}, &key, "unit"))
	require.Equal(t, model.NamespacePromptCacheKey(3, "retry-key"), key.Forwarded())
}

// TestPromptCacheKeyConversionsCarryOnlyScopedValues proves the Chat and
// Responses converters carry the client key between protocols while encoding
// only a scoped value, so a conversion can never leak the raw key.
func TestPromptCacheKeyConversionsCarryOnlyScopedValues(t *testing.T) {
	chat := &model.GeneralOpenAIRequest{Model: "gpt-4.1", PromptCacheKey: decodePromptCacheKey(t, `"conv-key"`)}
	chat.Messages = []model.Message{{Role: "user", Content: "hello"}}

	unscoped := ConvertChatCompletionToResponseAPI(chat)
	encoded, err := json.Marshal(unscoped)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "prompt_cache_key", "an unscoped key is never encoded")

	require.True(t, ScopePromptCacheKey(nil, &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 11}, &chat.PromptCacheKey, "unit"))
	scoped := ConvertChatCompletionToResponseAPI(chat)
	encoded, err = json.Marshal(scoped)
	require.NoError(t, err)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &root))
	require.JSONEq(t, `"`+model.NamespacePromptCacheKey(11, "conv-key")+`"`, string(root["prompt_cache_key"]))

	// Responses -> Chat fallback keeps the client value for the Chat
	// destination's own decision but withholds the Responses-side scope.
	back, err := ConvertResponseAPIToChatCompletionRequest(&ResponseAPIRequest{Model: "gpt-4.1", Input: ResponseAPIInput{"hello"}, PromptCacheKey: scoped.PromptCacheKey})
	require.NoError(t, err)
	encoded, err = json.Marshal(back)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "prompt_cache_key")
	require.True(t, back.PromptCacheKey.Present())
	require.True(t, ScopePromptCacheKey(nil, &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 12}, &back.PromptCacheKey, "unit"))
	require.Equal(t, model.NamespacePromptCacheKey(12, "conv-key"), back.PromptCacheKey.Forwarded(), "re-scoping derives from the client value, never the scoped one")
}

// TestResponseWSPromptCacheKeyIsTenantScoped proves a WebSocket response.create
// frame, which is otherwise forwarded verbatim, carries only the tenant-scoped
// key to native OpenAI, drops a malformed key and is closed for an ambiguous
// spelling.
func TestResponseWSPromptCacheKeyIsTenantScoped(t *testing.T) {
	state.SetForTest(nil)
	frame := []byte(`{"type":"response.create","model":"gpt-4o-mini","input":"hi","prompt_cache_key":"shared-key"}`)
	scoped := func(userID int) string {
		ownership := &responseWSSessionOwnership{}
		out, err := ownership.authorize(context.Background(), &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: userID, TokenId: 2, ChannelId: 3}, frame)
		require.NoError(t, err)
		require.NotContains(t, string(out), "shared-key", "the raw client key must never reach the shared organisation")
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &root))
		var forwarded string
		require.NoError(t, json.Unmarshal(root["prompt_cache_key"], &forwarded))
		return forwarded
	}
	first := scoped(1)
	require.Equal(t, model.NamespacePromptCacheKey(1, "shared-key"), first)
	require.Equal(t, first, scoped(1))
	require.NotEqual(t, first, scoped(2), "tenants sending one key must not share a cache partition")

	ownership := &responseWSSessionOwnership{}
	for name, bad := range map[string]string{
		"number":   `{"type":"response.create","prompt_cache_key":42}`,
		"oversize": `{"type":"response.create","prompt_cache_key":"` + strings.Repeat("k", model.MaxPromptCacheKeyLength+1) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := ownership.authorize(context.Background(), &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 1}, []byte(bad))
			require.NoError(t, err, "a malformed optional routing hint must not close the session")
			require.NotContains(t, string(out), "prompt_cache_key", "a malformed key is dropped, never forwarded")
		})
	}
	t.Run("case folded", func(t *testing.T) {
		out, err := ownership.authorize(context.Background(), &rmeta.Meta{ChannelType: channeltype.OpenAI, UserId: 1}, []byte(`{"type":"response.create","prompt_cache_key":"a","Prompt_Cache_Key":"b"}`))
		require.Nil(t, out)
		require.True(t, errors.Is(err, common.ErrAmbiguousJSONKey), "an ambiguous spelling must be rejected: %v", err)
	})
}

// TestResponseWSPromptCacheKeyEndToEnd drives a frame through the production
// WebSocket proxy and observes the bytes the provider socket receives.
func TestResponseWSPromptCacheKeyEndToEnd(t *testing.T) {
	state.SetForTest(nil)
	received := make(chan string, 4)
	upstream := newOwnershipUpstream(t, received)
	defer upstream.Close()
	done := make(chan struct{}, 1)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		lg, _ := observedPromptCacheKeyLogger(t)
		gmw.SetLogger(c, lg)
		meta := &rmeta.Meta{Mode: relaymode.ResponseAPI, ChannelType: channeltype.OpenAI, BaseURL: upstream.URL, APIKey: "fixture", OriginModelName: "gpt-4o-mini", ActualModelName: "gpt-4o-mini", UserId: 41, TokenId: 2, ChannelId: 3}
		_, _, _ = ResponseAPIWebSocketHandler(c, meta)
		done <- struct{}{}
	})
	proxy := httptest.NewServer(router)
	defer proxy.Close()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	defer func() { _ = conn.Close() }()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":"hi","prompt_cache_key":"ws-key"}`)))
	var forwarded string
	select {
	case forwarded = <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("frame was not forwarded")
	}
	require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not terminate")
	}
	require.NotContains(t, forwarded, "ws-key")
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(forwarded), &root))
	require.JSONEq(t, `"`+model.NamespacePromptCacheKey(41, "ws-key")+`"`, string(root["prompt_cache_key"]))
}
