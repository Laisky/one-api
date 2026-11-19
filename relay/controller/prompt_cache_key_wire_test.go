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
	"github.com/Laisky/one-api/relay/relaymode"
)

// promptCacheKeySecondUserID is a second gateway tenant sharing the fixture
// token and channel, so isolation can be observed on the same upstream.
const promptCacheKeySecondUserID = 99061

// promptCacheKeyRoute names one client protocol and destination combination.
type promptCacheKeyRoute struct {
	name     string
	channel  int
	surface  effortSurface
	model    string
	wantPath string
	// nativeResponses points the channel at api.openai.com with an endpoint
	// override, so Responses clients take the native raw-body route instead
	// of the custom-base-URL Chat fallback.
	nativeResponses bool
	// config is the channel configuration for the route.
	config model.ChannelConfig
}

// promptCacheKeyPayload builds the client body for surface carrying the
// prompt_cache_key JSON value keyJSON. Parameters: surface selects the
// protocol, modelID is the requested model and keyJSON is raw JSON (an empty
// string omits the field). It returns the request body.
func promptCacheKeyPayload(surface effortSurface, modelID, keyJSON string) string {
	field := ""
	if keyJSON != "" {
		field = `,"prompt_cache_key":` + keyJSON
	}
	switch surface {
	case effortResponses:
		return `{"model":"` + modelID + `","stream":false,"max_output_tokens":64,"input":"hello"` + field + `}`
	case effortClaude:
		return `{"model":"` + modelID + `","stream":false,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]` + field + `}`
	default:
		return `{"model":"` + modelID + `","stream":false,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]` + field + `}`
	}
}

// ensurePromptCacheKeySecondUser creates the second tenant's user row.
// Parameters: t owns cleanup. It returns nothing.
func ensurePromptCacheKeySecondUser(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Where("id = ?", promptCacheKeySecondUserID).Delete(&model.User{}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: promptCacheKeySecondUserID, Username: "prompt-cache-key-b", AccessToken: "prompt-cache-key-second-user-tok", AffCode: "pck2", Quota: 1_000_000, Status: model.UserStatusEnabled}).Error)
	t.Cleanup(func() {
		drainResponseFallbackBilling(t)
		_ = model.DB.Where("id = ?", promptCacheKeySecondUserID).Delete(&model.User{}).Error
	})
}

// promptCacheKeyRelayResult is the observable outcome of one relayed request.
type promptCacheKeyRelayResult struct {
	failure *relaymodel.ErrorWithStatusCode
	paths   []string
	bodies  []map[string]json.RawMessage
}

// relayPromptCacheKey drives one client request through the production relay
// helper for route as userID against a fake upstream that records every
// request. Parameters: t owns assertions, route selects protocol and channel,
// userID is the gateway tenant and payload is the client body. It returns the
// relay failure (nil on success) and every upstream request observed.
func relayPromptCacheKey(t *testing.T, route promptCacheKeyRoute, userID int, payload string) promptCacheKeyRelayResult {
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
	var result promptCacheKeyRelayResult
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body read failed", http.StatusBadRequest)
			return
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(raw, &root); err != nil {
			http.Error(w, "body is not a JSON object", http.StatusBadRequest)
			return
		}
		mu.Lock()
		result.bodies = append(result.bodies, root)
		result.paths = append(result.paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/responses") {
			_, _ = io.WriteString(w, effortResponsesReply)
			return
		}
		_, _ = io.WriteString(w, effortChatReply)
	}))
	t.Cleanup(upstream.Close)
	oldClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = oldClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, string(route.surface), strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer fixture-key")
	gmw.SetLogger(c, logger.Logger)
	channelConfig := route.config
	baseURL := upstream.URL
	if route.nativeResponses {
		baseURL = "https://api.openai.com"
		channelConfig.EndpointURLs = map[string]string{
			channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): upstream.URL + "/v1/responses",
		}
	}
	c.Set(ctxkey.Channel, route.channel)
	c.Set(ctxkey.ChannelId, fallbackNVIDIAChannelID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackNVIDIAChannelID, Type: route.channel})
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, userID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, route.model)
	c.Set(ctxkey.BaseURL, baseURL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_prompt_cache_key")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "prompt-cache-key")
	c.Set(ctxkey.UserObj, &model.User{Id: userID, Quota: 1_000_000})
	c.Set(ctxkey.Config, channelConfig)

	switch route.surface {
	case effortResponses:
		result.failure = RelayResponseAPIHelper(c)
	case effortClaude:
		result.failure = RelayClaudeMessagesHelper(c)
	default:
		result.failure = RelayTextHelper(c)
	}
	drainResponseFallbackBilling(t)
	mu.Lock()
	defer mu.Unlock()
	return result
}

