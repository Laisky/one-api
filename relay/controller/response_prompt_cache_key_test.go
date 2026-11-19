package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestNormalizeResponseAPIRawBodyPromptCacheKey proves the native Responses
// body builder never forwards the raw client key: a scoped typed key replaces
// it, and an unscoped one (any non-OpenAI destination) removes every spelling.
func TestNormalizeResponseAPIRawBodyPromptCacheKey(t *testing.T) {
	raw := []byte(`{"model":"gpt-4.1","input":"hi","prompt_cache_key":"raw-key","Prompt_Cache_Key":"raw-key-2"}`)
	decode := func(t *testing.T) *openai.ResponseAPIRequest {
		t.Helper()
		request := &openai.ResponseAPIRequest{}
		require.NoError(t, json.Unmarshal(raw, request))
		return request
	}

	t.Run("scoped for native OpenAI", func(t *testing.T) {
		request := decode(t)
		require.True(t, openai.ScopePromptCacheKey(nil, &metalib.Meta{ChannelType: channeltype.OpenAI, UserId: 9}, &request.PromptCacheKey, "unit"))
		body, _, changed, err := normalizeResponseAPIRawBody(raw, request, channeltype.OpenAI)
		require.NoError(t, err)
		require.True(t, changed)
		require.NotContains(t, string(body), "raw-key")
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &root))
		require.NotContains(t, root, "Prompt_Cache_Key")
		require.JSONEq(t, `"`+relaymodel.NamespacePromptCacheKey(9, "raw-key-2")+`"`, string(root[promptCacheKeyRootKey]))
	})

	for _, channel := range []int{channeltype.Azure, channeltype.OpenAICompatible, channeltype.XAI} {
		t.Run("withheld", func(t *testing.T) {
			request := decode(t)
			require.False(t, openai.ScopePromptCacheKey(nil, &metalib.Meta{ChannelType: channel, UserId: 9}, &request.PromptCacheKey, "unit"))
			body, _, _, err := normalizeResponseAPIRawBody(raw, request, channel)
			require.NoError(t, err)
			require.NotContains(t, string(body), "raw-key")
			require.NotContains(t, string(body), promptCacheKeyRootKey)
		})
	}
}

// TestSyncResponsePromptCacheKeyReportsChanges pins the change reporting the
// passthrough fast path depends on: an unchanged scoped body stays unchanged.
func TestSyncResponsePromptCacheKeyReportsChanges(t *testing.T) {
	key := relaymodel.NewPromptCacheKey("k")
	require.True(t, key.ScopeToUser(1))
	encoded, err := json.Marshal(key.Forwarded())
	require.NoError(t, err)

	root := map[string]json.RawMessage{promptCacheKeyRootKey: encoded}
	changed, err := syncResponsePromptCacheKey(root, key)
	require.NoError(t, err)
	require.False(t, changed)

	changed, err = syncResponsePromptCacheKey(map[string]json.RawMessage{}, relaymodel.PromptCacheKey{})
	require.NoError(t, err)
	require.False(t, changed)

	root = map[string]json.RawMessage{promptCacheKeyRootKey: json.RawMessage(`"raw"`)}
	changed, err = syncResponsePromptCacheKey(root, relaymodel.NewPromptCacheKey("raw"))
	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, root, promptCacheKeyRootKey)
}
