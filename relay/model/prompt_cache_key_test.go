package model

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPromptCacheKeyDecodeValidatesClientValue pins the client contract: a
// string of at most 64 characters or null, and HTTP-400-classifiable errors
// for every other JSON value, which is recorded as invalid rather than failing.
func TestPromptCacheKeyDecodeValidatesClientValue(t *testing.T) {
	for name, tc := range map[string]struct {
		raw     string
		present bool
		wantErr bool
	}{
		"string":             {raw: `"tenant-key"`, present: true},
		"null":               {raw: `null`},
		"empty":              {raw: `""`},
		"max runes":          {raw: `"` + strings.Repeat("é", MaxPromptCacheKeyLength) + `"`, present: true},
		"oversize":           {raw: `"` + strings.Repeat("k", MaxPromptCacheKeyLength+1) + `"`, wantErr: true},
		"oversize multibyte": {raw: `"` + strings.Repeat("é", MaxPromptCacheKeyLength+1) + `"`, wantErr: true},
		"number":             {raw: `42`, wantErr: true},
		"bool":               {raw: `true`, wantErr: true},
		"object":             {raw: `{"id":"x"}`, wantErr: true},
		"array":              {raw: `["x"]`, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var key PromptCacheKey
			err := json.Unmarshal([]byte(tc.raw), &key)
			require.NoError(t, err, "a malformed optional routing hint never fails the request decode")
			if tc.wantErr {
				require.True(t, key.Invalid())
				require.ErrorIs(t, key.Validate(), ErrInvalidPromptCacheKey)
				require.False(t, key.Present(), "a malformed key is never forwarded")
				return
			}
			require.Equal(t, tc.present, key.Present())
			require.Empty(t, key.Forwarded(), "decoding never produces a forwardable value")
		})
	}
}

// TestPromptCacheKeyRequestDecodeToleratesMalformedKey proves a malformed key
// never fails the typed request decode on any protocol DTO (requests carrying
// one were served before the key was forwarded), and is never forwarded.
func TestPromptCacheKeyRequestDecodeToleratesMalformedKey(t *testing.T) {
	body := `{"model":"m","prompt_cache_key":{"nested":true}}`
	var chat GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(body), &chat))
	require.True(t, chat.PromptCacheKey.Invalid())
	require.False(t, chat.PromptCacheKey.Present())
	var claude ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(body), &claude))
	require.True(t, claude.PromptCacheKey.Invalid())
}

// TestPromptCacheKeyEncodesOnlyScopedValue proves the client's raw key is
// never serialized: request DTOs omit an unscoped key entirely and encode only
// the tenant-scoped value after ScopeToUser.
func TestPromptCacheKeyEncodesOnlyScopedValue(t *testing.T) {
	var chat GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m","prompt_cache_key":"raw-client-key"}`), &chat))
	var claude ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m","max_tokens":1,"messages":[],"prompt_cache_key":"raw-client-key"}`), &claude))
	for name, value := range map[string]any{"chat": chat, "claude": claude} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "prompt_cache_key", "%s must omit an unscoped key", name)
		require.NotContains(t, string(encoded), "raw-client-key")
	}

	require.True(t, chat.PromptCacheKey.ScopeToUser(5))
	encoded, err := json.Marshal(chat)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "raw-client-key")
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &root))
	require.JSONEq(t, `"`+NamespacePromptCacheKey(5, "raw-client-key")+`"`, string(root["prompt_cache_key"]))

	chat.PromptCacheKey.Withhold()
	encoded, err = json.Marshal(chat)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "prompt_cache_key")
	require.JSONEq(t, `null`, string(mustMarshalPromptCacheKey(t, chat.PromptCacheKey)))
}

// mustMarshalPromptCacheKey encodes key directly, outside a request DTO.
func mustMarshalPromptCacheKey(t *testing.T, key PromptCacheKey) []byte {
	t.Helper()
	encoded, err := json.Marshal(key)
	require.NoError(t, err)
	return encoded
}

// TestPromptCacheKeyScopeIsIdempotentAndTenantBound proves scoping always
// derives from the client value: repeating it is stable, scoping for another
// user never hashes an already-scoped value, and absent keys stay absent.
func TestPromptCacheKeyScopeIsIdempotentAndTenantBound(t *testing.T) {
	key := NewPromptCacheKey("session-1")
	require.True(t, key.ScopeToUser(1))
	first := key.Forwarded()
	require.True(t, key.ScopeToUser(1))
	require.Equal(t, first, key.Forwarded(), "re-scoping must not hash the scoped value again")
	require.True(t, key.ScopeToUser(2))
	require.Equal(t, NamespacePromptCacheKey(2, "session-1"), key.Forwarded())

	absent := NewPromptCacheKey("")
	require.False(t, absent.ScopeToUser(1))
	require.True(t, absent.IsZero())
}

// TestNamespacePromptCacheKeyDerivation pins the derivation: a stable 32-hex
// value within OpenAI's 64-character limit, distinct per user and per key, and
// unambiguous across user/key boundaries.
func TestNamespacePromptCacheKeyDerivation(t *testing.T) {
	derived := NamespacePromptCacheKey(42, "codex-session-0001")
	// Golden value: changing the derivation re-partitions every tenant's cache.
	require.Equal(t, "4ec3c281f8827c7ab898818fd08bb14b", derived) // sha256("one-api/prompt_cache_key/v1\x0042\x00codex-session-0001")[:16]
	require.Len(t, derived, 32)
	require.LessOrEqual(t, len(derived), MaxPromptCacheKeyLength)
	_, err := hex.DecodeString(derived)
	require.NoError(t, err)

	require.Equal(t, derived, NamespacePromptCacheKey(42, "codex-session-0001"))
	require.NotEqual(t, derived, NamespacePromptCacheKey(43, "codex-session-0001"))
	require.NotEqual(t, derived, NamespacePromptCacheKey(42, "codex-session-0002"))
	require.NotEqual(t, NamespacePromptCacheKey(1, "23"), NamespacePromptCacheKey(12, "3"), "user/key boundaries must not be re-splittable")
	require.NotContains(t, derived, "codex")
}