// relayPromptCacheKeyWire relays one successful request and returns the single
// upstream body. Parameters mirror relayPromptCacheKey. It returns the decoded
// root object the provider received.
func relayPromptCacheKeyWire(t *testing.T, route promptCacheKeyRoute, userID int, payload string) map[string]json.RawMessage {
	t.Helper()
	result := relayPromptCacheKey(t, route, userID, payload)
	require.Nil(t, result.failure, "relay must succeed: %v", result.failure)
	require.Len(t, result.bodies, 1, "exactly one upstream request")
	require.Equal(t, route.wantPath, result.paths[0])
	return result.bodies[0]
}

// forwardedPromptCacheKey returns the prompt_cache_key string a provider
// received, failing when it is absent or not a string.
func forwardedPromptCacheKey(t *testing.T, root map[string]json.RawMessage) string {
	t.Helper()
	raw, ok := root["prompt_cache_key"]
	require.True(t, ok, "native OpenAI must receive prompt_cache_key; got keys %v", rootKeys(root))
	var key string
	require.NoError(t, json.Unmarshal(raw, &key))
	return key
}

// rootKeys lists the root keys of an upstream body for failure messages.
func rootKeys(root map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(root))
	for key := range root {
		keys = append(keys, key)
	}
	return keys
}

// forEachIncludeUsageMode runs body once with ENFORCE_INCLUDE_USAGE on (the
// default) and once off, because turning it off enables the raw Chat
// passthrough and unknown-field merge branches. Parameters: t owns the
// subtests and body receives each subtest.
func forEachIncludeUsageMode(t *testing.T, body func(t *testing.T)) {
	t.Helper()
	for _, enforce := range []bool{true, false} {
		name := "enforce_include_usage"
		if !enforce {
			name = "raw_passthrough"
		}
		t.Run(name, func(t *testing.T) {
			previous := config.EnforceIncludeUsage
			config.EnforceIncludeUsage = enforce
			t.Cleanup(func() { config.EnforceIncludeUsage = previous })
			body(t)
		})
	}
}

// nativeOpenAIPromptCacheKeyRoutes are every native OpenAI route that must
// forward the tenant-scoped key.
var nativeOpenAIPromptCacheKeyRoutes = []promptCacheKeyRoute{
	{name: "responses native raw", channel: channeltype.OpenAI, surface: effortResponses, model: "gpt-4.1", wantPath: "/v1/responses", nativeResponses: true},
	{name: "responses chat fallback", channel: channeltype.OpenAI, surface: effortResponses, model: "gpt-4.1", wantPath: "/v1/chat/completions"},
	{name: "chat to responses", channel: channeltype.OpenAI, surface: effortChat, model: "gpt-4.1", wantPath: "/v1/responses"},
	{name: "chat completions body", channel: channeltype.OpenAI, surface: effortChat, model: "gpt-4o-search-preview", wantPath: "/v1/chat/completions"},
	{name: "claude to responses", channel: channeltype.OpenAI, surface: effortClaude, model: "gpt-4.1", wantPath: "/v1/responses"},
	{name: "claude to chat completions", channel: channeltype.OpenAI, surface: effortClaude, model: "gpt-4o-search-preview", wantPath: "/v1/chat/completions"},
}

// TestPromptCacheKeyNativeOpenAIForwardsTenantScopedKey proves every native
// OpenAI route forwards prompt_cache_key, never the client's raw value, and
// that the forwarded value is stable for one tenant and distinct across
// tenants that send the same key.
func TestPromptCacheKeyNativeOpenAIForwardsTenantScopedKey(t *testing.T) {
	const clientKey = "codex-session-0001"
	forEachIncludeUsageMode(t, func(t *testing.T) {
		for _, route := range nativeOpenAIPromptCacheKeyRoutes {
			t.Run(route.name, func(t *testing.T) {
				payload := promptCacheKeyPayload(route.surface, route.model, `"`+clientKey+`"`)
				first := forwardedPromptCacheKey(t, relayPromptCacheKeyWire(t, route, fallbackUserID, payload))
				require.NotEqual(t, clientKey, first, "the raw client key must never reach the shared upstream organisation")
				require.NotContains(t, first, clientKey)
				require.LessOrEqual(t, len(first), 64, "OpenAI rejects prompt_cache_key longer than 64 characters")
				require.Equal(t, relaymodel.NamespacePromptCacheKey(fallbackUserID, clientKey), first)

				again := forwardedPromptCacheKey(t, relayPromptCacheKeyWire(t, route, fallbackUserID, payload))
				require.Equal(t, first, again, "one tenant's key must map to one stable cache partition")

				ensurePromptCacheKeySecondUser(t)
				other := forwardedPromptCacheKey(t, relayPromptCacheKeyWire(t, route, promptCacheKeySecondUserID, payload))
				require.NotEqual(t, first, other, "two tenants sending the same key must not share a cache partition")
				require.Equal(t, relaymodel.NamespacePromptCacheKey(promptCacheKeySecondUserID, clientKey), other)
			})
		}
	})
}

// TestPromptCacheKeyNativeOpenAIOmittedKeyStaysOmitted proves the gateway does
// not invent a key for clients that send none (or an explicit null/empty one).
func TestPromptCacheKeyNativeOpenAIOmittedKeyStaysOmitted(t *testing.T) {
	for _, route := range nativeOpenAIPromptCacheKeyRoutes {
		for _, keyJSON := range []string{"", "null", `""`} {
			t.Run(route.name+"/"+keyJSON, func(t *testing.T) {
				root := relayPromptCacheKeyWire(t, route, fallbackUserID, promptCacheKeyPayload(route.surface, route.model, keyJSON))
				require.NotContains(t, root, "prompt_cache_key")
			})
		}
	}
}

// TestPromptCacheKeyOtherChannelsNeverReceiveKey proves non-OpenAI channel
// types never receive prompt_cache_key on any protocol, raw passthrough
// included, because other hosts may reject unknown fields and share one
// account across tenants.
func TestPromptCacheKeyOtherChannelsNeverReceiveKey(t *testing.T) {
	responseFormat := model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse}
	forEachIncludeUsageMode(t, func(t *testing.T) {
		for _, route := range []promptCacheKeyRoute{
			{name: "azure chat passthrough", channel: channeltype.Azure, surface: effortChat, model: "gpt-4o", wantPath: "/openai/deployments/gpt-4o/chat/completions"},
			{name: "azure chat to responses", channel: channeltype.Azure, surface: effortChat, model: "gpt-5", wantPath: "/openai/v1/responses"},
			{name: "azure responses native", channel: channeltype.Azure, surface: effortResponses, model: "gpt-5", wantPath: "/openai/v1/responses"},
			{name: "azure claude", channel: channeltype.Azure, surface: effortClaude, model: "gpt-5", wantPath: "/openai/v1/responses"},
			{name: "compatible chat passthrough", channel: channeltype.OpenAICompatible, surface: effortChat, model: "gpt-4.1", wantPath: "/v1/chat/completions"},
			{name: "compatible responses fallback", channel: channeltype.OpenAICompatible, surface: effortResponses, model: "gpt-4.1", wantPath: "/v1/chat/completions"},
			{name: "compatible responses native", channel: channeltype.OpenAICompatible, surface: effortResponses, model: "gpt-4.1", wantPath: "/v1/responses", config: responseFormat},
			{name: "compatible chat to responses", channel: channeltype.OpenAICompatible, surface: effortChat, model: "gpt-4.1", wantPath: "/v1/responses", config: responseFormat},
			{name: "compatible claude", channel: channeltype.OpenAICompatible, surface: effortClaude, model: "gpt-4.1", wantPath: "/v1/chat/completions"},
		} {
			t.Run(route.name, func(t *testing.T) {
				payload := promptCacheKeyPayload(route.surface, route.model, `"codex-session-0001"`)
				root := relayPromptCacheKeyWire(t, route, fallbackUserID, payload)
				require.NotContains(t, root, "prompt_cache_key", "%s must not forward prompt_cache_key", route.name)
			})
		}
	})
}

// TestPromptCacheKeyMalformedValueIsDropped proves a non-string or oversize
// prompt_cache_key never fails a request (it was served before the key was
// forwarded) and never reaches the provider, on every protocol and channel
// type.
func TestPromptCacheKeyMalformedValueIsDropped(t *testing.T) {
	oversize := `"` + strings.Repeat("k", relaymodel.MaxPromptCacheKeyLength+1) + `"`
	forEachIncludeUsageMode(t, func(t *testing.T) {
		for _, route := range []promptCacheKeyRoute{
			nativeOpenAIPromptCacheKeyRoutes[0],
			nativeOpenAIPromptCacheKeyRoutes[2],
			nativeOpenAIPromptCacheKeyRoutes[4],
			{name: "compatible chat passthrough", channel: channeltype.OpenAICompatible, surface: effortChat, model: "gpt-4.1", wantPath: "/v1/chat/completions"},
		} {
			for name, keyJSON := range map[string]string{"number": `42`, "object": `{"id":"x"}`, "oversize": oversize} {
				t.Run(route.name+"/"+name, func(t *testing.T) {
					root := relayPromptCacheKeyWire(t, route, fallbackUserID, promptCacheKeyPayload(route.surface, route.model, keyJSON))
					require.NotContains(t, root, "prompt_cache_key", "a malformed key is dropped, never forwarded")
				})
			}
		}
	})
}

// TestPromptCacheKeyBoundaryLengthIsAccepted proves a key of exactly the
// documented maximum length is accepted and scoped.
func TestPromptCacheKeyBoundaryLengthIsAccepted(t *testing.T) {
	clientKey := strings.Repeat("é", relaymodel.MaxPromptCacheKeyLength)
	route := nativeOpenAIPromptCacheKeyRoutes[0]
	root := relayPromptCacheKeyWire(t, route, fallbackUserID, promptCacheKeyPayload(route.surface, route.model, `"`+clientKey+`"`))
	require.Equal(t, relaymodel.NamespacePromptCacheKey(fallbackUserID, clientKey), forwardedPromptCacheKey(t, root))
}

// TestPromptCacheKeyCaseFoldedSpellingNeverLeaksRawValue proves a case-folded
// spelling, which Go's struct decoder folds into the typed field while raw
// maps keep it verbatim, never carries the client's raw value upstream: native
// OpenAI receives only the canonical, tenant-scoped key and every other
// channel receives no spelling at all.
func TestPromptCacheKeyCaseFoldedSpellingNeverLeaksRawValue(t *testing.T) {
	const rawKey = "raw-leak-key"
	routes := append([]promptCacheKeyRoute(nil), nativeOpenAIPromptCacheKeyRoutes...)
	routes = append(routes,
		promptCacheKeyRoute{name: "azure chat passthrough", channel: channeltype.Azure, surface: effortChat, model: "gpt-4o", wantPath: "/openai/deployments/gpt-4o/chat/completions"},
		promptCacheKeyRoute{name: "compatible chat passthrough", channel: channeltype.OpenAICompatible, surface: effortChat, model: "gpt-4.1", wantPath: "/v1/chat/completions"},
		promptCacheKeyRoute{name: "compatible responses native", channel: channeltype.OpenAICompatible, surface: effortResponses, model: "gpt-4.1", wantPath: "/v1/responses", config: model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse}},
	)
	forEachIncludeUsageMode(t, func(t *testing.T) {
		for _, route := range routes {
			t.Run(route.name, func(t *testing.T) {
				payload := strings.TrimSuffix(promptCacheKeyPayload(route.surface, route.model, ""), "}") + `,"Prompt_Cache_KEY":"` + rawKey + `"}`
				result := relayPromptCacheKey(t, route, fallbackUserID, payload)
				if result.failure != nil {
					require.Equal(t, http.StatusBadRequest, result.failure.StatusCode, "an ambiguous spelling may only be rejected as a client error")
					require.Empty(t, result.bodies)
					return
				}
				require.Len(t, result.bodies, 1)
				root := result.bodies[0]
				encoded, err := json.Marshal(root)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), rawKey, "the raw client key must never reach a provider")
				for key := range root {
					if strings.EqualFold(key, promptCacheKeyRootKey) {
						require.Equal(t, promptCacheKeyRootKey, key, "only the canonical spelling may be forwarded")
					}
				}
				if route.channel == channeltype.OpenAI {
					require.Equal(t, relaymodel.NamespacePromptCacheKey(fallbackUserID, rawKey), forwardedPromptCacheKey(t, root))
					return
				}
				require.NotContains(t, root, promptCacheKeyRootKey)
			})
		}
	})
}
